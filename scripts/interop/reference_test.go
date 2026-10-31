package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"io"
	nailu "nailu/ios/engine/anytls"
	"nailu/ios/engine/socks"
	"net"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// External interoperability harness only: this file is not shipped in NaiLu.
func TestNailuReferenceInterop(t *testing.T) {
	origin := httptest.NewTLSServer(nil)
	cert := origin.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(origin.Certificate())
	origin.Close()
	sum := sha256.Sum256([]byte("local-fixture"))
	passwordSha256 = sum[:]
	server := NewMyServer(&tls.Config{Certificates: []tls.Certificate{cert}})
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	var sessions atomic.Int32
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() {
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			sessions.Add(1)
			go handleTcpConnection(ctx, c, server)
		}
	}()
	tcpEcho, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer tcpEcho.Close()
	go func() {
		for {
			c, e := tcpEcho.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	udpEcho, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer udpEcho.Close()
	go func() {
		b := make([]byte, 65535)
		for {
			n, a, e := udpEcho.ReadFrom(b)
			if e != nil {
				return
			}
			if _, e := udpEcho.WriteTo(b[:n], a); e != nil {
				t.Errorf("UDP fixture echo: %v", e)
				return
			}
		}
	}()
	client, e := nailu.New(listener.Addr().String(), "local-fixture", "127.0.0.1", &tls.Config{RootCAs: roots})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	if e = client.Verify(ctx); e != nil {
		t.Fatal(e)
	}
	_, tcpPort, _ := net.SplitHostPort(tcpEcho.Addr().String())
	port, _ := strconv.Atoi(tcpPort)
	for i := 0; i < 3; i++ {
		s, e := client.Dial(ctx, "127.0.0.1", uint16(port))
		if e != nil {
			t.Fatal(e)
		}
		stop := context.AfterFunc(ctx, func() { s.Close() })
		payload := bytes.Repeat([]byte("NaiLu AnyTLS TCP "), 5000)
		written := make(chan error, 1)
		go func() { _, e := s.Write(payload); written <- e }()
		got := make([]byte, len(payload))
		if _, e = io.ReadFull(s, got); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(payload, got) {
			t.Fatal("TCP payload mismatch")
		}
		if e = <-written; e != nil {
			t.Fatal(e)
		}
		stop()
		s.Close()
	}
	u, e := client.DialUDP(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer u.Close()
	stop := context.AfterFunc(ctx, func() { u.Close() })
	defer stop()
	_, udpPort, _ := net.SplitHostPort(udpEcho.LocalAddr().String())
	port, _ = strconv.Atoi(udpPort)
	baseline, e := net.Dial("udp", udpEcho.LocalAddr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer baseline.Close()
	for _, payload := range [][]byte{[]byte("DNS-style datagram"), bytes.Repeat([]byte{1, 2, 3, 4}, 300), bytes.Repeat([]byte{1, 2, 3, 4}, 2048), bytes.Repeat([]byte{1, 2, 3, 4}, 3000)} {
		// macOS limits outgoing UDP datagrams independently of the tunnel.
		// Require the 18, 1200 and 8192-byte cases everywhere; test 12000
		// only when the direct loopback socket accepts it. Transport errors
		// after a successful baseline always fail the interoperability test.
		if e = baseline.SetDeadline(time.Now().Add(2 * time.Second)); e != nil {
			t.Fatal(e)
		}
		if _, e = baseline.Write(payload); e != nil {
			if len(payload) > 8192 && errors.Is(e, syscall.EMSGSIZE) {
				t.Logf("host UDP limit rejects %d bytes before tunneling: %v", len(payload), e)
				continue
			}
			t.Fatalf("direct UDP baseline (%d bytes): %v", len(payload), e)
		}
		direct := make([]byte, len(payload)+1)
		n, e := baseline.Read(direct)
		if e != nil || !bytes.Equal(direct[:n], payload) {
			t.Fatalf("direct UDP baseline echo (%d bytes): %v", len(payload), e)
		}
		t.Logf("testing UOT v2 datagram: %d bytes", len(payload))
		address, _ := nailu.Address("127.0.0.1", uint16(port), true)
		packet := binary.BigEndian.AppendUint16(address, uint16(len(payload)))
		packet = append(packet, payload...)
		if _, e = u.Write(packet); e != nil {
			t.Fatal(e)
		}
		host, returnedPort, e := nailu.ReadAddress(u, true)
		if e != nil {
			t.Fatalf("UOT response address (%d-byte payload): %v", len(payload), e)
		}
		if host != "127.0.0.1" || returnedPort != uint16(port) {
			t.Fatal("UDP endpoint mismatch")
		}
		var size [2]byte
		if _, e = io.ReadFull(u, size[:]); e != nil {
			t.Fatal(e)
		}
		got := make([]byte, binary.BigEndian.Uint16(size[:]))
		if _, e = io.ReadFull(u, got); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("UDP payload mismatch")
		}
	}
	if sessions.Load() != 1 {
		t.Fatalf("session reuse failed: %d", sessions.Load())
	}
	u.Close()
	proxyListener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	bridge := socks.New(proxyListener, func(ctx context.Context, host string, port uint16) (io.ReadWriteCloser, error) {
		return client.Dial(ctx, host, port)
	}, func(ctx context.Context) (io.ReadWriteCloser, error) { return client.DialUDP(ctx) })
	defer bridge.Close()
	port, _ = strconv.Atoi(tcpPort)
	tunnel, _, _ := openFixtureSOCKS(t, bridge.Addr().String(), 1, "127.0.0.1", uint16(port))
	expected := bytes.Repeat([]byte("HEV SOCKS path -> NaiLu AnyTLS -> reference server -> TCP echo\n"), 1500)
	sent := make(chan error, 1)
	go func() { _, err := tunnel.Write(expected); sent <- err }()
	actual := make([]byte, len(expected))
	if _, e = io.ReadFull(tunnel, actual); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(expected, actual) {
		t.Fatal("SOCKS/AnyTLS TCP mismatch")
	}
	if e = <-sent; e != nil {
		t.Fatal(e)
	}
	tunnel.Close()
	control, bound, boundPort := openFixtureSOCKS(t, bridge.Addr().String(), 3, "0.0.0.0", 0)
	defer control.Close()
	udpClient, e := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP(bound), Port: int(boundPort)})
	if e != nil {
		t.Fatal(e)
	}
	defer udpClient.Close()
	udpClient.SetDeadline(time.Now().Add(5 * time.Second))
	port, _ = strconv.Atoi(udpPort)
	address, _ := nailu.Address("127.0.0.1", uint16(port), false)
	expected = bytes.Repeat([]byte{1, 2, 3, 4}, 300)
	packet := append([]byte{0, 0, 0}, address...)
	packet = append(packet, expected...)
	if _, e = udpClient.Write(packet); e != nil {
		t.Fatal(e)
	}
	received := make([]byte, 4096)
	n, e := udpClient.Read(received)
	if e != nil {
		t.Fatal(e)
	}
	reader := bytes.NewReader(received[3:n])
	responseHost, responsePort, e := nailu.ReadAddress(reader, false)
	if e != nil || responseHost != "127.0.0.1" || responsePort != uint16(port) || !bytes.Equal(received[n-reader.Len():n], expected) {
		t.Fatal("SOCKS/AnyTLS UDP mismatch")
	}
	t.Log("reference server: verified TLS + authentication + TCP + UOT v2 UDP + session reuse")
	t.Log("standalone SOCKS bridge: verified TCP and UDP through AnyTLS reference server; no Xray runtime")
}
func openFixtureSOCKS(t *testing.T, address string, command byte, host string, port uint16) (net.Conn, string, uint16) {
	t.Helper()
	c, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err = c.Write([]byte{5, 1, 0}); err != nil {
		t.Fatal(err)
	}
	var methods [2]byte
	if _, err = io.ReadFull(c, methods[:]); err != nil || methods != [2]byte{5, 0} {
		t.Fatal("SOCKS authentication mismatch", err)
	}
	encoded, _ := nailu.Address(host, port, false)
	if _, err = c.Write(append([]byte{5, command, 0}, encoded...)); err != nil {
		t.Fatal(err)
	}
	var reply [3]byte
	if _, err = io.ReadFull(c, reply[:]); err != nil || reply != [3]byte{5, 0, 0} {
		t.Fatalf("SOCKS command failed: %v %v", reply, err)
	}
	bound, boundPort, err := nailu.ReadAddress(c, false)
	if err != nil {
		t.Fatal(err)
	}
	return c, bound, boundPort
}
