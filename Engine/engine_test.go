package engine

import (
	"strings"
	"testing"
)

func TestEngineLifecycle(t *testing.T) {
	config, err := Configuration(`{"id":"fixture","name":"Fixture","protocol":"trojan","host":"example.invalid","port":443,"password":"fixture","serverName":"example.invalid","network":"tcp"}`)
	if err != nil {
		t.Fatal(err)
	}
	config = strings.Replace(config, `"port":10808`, `"port":0`, 1)
	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if err := Start(config); err == nil {
		t.Fatal("duplicate start accepted")
	}
	if err := Stop(); err != nil {
		t.Fatal(err)
	}
	if err := Stop(); err != nil {
		t.Fatal("stop is not idempotent")
	}
}
