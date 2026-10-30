package profile

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAndSecureConfig(t *testing.T) {
	raw := "trojan://secret@example.com:443?sni=tls.example.com#SG%20One\nss://" + base64.RawStdEncoding.EncodeToString([]byte("aes-128-gcm:pass")) + "@example.org:8443#HK"
	result, err := Parse(base64.StdEncoding.EncodeToString([]byte(raw)))
	if err != nil || len(result.Nodes) != 2 {
		t.Fatalf("parse failed: %v", err)
	}
	if result.Nodes[0].Name != "SG One" {
		t.Fatal("label not decoded")
	}
	cfg, err := Config(result.Nodes[0])
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if json.Unmarshal([]byte(cfg), &data) != nil {
		t.Fatal("invalid JSON")
	}
	if !strings.Contains(cfg, `"allowInsecure":false`) || !strings.Contains(cfg, `"listen":"127.0.0.1"`) {
		t.Fatal("unsafe config")
	}
	if _, err = Config(result.Nodes[1]); err != nil {
		t.Fatal(err)
	}
}
func TestRejectUnsafeAndUnsupported(t *testing.T) {
	for _, s := range []string{"trojan://pw@example.com:443?allowInsecure=1", "trojan://pw@example.com:0", "hysteria2://pw@example.com:443", "ss://YWVzLTEyOC1nY206cHc@example.com:443?plugin=unsafe"} {
		if _, err := Parse(s); err == nil {
			t.Fatalf("accepted unsafe or unsupported node %s", s)
		}
	}
	if _, err := Config(Node{Protocol: "ss", Host: "example.com", Port: 443, Password: "pw", Method: "none"}); err == nil {
		t.Fatal("accepted unencrypted method")
	}
}
func TestDedupAndUnsupportedCount(t *testing.T) {
	line := "trojan://pw@example.com:443#HK"
	p, err := Parse(line + "\n" + line + "\ntuic://pw@example.net:443")
	if err != nil || len(p.Nodes) != 1 || p.Unsupported != 1 {
		t.Fatal("dedup or protocol accounting failed")
	}
}
func TestCredentialEscaping(t *testing.T) {
	n := Node{Protocol: "trojan", Host: "example.com", Port: 443, Password: "secret\"\\\n", ServerName: "tls.example.com", Network: "ws", Path: "/v1"}
	cfg, err := Config(n)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]any
	if json.Unmarshal([]byte(cfg), &obj) != nil {
		t.Fatal("credential broke JSON")
	}
}

func TestWebSocketHostRemainsIndependentFromTLSName(t *testing.T) {
	p, err := Parse("trojan://pw@example.com:443?sni=tls.example.com&type=ws&host=edge.example.com&path=%2Fsocket#Fixture")
	if err != nil {
		t.Fatal(err)
	}
	config, err := Config(p.Nodes[0])
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal([]byte(config), &object); err != nil {
		t.Fatal(err)
	}
	outbound := object["outbounds"].([]any)[0].(map[string]any)
	stream := outbound["streamSettings"].(map[string]any)
	if stream["tlsSettings"].(map[string]any)["serverName"] != "tls.example.com" {
		t.Fatal("TLS SNI changed")
	}
	if stream["wsSettings"].(map[string]any)["headers"].(map[string]any)["Host"] != "edge.example.com" {
		t.Fatal("WebSocket host lost")
	}
}

func TestNaiLuAnyTLSSubscription(t *testing.T) {
	raw := "anytls://p%40ss%3Aword@[2001:db8::1]:443/?type=tcp&insecure=1&fp=chrome&sni=secure.example.com#SG%20AnyTLS"
	parsed, e := Parse(base64.StdEncoding.EncodeToString([]byte(raw)))
	if e != nil {
		t.Fatal(e)
	}
	n := parsed.Nodes[0]
	if n.Protocol != "anytls" || n.Password != "p@ss:word" || n.Host != "2001:db8::1" || n.ServerName != "secure.example.com" {
		t.Fatal("AnyTLS URI fields lost")
	}
	config, e := Config(n)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(config, `"protocol":"nailu-anytls"`) || !strings.Contains(config, `"allowInsecure":false`) {
		t.Fatal("AnyTLS engine mapping or TLS validation incorrect")
	}
	for _, query := range []string{"type=ws", "security=reality&pbk=fixture", "pcs=fixture"} {
		if _, e = Parse("anytls://pw@example.com:443?" + query); e == nil {
			t.Fatal("unsupported AnyTLS transport accepted")
		}
	}
}
