package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	_ "github.com/xtls/xray-core/main/distro/all"
	"io"
	nailu "nailu/ios/engine/anytls"
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
	configuration, e := core.LoadConfig("json", bytes.NewBufferString(`{"log":{"loglevel":"none"},"inbounds":[],"outbounds":[]}`))
	if e != nil {
		t.Fatal(e)
	}
	instance, e := core.New(configuration)
	if e != nil {
		t.Fatal(e)
	}
	defer instance.Close()
	if e = instance.GetFeature(outbound.ManagerType()).(outbound.Manager).AddHandler(ctx, &nailu.Handler{Client: client}); e != nil {
		t.Fatal(e)
	}
	if e = instance.Start(); e != nil {
		t.Fatal(e)
	}
	port, _ = strconv.Atoi(tcpPort)
	tunnel, e := core.Dial(ctx, instance, xnet.TCPDestination(xnet.ParseAddress("127.0.0.1"), xnet.Port(port)))
	if e != nil {
		t.Fatal(e)
	}
	expected := []byte("Xray -> NaiLu AnyTLS -> reference server -> TCP echo")
	if _, e = tunnel.Write(expected); e != nil {
		t.Fatal(e)
	}
	actual := make([]byte, len(expected))
	if _, e = io.ReadFull(tunnel, actual); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(expected, actual) {
		t.Fatal("Xray TCP mismatch")
	}
	tunnel.Close()
	packetTunnel, e := core.DialUDP(ctx, instance)
	if e != nil {
		t.Fatal(e)
	}
	defer packetTunnel.Close()
	expected = []byte("Xray UDP DNS-path fixture")
	if _, e = packetTunnel.WriteTo(expected, udpEcho.LocalAddr()); e != nil {
		t.Fatal(e)
	}
	received := make([]byte, 1024)
	n, _, e := packetTunnel.ReadFrom(received)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(expected, received[:n]) {
		t.Fatal("Xray UDP mismatch")
	}
	t.Log("reference server: verified TLS + authentication + TCP + UOT v2 UDP + session reuse")
	t.Log("Xray outbound adapter: verified TCP and UDP echo through reference server")
}
