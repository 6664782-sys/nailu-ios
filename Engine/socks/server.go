// Package socks implements a bounded, loopback-only SOCKS5 bridge for HEV.
// Destinations always traverse the supplied AnyTLS dialers; this package never
// resolves target hostnames or opens a direct connection to a destination.
package socks

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"nailu/ios/engine/anytls"
	"net"
	"sync"
	"time"
)

type TCPDial func(context.Context, string, uint16) (io.ReadWriteCloser, error)
type UDPDial func(context.Context) (io.ReadWriteCloser, error)
type Server struct {
	listener    net.Listener
	tcp         TCPDial
	udp         UDPDial
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	closed      bool
	connections map[net.Conn]struct{}
	workers     sync.WaitGroup
	slots       chan struct{}
}

func New(listener net.Listener, tcp TCPDial, udp UDPDial) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{listener: listener, tcp: tcp, udp: udp, ctx: ctx, cancel: cancel, connections: map[net.Conn]struct{}{}, slots: make(chan struct{}, 128)}
	s.workers.Add(1)
	go s.accept()
	return s
}
func (s *Server) Addr() net.Addr { return s.listener.Addr() }
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.listener.Close()
	for c := range s.connections {
		c.Close()
	}
	s.mu.Unlock()
	s.workers.Wait()
	return nil
}
func (s *Server) accept() {
	defer s.workers.Done()
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		peer, ok := c.RemoteAddr().(*net.TCPAddr)
		if !ok || !peer.IP.IsLoopback() {
			c.Close()
			continue
		}
		select {
		case s.slots <- struct{}{}:
		default:
			c.Close()
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			<-s.slots
			c.Close()
			return
		}
		s.connections[c] = struct{}{}
		s.workers.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.workers.Done()
			defer func() { s.mu.Lock(); delete(s.connections, c); s.mu.Unlock(); <-s.slots; c.Close() }()
			s.serve(c, peer.IP)
		}()
	}
}
func (s *Server) serve(c net.Conn, peer net.IP) {
	c.SetDeadline(time.Now().Add(10 * time.Second))
	var greeting [2]byte
	if _, err := io.ReadFull(c, greeting[:]); err != nil || greeting[0] != 5 || greeting[1] == 0 {
		return
	}
	methods := make([]byte, int(greeting[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	if !bytes.Contains(methods, []byte{0}) {
		writeAll(c, []byte{5, 255})
		return
	}
	if writeAll(c, []byte{5, 0}) != nil {
		return
	}
	var request [3]byte
	if _, err := io.ReadFull(c, request[:]); err != nil || request[0] != 5 || request[2] != 0 {
		return
	}
	host, port, err := anytls.ReadAddress(c, false)
	if err != nil {
		reply(c, 8, nil)
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	switch request[1] {
	case 1:
		if port == 0 {
			reply(c, 8, nil)
			return
		}
		dialCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		remote, err := s.tcp(dialCtx, host, port)
		stop()
		if err != nil {
			reply(c, 5, nil)
			return
		}
		defer remote.Close()
		if reply(c, 0, nil) != nil {
			return
		}
		c.SetDeadline(time.Time{})
		relayTCP(ctx, c, remote)
	case 3:
		ip := net.ParseIP(host)
		if ip == nil || (!ip.IsUnspecified() && !ip.Equal(peer)) {
			reply(c, 2, nil)
			return
		}
		s.relayUDP(ctx, c, peer, port)
	default:
		reply(c, 7, nil)
	}
}
func reply(c io.Writer, code byte, address net.Addr) error {
	host := "0.0.0.0"
	port := 0
	if udp, ok := address.(*net.UDPAddr); ok {
		host = udp.IP.String()
		port = udp.Port
	}
	encoded, err := anytls.Address(host, uint16(port), false)
	if err != nil {
		return err
	}
	return writeAll(c, append([]byte{5, code, 0}, encoded...))
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

type activeReader struct {
	io.Reader
	touch func()
}

func (r activeReader) Read(b []byte) (int, error) {
	n, err := r.Reader.Read(b)
	if n > 0 {
		r.touch()
	}
	return n, err
}
func relayTCP(ctx context.Context, local net.Conn, remote io.ReadWriteCloser) {
	var once sync.Once
	closeAll := func() { once.Do(func() { local.Close(); remote.Close() }) }
	stop := context.AfterFunc(ctx, closeAll)
	defer stop()
	defer closeAll()
	timer := time.AfterFunc(5*time.Minute, closeAll)
	defer timer.Stop()
	var activity sync.Mutex
	touch := func() { activity.Lock(); timer.Reset(5 * time.Minute); activity.Unlock() }
	done := make(chan struct{}, 2)
	go func() { io.Copy(remote, activeReader{local, touch}); done <- struct{}{} }()
	go func() { io.Copy(local, activeReader{remote, touch}); done <- struct{}{} }()
	<-done
	closeAll()
	<-done
}
func (s *Server) relayUDP(ctx context.Context, control net.Conn, peerIP net.IP, requestedPort uint16) {
	socket, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		reply(control, 1, nil)
		return
	}
	defer socket.Close()
	dialCtx, stopDial := context.WithTimeout(ctx, 10*time.Second)
	stream, err := s.udp(dialCtx)
	stopDial()
	if err != nil {
		reply(control, 5, nil)
		return
	}
	defer stream.Close()
	if reply(control, 0, socket.LocalAddr()) != nil {
		return
	}
	control.SetDeadline(time.Time{})
	var once sync.Once
	closeAll := func() { once.Do(func() { control.Close(); socket.Close(); stream.Close() }) }
	stop := context.AfterFunc(ctx, closeAll)
	defer stop()
	defer closeAll()
	timer := time.AfterFunc(90*time.Second, closeAll)
	defer timer.Stop()
	var activity sync.Mutex
	touch := func() { activity.Lock(); timer.Reset(90 * time.Second); activity.Unlock() }
	var peerMu sync.Mutex
	var endpoint *net.UDPAddr
	done := make(chan error, 3)
	go func() { _, err := io.Copy(io.Discard, control); done <- err }()
	go func() {
		b := make([]byte, 65535)
		for {
			n, from, err := socket.ReadFromUDP(b)
			if err != nil {
				done <- err
				return
			}
			if !from.IP.Equal(peerIP) || (requestedPort != 0 && from.Port != int(requestedPort)) {
				continue
			}
			// SOCKS5 UDP fragmentation is unsupported; never forward nonzero FRAG.
			if n < 4 || b[0] != 0 || b[1] != 0 || b[2] != 0 {
				continue
			}
			packet := bytes.NewReader(b[3:n])
			host, port, err := anytls.ReadAddress(packet, false)
			if err != nil || port == 0 {
				continue
			}
			peerMu.Lock()
			if endpoint == nil {
				copyPeer := *from
				endpoint = &copyPeer
			}
			accepted := endpoint.Port == from.Port && endpoint.IP.Equal(from.IP)
			peerMu.Unlock()
			if !accepted {
				continue
			}
			address, err := anytls.Address(host, port, true)
			if err != nil {
				continue
			}
			payload := b[n-packet.Len() : n]
			frame := binary.BigEndian.AppendUint16(address, uint16(len(payload)))
			frame = append(frame, payload...)
			if err = writeAll(stream, frame); err != nil {
				done <- err
				return
			}
			touch()
		}
	}()
	go func() {
		payload := make([]byte, 65535)
		for {
			host, port, err := anytls.ReadAddress(stream, true)
			if err != nil {
				done <- err
				return
			}
			var size [2]byte
			if _, err = io.ReadFull(stream, size[:]); err != nil {
				done <- err
				return
			}
			n := int(binary.BigEndian.Uint16(size[:]))
			if _, err = io.ReadFull(stream, payload[:n]); err != nil {
				done <- err
				return
			}
			address, err := anytls.Address(host, port, false)
			if err != nil {
				done <- err
				return
			}
			peerMu.Lock()
			var target *net.UDPAddr
			if endpoint != nil {
				copyPeer := *endpoint
				target = &copyPeer
			}
			peerMu.Unlock()
			if target == nil {
				done <- errors.New("unsolicited UDP reply")
				return
			}
			frame := append([]byte{0, 0, 0}, address...)
			frame = append(frame, payload[:n]...)
			if _, err = socket.WriteToUDP(frame, target); err != nil {
				done <- err
				return
			}
			touch()
		}
	}()
	<-done
	closeAll()
	<-done
	<-done
}
