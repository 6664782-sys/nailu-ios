package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"nailu/ios/engine/anytls"
	"nailu/ios/engine/profile"
	"nailu/ios/engine/socks"
	"net"
	"runtime/debug"
	"strconv"
	"sync"
	"time"
)

var lock sync.Mutex
var instance *runningEngine

type runningEngine struct {
	client *anytls.Client
	socks  *socks.Server
}

func (v *runningEngine) close() { v.client.Close(); v.socks.Close() }
func ParseSubscription(raw string) (string, error) {
	p, err := profile.Parse(raw)
	if err != nil {
		return "", err
	}
	nodes := p.Nodes[:0]
	for _, n := range p.Nodes {
		if n.Protocol == "anytls" {
			nodes = append(nodes, n)
		} else {
			p.Unsupported++
		}
	}
	p.Nodes = nodes
	if len(nodes) == 0 {
		return "", errors.New("未找到可用的 AnyTLS 节点")
	}
	b, err := json.Marshal(p)
	return string(b), err
}
func Configuration(raw string) (string, error) {
	var n profile.Node
	if json.Unmarshal([]byte(raw), &n) != nil {
		return "", errors.New("节点数据无效")
	}
	if n.Protocol != "anytls" {
		return "", errors.New("此客户端仅支持 AnyTLS 节点")
	}
	return profile.Config(n)
}
func Start(raw string) error {
	lock.Lock()
	defer lock.Unlock()
	if instance != nil {
		return errors.New("连接引擎已运行")
	}
	debug.SetMemoryLimit(24 << 20)
	debug.SetGCPercent(40)
	next, err := startInstance(raw)
	if err != nil {
		return errors.New("无法启动 AnyTLS 引擎，请检查节点配置和证书")
	}
	instance = next
	return nil
}
func startInstance(raw string) (*runningEngine, error) {
	var config struct {
		Inbounds []struct {
			Listen   string
			Port     int
			Protocol string
			Settings struct {
				Auth string
				UDP  bool
			}
		}
		Outbounds []struct {
			Protocol string
			Settings struct {
				Servers []struct {
					Address  string
					Port     int
					Password string
				}
			}
			StreamSettings struct {
				Network     string
				Security    string
				TLSSettings struct {
					ServerName    string
					AllowInsecure bool
				}
			}
		}
	}
	if len(raw) > 4<<20 || json.Unmarshal([]byte(raw), &config) != nil || len(config.Inbounds) != 1 || len(config.Outbounds) != 1 {
		return nil, errors.New("invalid configuration")
	}
	in := config.Inbounds[0]
	peer := config.Outbounds[0]
	if in.Listen != "127.0.0.1" || in.Port < 0 || in.Port > 65535 || in.Protocol != "socks" || in.Settings.Auth != "noauth" || !in.Settings.UDP {
		return nil, errors.New("invalid loopback SOCKS configuration")
	}
	if peer.Protocol != "nailu-anytls" || len(peer.Settings.Servers) != 1 || peer.StreamSettings.Network != "tcp" || peer.StreamSettings.Security != "tls" || peer.StreamSettings.TLSSettings.AllowInsecure {
		return nil, errors.New("invalid AnyTLS configuration")
	}
	server := peer.Settings.Servers[0]
	if server.Port < 1 || server.Port > 65535 {
		return nil, errors.New("invalid peer port")
	}
	client, err := anytls.New(net.JoinHostPort(server.Address, strconv.Itoa(server.Port)), server.Password, peer.StreamSettings.TLSSettings.ServerName, nil)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			client.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = client.Verify(ctx); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(in.Listen, strconv.Itoa(in.Port)))
	if err != nil {
		return nil, err
	}
	bridge := socks.New(listener, func(ctx context.Context, host string, port uint16) (io.ReadWriteCloser, error) {
		return client.Dial(ctx, host, port)
	}, func(ctx context.Context) (io.ReadWriteCloser, error) { return client.DialUDP(ctx) })
	success = true
	return &runningEngine{client: client, socks: bridge}, nil
}
func Stop() error {
	lock.Lock()
	defer lock.Unlock()
	if instance != nil {
		instance.close()
		instance = nil
	}
	return nil
}
