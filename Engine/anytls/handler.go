package anytls

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	xsession "github.com/xtls/xray-core/common/session"
	"github.com/xtls/xray-core/transport"
	"io"
	"sync"
	"time"
)

// Handler implements Xray's public outbound interface. No direct/freedom
// fallback is installed; all TCP and UDP targets must traverse AnyTLS.
type Handler struct{ Client *Client }

func (h *Handler) Tag() string                          { return "proxy" }
func (h *Handler) SenderSettings() *serial.TypedMessage { return nil }
func (h *Handler) ProxySettings() *serial.TypedMessage  { return nil }
func (h *Handler) Start() error                         { return nil }
func (h *Handler) Close() error                         { return h.Client.Close() }
func (h *Handler) Dispatch(ctx context.Context, link *transport.Link) {
	defer common.Interrupt(link.Reader)
	defer common.Close(link.Writer)
	targets := xsession.OutboundsFromContext(ctx)
	if len(targets) == 0 {
		return
	}
	target := targets[len(targets)-1].Target
	if !target.IsValid() {
		return
	}
	udp := target.Network == xnet.Network_UDP
	if !udp && target.Network != xnet.Network_TCP {
		return
	}
	host, port := target.Address.String(), uint16(target.Port)
	if udp {
		host = "sp.v2.udp-over-tcp.arpa"
		port = 0
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stream *Stream
	var e error
	if udp {
		stream, e = h.Client.DialUDP(ctx)
	} else {
		stream, e = h.Client.Dial(ctx, host, port)
	}
	if e != nil {
		xsession.SubmitOutboundErrorToOriginator(ctx, errors.New("AnyTLS connection failed"))
		return
	}
	defer stream.Close()
	stop := context.AfterFunc(ctx, func() { stream.Close(); common.Interrupt(link.Reader) })
	defer stop()
	// Timer is reset on traffic, so quiet sessions cannot retain the extension
	// indefinitely. UDP has a shorter NAT lifetime than TCP.
	var activity sync.Mutex
	ttl := 5 * time.Minute
	if udp {
		ttl = 90 * time.Second
	}
	timer := time.AfterFunc(ttl, cancel)
	defer timer.Stop()
	touch := func() { activity.Lock(); timer.Reset(ttl); activity.Unlock() }
	results := make(chan error, 2)
	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		for {
			mb, e := link.Reader.ReadMultiBuffer()
			if len(mb) > 0 {
				touch()
			}
			for _, packet := range mb {
				if udp {
					destination := target
					if packet.UDP != nil {
						destination = *packet.UDP
					}
					address, err := Address(destination.Address.String(), uint16(destination.Port), true)
					if err != nil {
						e = err
						break
					}
					address = binary.BigEndian.AppendUint16(address, uint16(packet.Len()))
					if _, e = stream.Write(address); e != nil {
						break
					}
				}
				if _, e = stream.Write(packet.Bytes()); e != nil {
					break
				}
			}
			buf.ReleaseMulti(mb)
			if e != nil {
				results <- e
				return
			}
		}
	}()
	go func() {
		defer copies.Done()
		if !udp {
			results <- buf.Copy(&activityReader{Reader: buf.NewReader(stream), touch: touch}, link.Writer)
			return
		}
		for {
			host, port, e := ReadAddress(stream, true)
			if e != nil {
				results <- e
				return
			}
			var length [2]byte
			if _, e = io.ReadFull(stream, length[:]); e != nil {
				results <- e
				return
			}
			n := int32(binary.BigEndian.Uint16(length[:]))
			packet := buf.NewWithSize(max(n, buf.Size))
			if _, e = io.ReadFull(stream, packet.Extend(n)); e != nil {
				packet.Release()
				results <- e
				return
			}
			destination := xnet.UDPDestination(xnet.ParseAddress(host), xnet.Port(port))
			packet.UDP = &destination
			touch()
			if e = link.Writer.WriteMultiBuffer(buf.MultiBuffer{packet}); e != nil {
				results <- e
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
	case <-results:
	}
	cancel()
	stream.Close()
	common.Interrupt(link.Reader)
	common.Interrupt(link.Writer)
	copies.Wait()
}

type activityReader struct {
	buf.Reader
	touch func()
}

func (r *activityReader) ReadMultiBuffer() (buf.MultiBuffer, error) {
	mb, e := r.Reader.ReadMultiBuffer()
	if len(mb) > 0 {
		r.touch()
	}
	return mb, e
}
