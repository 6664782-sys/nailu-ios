package anytls

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	waste byte = iota
	syn
	push
	fin
	settings
	alert
	updatePadding
	synAck
	heartRequest
	heartResponse
	serverSettings
)

var errClosed = errors.New("AnyTLS connection closed")

type Client struct {
	address, password string
	tls               *tls.Config
	mu                sync.Mutex
	sessions          map[*session]struct{}
	scheme            *padding
	seq               uint64
	closed            bool
	creating          int
	done              chan struct{}
}

func New(address, password, serverName string, tlsConfig *tls.Config) (*Client, error) {
	p, e := parsePadding(defaultScheme)
	if e != nil {
		return nil, e
	}
	if _, _, e = net.SplitHostPort(address); e != nil || password == "" || serverName == "" {
		return nil, errors.New("invalid AnyTLS peer")
	}
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName}
	} else {
		tlsConfig = tlsConfig.Clone()
		tlsConfig.ServerName = serverName
	}
	if tlsConfig.InsecureSkipVerify {
		return nil, errors.New("insecure AnyTLS TLS is disabled")
	}
	c := &Client{address: address, password: password, tls: tlsConfig, sessions: map[*session]struct{}{}, scheme: p, done: make(chan struct{})}
	go c.reap()
	return c, nil
}
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.done)
	all := make([]*session, 0, len(c.sessions))
	for s := range c.sessions {
		all = append(all, s)
	}
	c.mu.Unlock()
	for _, s := range all {
		s.close()
	}
	return nil
}
func (c *Client) reap() {
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-c.done:
			return
		case now := <-tick.C:
			c.mu.Lock()
			var old []*session
			for s := range c.sessions {
				if !s.busy && now.Sub(s.idle) > 30*time.Second {
					s.busy = true
					old = append(old, s)
				}
			}
			c.mu.Unlock()
			for _, s := range old {
				s.close()
			}
		}
	}
}
func (c *Client) acquire(ctx context.Context) (*session, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errClosed
	}
	var newest *session
	for s := range c.sessions {
		if !s.busy && (newest == nil || s.seq > newest.seq) {
			newest = s
		}
	}
	if newest != nil {
		newest.busy = true
		c.mu.Unlock()
		return newest, nil
	}
	if len(c.sessions)+c.creating >= 64 {
		c.mu.Unlock()
		return nil, errors.New("AnyTLS session limit reached")
	}
	c.creating++
	c.seq++
	seq := c.seq
	p := c.scheme
	c.mu.Unlock()
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: c.tls}
	conn, e := dialer.DialContext(ctx, "tcp", c.address)
	c.mu.Lock()
	c.creating--
	closed := c.closed
	c.mu.Unlock()
	if e != nil {
		return nil, errors.New("AnyTLS TLS handshake failed")
	}
	if closed {
		conn.Close()
		return nil, errClosed
	}
	s := &session{client: c, conn: conn, pad: p, seq: seq, busy: true, done: make(chan struct{}), ready: make(chan struct{})}
	auth := sha256.Sum256([]byte(c.password))
	n := 0
	if ranges := p.records[0]; len(ranges) > 0 && !ranges[0].check {
		n = choose(ranges[0])
	}
	bytes := make([]byte, 34+n)
	copy(bytes, auth[:])
	binary.BigEndian.PutUint16(bytes[32:34], uint16(n))
	_ = conn.SetWriteDeadline(time.Now().Add(8 * time.Second))
	e = writeFull(conn, bytes)
	_ = conn.SetWriteDeadline(time.Time{})
	if e != nil {
		conn.Close()
		return nil, e
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		conn.Close()
		return nil, errClosed
	}
	c.sessions[s] = struct{}{}
	c.mu.Unlock()
	return s, nil
}
func (c *Client) Verify(ctx context.Context) error {
	s, e := c.acquire(ctx)
	if e != nil {
		return e
	}
	if e = s.initialize(nil); e != nil {
		s.close()
		return e
	}
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case <-s.ready:
		c.release(s)
		return nil
	case <-s.done:
		return errors.New("AnyTLS authentication failed")
	case <-ctx.Done():
		s.close()
		return ctx.Err()
	case <-timer.C:
		s.close()
		return errors.New("AnyTLS v2 handshake timed out")
	}
}
func (c *Client) Dial(ctx context.Context, host string, port uint16) (*Stream, error) {
	return c.dial(ctx, host, port, nil)
}
func (c *Client) DialUDP(ctx context.Context) (*Stream, error) {
	header, _ := Address("0.0.0.0", 0, false)
	return c.dial(ctx, "sp.v2.udp-over-tcp.arpa", 0, append([]byte{0}, header...))
}
func (c *Client) dial(ctx context.Context, host string, port uint16, extra []byte) (*Stream, error) {
	destination, e := Address(host, port, false)
	if e != nil {
		return nil, e
	}
	s, e := c.acquire(ctx)
	if e != nil {
		return nil, e
	}
	s.mu.Lock()
	s.next++
	id := s.next
	stream := &Stream{s: s, id: id, data: make(chan []byte), done: make(chan struct{}), ack: make(chan error, 1)}
	s.active = stream
	s.mu.Unlock()
	destination = append(destination, extra...)
	initial := append(frame(syn, id, nil), frame(push, id, destination)...)
	if e = s.initialize(initial); e != nil {
		s.close()
		return nil, e
	}
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case e = <-stream.ack:
		if e != nil {
			stream.Close()
			return nil, e
		}
		return stream, nil
	case <-ctx.Done():
		stream.Close()
		return nil, ctx.Err()
	case <-timer.C:
		s.close()
		return nil, errors.New("AnyTLS v2 stream acknowledgement timed out")
	case <-stream.done:
		return nil, errClosed
	}
}
func (c *Client) release(s *session) {
	c.mu.Lock()
	if _, ok := c.sessions[s]; ok {
		s.busy = false
		s.idle = time.Now()
	}
	idle := 0
	for v := range c.sessions {
		if !v.busy {
			idle++
		}
	}
	closeIt := c.closed || idle > 4
	c.mu.Unlock()
	if closeIt {
		s.close()
	}
}

type session struct {
	client *Client
	conn   net.Conn
	pad    *padding
	seq    uint64
	// busy and idle are protected by client.mu; all protocol state by mu/writeMu.
	busy        bool
	idle        time.Time
	mu          sync.Mutex
	active      *Stream
	next        uint32
	writeMu     sync.Mutex
	writes      int
	once        sync.Once
	done        chan struct{}
	initialized bool
	ready       chan struct{}
	readyOnce   sync.Once
}

func (s *session) initialize(first []byte) error {
	// acquire exclusively reserves a session until its stream closes.
	if s.initialized {
		return s.write(first)
	}
	s.initialized = true
	b := frame(settings, 0, []byte("v=2\nclient=NaiLu/0.2.0\npadding-md5="+s.pad.digest()))
	b = append(b, first...)
	if e := s.write(b); e != nil {
		return e
	}
	go s.readLoop()
	return nil
}
func (s *session) close() {
	s.once.Do(func() {
		close(s.done)
		s.conn.Close()
		s.mu.Lock()
		active := s.active
		s.active = nil
		s.mu.Unlock()
		if active != nil {
			active.finish()
		}
		s.client.mu.Lock()
		delete(s.client.sessions, s)
		s.client.mu.Unlock()
	})
}
func frame(command byte, id uint32, payload []byte) []byte {
	b := make([]byte, 7+len(payload))
	b[0] = command
	binary.BigEndian.PutUint32(b[1:5], id)
	binary.BigEndian.PutUint16(b[5:7], uint16(len(payload)))
	copy(b[7:], payload)
	return b
}
func writeFull(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		b = b[n:]
	}
	return nil
}
func (s *session) write(b []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	select {
	case <-s.done:
		return errClosed
	default:
	}
	s.writes++
	rules := s.pad.records[s.writes]
	if s.writes >= s.pad.stop {
		rules = nil
	}
	_ = s.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	defer s.conn.SetWriteDeadline(time.Time{})
	for _, r := range rules {
		if r.check {
			if len(b) == 0 {
				return nil
			}
			continue
		}
		n := choose(r)
		if n == 0 {
			continue
		}
		if len(b) >= n {
			if e := writeFull(s.conn, b[:n]); e != nil {
				return e
			}
			b = b[n:]
			continue
		}
		packet := append([]byte(nil), b...)
		b = nil
		if extra := n - len(packet); extra >= 7 {
			packet = append(packet, frame(waste, 0, make([]byte, extra-7))...)
		}
		if len(packet) > 0 {
			if e := writeFull(s.conn, packet); e != nil {
				return e
			}
		}
	}
	return writeFull(s.conn, b)
}
func (s *session) readLoop() {
	defer s.close()
	for {
		var header [7]byte
		if _, e := io.ReadFull(s.conn, header[:]); e != nil {
			return
		}
		id := binary.BigEndian.Uint32(header[1:5])
		n := int(binary.BigEndian.Uint16(header[5:7]))
		data := make([]byte, n)
		if _, e := io.ReadFull(s.conn, data); e != nil {
			return
		}
		s.mu.Lock()
		active := s.active
		s.mu.Unlock()
		switch header[0] {
		case waste:
		case serverSettings:
			for _, line := range strings.Split(string(data), "\n") {
				if strings.TrimSpace(line) == "v=2" {
					s.readyOnce.Do(func() { close(s.ready) })
				}
			}
		case alert:
			return
		case updatePadding:
			p, e := parsePadding(string(data))
			if e != nil {
				return
			}
			s.client.mu.Lock()
			s.client.scheme = p
			s.client.mu.Unlock()
		case heartRequest:
			if e := s.write(frame(heartResponse, id, nil)); e != nil {
				return
			}
		case heartResponse:
		case synAck:
			if active != nil && active.id == id {
				var e error
				if n > 0 {
					e = errors.New("AnyTLS remote target rejected")
				}
				select {
				case active.ack <- e:
				default:
				}
			}
		case push:
			if active != nil && active.id == id && n > 0 {
				select {
				case active.data <- data:
				case <-active.done:
				case <-s.done:
					return
				}
			}
		case fin:
			if active != nil && active.id == id {
				s.mu.Lock()
				if s.active == active {
					s.active = nil
				}
				s.mu.Unlock()
				active.finish()
				s.client.release(s)
			}
		default:
			return
		}
	}
}

type Stream struct {
	readMu  sync.Mutex
	s       *session
	id      uint32
	data    chan []byte
	done    chan struct{}
	ack     chan error
	once    sync.Once
	pending []byte
}

func (v *Stream) finish() { v.once.Do(func() { close(v.done) }) }
func (v *Stream) Read(b []byte) (int, error) {
	v.readMu.Lock()
	defer v.readMu.Unlock()
	if len(b) == 0 {
		return 0, nil
	}
	if len(v.pending) == 0 {
		select {
		case v.pending = <-v.data:
		case <-v.done:
			return 0, io.EOF
		case <-v.s.done:
			return 0, io.EOF
		}
	}
	n := copy(b, v.pending)
	v.pending = v.pending[n:]
	return n, nil
}
func (v *Stream) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		select {
		case <-v.done:
			return total, errClosed
		default:
		}
		n := min(len(b), 16384)
		if e := v.s.write(frame(push, v.id, b[:n])); e != nil {
			v.s.close()
			return total, e
		}
		total += n
		b = b[n:]
	}
	return total, nil
}
func (v *Stream) Close() error {
	v.s.mu.Lock()
	owned := v.s.active == v
	if owned {
		v.s.active = nil
	}
	v.s.mu.Unlock()
	v.finish()
	if owned {
		if e := v.s.write(frame(fin, v.id, nil)); e != nil {
			v.s.close()
			return e
		}
		v.s.client.release(v.s)
	}
	return nil
}
func (v *Stream) LocalAddr() net.Addr  { return v.s.conn.LocalAddr() }
func (v *Stream) RemoteAddr() net.Addr { return v.s.conn.RemoteAddr() }

// Session is idle-pool reusable, so per-stream deadlines must not mutate a
// reused TLS connection. Callers cancel/close streams via context instead.
func (v *Stream) SetDeadline(time.Time) error      { return os.ErrInvalid }
func (v *Stream) SetReadDeadline(time.Time) error  { return os.ErrInvalid }
func (v *Stream) SetWriteDeadline(time.Time) error { return os.ErrInvalid }
