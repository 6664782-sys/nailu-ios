package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/outbound"
	_ "github.com/xtls/xray-core/main/distro/all"
	"nailu/ios/engine/anytls"
	"nailu/ios/engine/profile"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"
)

var lock sync.Mutex
var instance *core.Instance

func ParseSubscription(raw string) (string, error) {
	p, err := profile.Parse(raw)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(p)
	return string(data), err
}
func Configuration(nodeJSON string) (string, error) {
	var n profile.Node
	if err := json.Unmarshal([]byte(nodeJSON), &n); err != nil {
		return "", errors.New("节点数据无效")
	}
	return profile.Config(n)
}
func Start(configuration string) error {
	lock.Lock()
	defer lock.Unlock()
	if instance != nil {
		return errors.New("连接引擎已运行")
	}
	debug.SetMemoryLimit(24 << 20)
	debug.SetGCPercent(40)
	next, err := startInstance(configuration)
	if err != nil {
		return errors.New("无法启动连接引擎，请检查节点配置")
	}
	instance = next
	return nil
}
func startInstance(configuration string) (*core.Instance, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configuration), &document); err != nil {
		return nil, err
	}
	var peers []struct {
		Protocol string `json:"protocol"`
		Settings struct {
			Servers []struct {
				Address  string `json:"address"`
				Port     int    `json:"port"`
				Password string `json:"password"`
			} `json:"servers"`
		} `json:"settings"`
		Stream struct {
			TLS struct {
				ServerName string `json:"serverName"`
				Insecure   bool   `json:"allowInsecure"`
			} `json:"tlsSettings"`
		} `json:"streamSettings"`
	}
	if err := json.Unmarshal(document["outbounds"], &peers); err != nil {
		return nil, err
	}
	if len(peers) != 1 || peers[0].Protocol != "nailu-anytls" {
		return core.StartInstance("json", []byte(configuration))
	}
	peer := peers[0]
	if len(peer.Settings.Servers) != 1 || peer.Stream.TLS.Insecure {
		return nil, errors.New("invalid AnyTLS peer")
	}
	server := peer.Settings.Servers[0]
	if server.Port < 1 || server.Port > 65535 {
		return nil, errors.New("invalid AnyTLS port")
	}
	client, err := anytls.New(net.JoinHostPort(server.Address, strconv.Itoa(server.Port)), server.Password, peer.Stream.TLS.ServerName, nil)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			client.Close()
		}
	}()
	// Authenticate the actual encrypted peer before reporting tunnel startup.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = client.Verify(ctx); err != nil {
		return nil, err
	}
	document["outbounds"] = json.RawMessage(`[]`)
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	config, err := core.LoadConfig("json", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	next, err := core.New(config)
	if err != nil {
		return nil, err
	}
	manager, ok := next.GetFeature(outbound.ManagerType()).(outbound.Manager)
	if !ok {
		next.Close()
		return nil, errors.New("outbound manager missing")
	}
	if err = manager.AddHandler(context.Background(), &anytls.Handler{Client: client}); err != nil {
		next.Close()
		return nil, err
	}
	if err = next.Start(); err != nil {
		next.Close()
		return nil, err
	}
	success = true
	return next, nil
}
func Stop() error {
	lock.Lock()
	defer lock.Unlock()
	if instance == nil {
		return nil
	}
	err := instance.Close()
	instance = nil
	return err
}
