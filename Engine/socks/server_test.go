package socks

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"nailu/ios/engine/anytls"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func testServer(t *testing.T, tcp TCPDial, udp UDPDial) *Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := New(listener, tcp, udp)
	t.Cleanup(func() { s.Close() })
	return s
}
func openRequest(t *testing.T, s *Server, command byte, host string, port uint16) (net.Conn, string, uint16, byte) {
	t.Helper()
	c, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if writeAll(c, []byte{5, 1, 0}) != nil {
		t.Fatal("greeting write failed")
	}
	var response [2]byte
	if _, err = io.ReadFull(c, response[:]); err != nil || response != [2]byte{5, 0} {
		t.Fatalf("method response: %v %v", response, err)
	}
	address, _ := anytls.Address(host, port, false)
	if err = writeAll(c, append([]byte{5, command, 0}, address...)); err != nil {
		t.Fatal(err)
	}
	var header [3]byte
	if _, err = io.ReadFull(c, header[:]); err != nil {
		t.Fatal(err)
	}
	if header[0] != 5 || header[2] != 0 {
		t.Fatalf("invalid reply: %v", header)
	}
	bound, boundPort, err := anytls.ReadAddress(c, false)
	if err != nil {
		t.Fatal(err)
	}
	return c, bound, boundPort, header[1]
}
func TestTCPRelaysWithoutResolvingDestination(t *testing.T) {
	destinations := make(chan string, 1)
	s := testServer(t, func(ctx context.Context, host string, port uint16) (io.ReadWriteCloser, error) {
		if port != 443 {
			t.Errorf("unexpected port: %d", port)
		}
		destinations <- host
		client, peer := net.Pipe()
		go func() { defer peer.Close(); io.Copy(peer, peer) }()
		return client, nil
	}, nil)
	c, _, _, code := openRequest(t, s, 1, "must-not-resolve.invalid", 443)
	if code != 0 {
		t.Fatalf("CONNECT failed: %d", code)
	}
	payload := bytes.Repeat([]byte("bidirectional AnyTLS stream "), 4000)
	sent := make(chan error, 1)
	go func() { sent <- writeAll(c, payload) }()
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("TCP data mismatch")
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if <-destinations != "must-not-resolve.invalid" {
		t.Fatal("destination was changed")
	}
	c.Close()
}
func TestUDPFramingOwnershipAndControlLifetime(t *testing.T) {
	var packets atomic.Int32
	destinations := make(chan string, 4)
	s := testServer(t, nil, func(context.Context) (io.ReadWriteCloser, error) {
		client, peer := net.Pipe()
		go func() {
			defer peer.Close()
			for {
				host, port, err := anytls.ReadAddress(peer, true)
				if err != nil {
					return
				}
				var size [2]byte
				if _, err = io.ReadFull(peer, size[:]); err != nil {
					return
				}
				payload := make([]byte, int(binary.BigEndian.Uint16(size[:])))
				if _, err = io.ReadFull(peer, payload); err != nil {
					return
				}
				packets.Add(1)
				destinations <- host
				address, _ := anytls.Address(host, port, true)
				frame := binary.BigEndian.AppendUint16(address, uint16(len(payload)))
				frame = append(frame, payload...)
				if writeAll(peer, frame) != nil {
					return
				}
			}
		}()
		return client, nil
	})
	control, host, port, code := openRequest(t, s, 3, "0.0.0.0", 0)
	if code != 0 || host != "127.0.0.1" || port == 0 {
		t.Fatalf("UDP association failed: %s:%d %d", host, port, code)
	}
	endpoint := &net.UDPAddr{IP: net.ParseIP(host), Port: int(port)}
	peer, err := net.DialUDP("udp4", nil, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	peer.SetDeadline(time.Now().Add(2 * time.Second))
	address, _ := anytls.Address("dns-through-tunnel.invalid", 53, false)
	payload := bytes.Repeat([]byte{1, 2, 3, 4}, 300)
	packet := append([]byte{0, 0, 0}, address...)
	packet = append(packet, payload...)
	fragmented := append([]byte(nil), packet...)
	fragmented[2] = 1
	peer.Write(fragmented) // Must not reach the transport or claim the association.
	peer.Write(packet)
	replyBuffer := make([]byte, 4096)
	n, err := peer.Read(replyBuffer)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(replyBuffer[3:n])
	returnedHost, returnedPort, err := anytls.ReadAddress(reader, false)
	if err != nil || returnedHost != "dns-through-tunnel.invalid" || returnedPort != 53 || !bytes.Equal(replyBuffer[n-reader.Len():n], payload) {
		t.Fatalf("UDP reply mismatch: %s:%d %v", returnedHost, returnedPort, err)
	}
	attacker, err := net.DialUDP("udp4", nil, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer attacker.Close()
	attacker.Write(packet) // A second local port must not hijack the association.
	peer.Write(packet)
	if _, err = peer.Read(replyBuffer); err != nil {
		t.Fatal(err)
	}
	if <-destinations != "dns-through-tunnel.invalid" || <-destinations != "dns-through-tunnel.invalid" {
		t.Fatal("UDP target was resolved or changed")
	}
	control.Close()
	stopped := make(chan struct{})
	go func() { s.Close(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("association did not stop when control closed")
	}
	if packets.Load() != 2 {
		t.Fatalf("fragment or foreign peer forwarded: %d packets", packets.Load())
	}
}
func TestRejectsAuthenticationAndUnsupportedCommands(t *testing.T) {
	s := testServer(t, nil, nil)
	c, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	writeAll(c, []byte{5, 1, 2})
	var response [2]byte
	if _, err = io.ReadFull(c, response[:]); err != nil || response != [2]byte{5, 255} {
		t.Fatalf("unsupported authentication accepted: %v %v", response, err)
	}
	_, _, _, code := openRequest(t, s, 2, "127.0.0.1", 80)
	if code != 7 {
		t.Fatalf("BIND accepted: %d", code)
	}
	_, _, _, code = openRequest(t, s, 3, "198.51.100.1", 0)
	if code != 2 {
		t.Fatalf("foreign UDP endpoint accepted: %d", code)
	}
}
func TestCloseInterruptsIncompleteHandshake(t *testing.T) {
	s := testServer(t, nil, nil)
	c, err := net.Dial("tcp", s.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown waited for handshake timeout")
	}
}
