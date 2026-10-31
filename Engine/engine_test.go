package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOnlyAnyTLSIsOffered(t *testing.T) {
	raw := "trojan://other@example.invalid:443#Other\nanytls://fixture@example.invalid:443?sni=example.invalid#NaiLu"
	result, err := ParseSubscription(raw)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Nodes       []struct{ Protocol string }
		Unsupported int
	}
	if err = json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Nodes) != 1 || parsed.Nodes[0].Protocol != "anytls" || parsed.Unsupported != 1 {
		t.Fatalf("unexpected supported nodes: %s", result)
	}
	if _, err = Configuration(`{"protocol":"trojan","host":"example.invalid","port":443,"password":"fixture"}`); err == nil {
		t.Fatal("legacy protocol accepted")
	}
}
func TestUnsafeListenerAndTLSAreRejectedBeforeOpeningSockets(t *testing.T) {
	config, err := Configuration(`{"id":"fixture","name":"Fixture","protocol":"anytls","host":"example.invalid","port":443,"password":"fixture","serverName":"example.invalid","network":"tcp"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{strings.Replace(config, `127.0.0.1`, `0.0.0.0`, 1), strings.Replace(config, `"allowInsecure":false`, `"allowInsecure":true`, 1), strings.Replace(config, `"nailu-anytls"`, `"freedom"`, 1)} {
		if _, err := startInstance(bad); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	if err = Stop(); err != nil {
		t.Fatal(err)
	}
	if err = Stop(); err != nil {
		t.Fatal("stop must be idempotent")
	}
}
