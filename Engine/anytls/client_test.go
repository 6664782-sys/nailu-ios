package anytls

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"net"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Independent wire fixture validates bytes and lifecycle, not the implementation
// helpers. Reference-server interoperability is a separate external test.
func fixture(t *testing.T) (string, *tls.Config, *atomic.Int32) {
	t.Helper()
	certificateSource := httptest.NewTLSServer(nil)
	certificate := certificateSource.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certificateSource.Certificate())
	certificateSource.Close()
	listener, e := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{certificate}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { listener.Close() })
	count := &atomic.Int32{}
	go func() {
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			count.Add(1)
			go func() {
				defer conn.Close()
				var auth [34]byte
				if _, e = io.ReadFull(conn, auth[:]); e != nil {
					return
				}
				sum := sha256.Sum256([]byte("fixture"))
				if !bytes.Equal(auth[:32], sum[:]) {
					return
				}
				if _, e = io.CopyN(io.Discard, conn, int64(binary.BigEndian.Uint16(auth[32:]))); e != nil {
					return
				}
				receivedSettings := false
				streams := map[uint32]bool{}
				for {
					var h [7]byte
					if _, e := io.ReadFull(conn, h[:]); e != nil {
						return
					}
					id := binary.BigEndian.Uint32(h[1:5])
					data := make([]byte, int(binary.BigEndian.Uint16(h[5:])))
					if _, e := io.ReadFull(conn, data); e != nil {
						return
					}
					send := func(cmd byte, payload []byte) {
						header := make([]byte, 7)
						header[0] = cmd
						binary.BigEndian.PutUint32(header[1:5], id)
						binary.BigEndian.PutUint16(header[5:], uint16(len(payload)))
						writeFull(conn, append(header, payload...))
					}
					switch h[0] {
					case 0:
					case 4:
						receivedSettings = true
						send(10, []byte("v=2"))
					case 1:
						if !receivedSettings {
							return
						}
						streams[id] = false
					case 2:
						if !streams[id] {
							host, port, e := ReadAddress(bytes.NewReader(data), false)
							if e != nil || host != "example.test" || port != 443 {
								return
							}
							streams[id] = true
							send(7, nil)
						} else {
							send(2, data)
						}
					case 3:
						delete(streams, id)
					}
				}
			}()
		}
	}()
	return listener.Addr().String(), &tls.Config{RootCAs: roots}, count
}
func TestTLSAuthenticationDataAndSessionReuse(t *testing.T) {
	address, config, count := fixture(t)
	client, e := New(address, "fixture", "127.0.0.1", config)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e = client.Verify(ctx); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		stream, e := client.Dial(ctx, "example.test", 443)
		if e != nil {
			t.Fatal(e)
		}
		payload := bytes.Repeat([]byte("payload"), 9000)
		done := make(chan error, 1)
		go func() { _, e := stream.Write(payload); done <- e }()
		result := make([]byte, len(payload))
		if _, e = io.ReadFull(stream, result); e != nil {
			t.Fatal(e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(payload, result) {
			t.Fatal("stream payload changed")
		}
		stream.Close()
	}
	if count.Load() != 1 {
		t.Fatalf("idle session not reused: %d", count.Load())
	}
	client.Close()
	if _, e = client.Dial(ctx, "example.test", 443); e == nil {
		t.Fatal("dial after close accepted")
	}
}
func TestRejectWrongPasswordAndCertificate(t *testing.T) {
	address, config, _ := fixture(t)
	client, e := New(address, "wrong", "127.0.0.1", config)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = client.Verify(ctx); e == nil {
		t.Fatal("wrong password accepted")
	}
	untrusted, e := New(address, "fixture", "127.0.0.1", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer untrusted.Close()
	if e = untrusted.Verify(ctx); e == nil {
		t.Fatal("untrusted certificate accepted")
	}
}
func TestUOTAddressAndPaddingBounds(t *testing.T) {
	for _, host := range []string{"1.1.1.1", "2001:db8::1", "dns.example.test"} {
		for _, uot := range []bool{false, true} {
			b, e := Address(host, 53, uot)
			if e != nil {
				t.Fatal(e)
			}
			got, port, e := ReadAddress(bytes.NewReader(b), uot)
			if e != nil || got != host || port != 53 {
				t.Fatal("address codec mismatch")
			}
		}
	}
	for _, bad := range []string{"stop=65\n0=30-30", "stop=8\n1=1-9999999", "stop=8\n1=-1-1"} {
		if _, e := parsePadding(bad); e == nil {
			t.Fatal("unbounded padding accepted")
		}
	}
	if _, e := New(net.JoinHostPort("127.0.0.1", "443"), "fixture", "localhost", &tls.Config{InsecureSkipVerify: true}); e == nil {
		t.Fatal("insecure TLS accepted")
	}
}
