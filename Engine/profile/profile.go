package profile

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Node struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Protocol      string `json:"protocol"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Password      string `json:"password"`
	Method        string `json:"method,omitempty"`
	ServerName    string `json:"serverName,omitempty"`
	Network       string `json:"network,omitempty"`
	Path          string `json:"path,omitempty"`
	WebSocketHost string `json:"webSocketHost,omitempty"`
}
type Subscription struct {
	Nodes       []Node `json:"nodes"`
	Unsupported int    `json:"unsupported"`
}

func decode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	for _, e := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if d, err := e.DecodeString(s); err == nil {
			return d, nil
		}
	}
	return nil, errors.New("订阅编码无效")
}
func Parse(raw string) (Subscription, error) {
	out := Subscription{Nodes: []Node{}}
	if len(raw) > 4<<20 {
		return out, errors.New("订阅文件过大")
	}
	if !strings.Contains(raw, "://") {
		d, err := decode(raw)
		if err != nil {
			return out, err
		}
		raw = string(d)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		node, err := parseNode(line)
		if err != nil {
			out.Unsupported++
			continue
		}
		if !seen[node.ID] {
			seen[node.ID] = true
			out.Nodes = append(out.Nodes, node)
		}
	}
	if len(out.Nodes) == 0 {
		return out, errors.New("未找到可用的 AnyTLS、Trojan 或 Shadowsocks 节点")
	}
	return out, nil
}
func parseNode(line string) (Node, error) {
	var n Node
	u, err := url.Parse(line)
	if err != nil {
		return n, errors.New("节点格式无效")
	}
	n.Protocol = strings.ToLower(u.Scheme)
	if n.Protocol != "trojan" && n.Protocol != "ss" && n.Protocol != "anytls" {
		return n, errors.New("协议尚未支持")
	}
	if u.User == nil && n.Protocol == "ss" {
		d, err := decode(u.Host)
		if err != nil {
			return n, err
		}
		rebuilt := "ss://" + string(d)
		if u.Fragment != "" {
			rebuilt += "#" + url.PathEscape(u.Fragment)
		}
		u, err = url.Parse(rebuilt)
		if err != nil {
			return n, err
		}
	}
	n.Host = u.Hostname()
	n.Port, _ = strconv.Atoi(u.Port())
	if n.Host == "" || n.Port < 1 || n.Port > 65535 || u.User == nil {
		return n, errors.New("节点地址无效")
	}
	q := u.Query()
	if n.Protocol == "trojan" || n.Protocol == "anytls" {
		n.Password = u.User.Username()
		n.ServerName = q.Get("sni")
		if n.ServerName == "" {
			n.ServerName = q.Get("peer")
		}
		n.Network = q.Get("type")
		if n.Network == "" {
			n.Network = "tcp"
		}
		if n.Network != "tcp" && n.Network != "ws" {
			return n, errors.New("传输方式尚未支持")
		}
		if n.Protocol == "anytls" && (n.Network != "tcp" || q.Get("security") == "reality") {
			return n, errors.New("AnyTLS 首版仅支持标准 TLS TCP")
		}
		n.Path = q.Get("path")
		n.WebSocketHost = q.Get("host")
		if q.Get("pcs") != "" || q.Get("ech") != "" {
			return n, errors.New("证书固定或 ECH 配置尚未支持")
		}
		// Some panels mark every AnyTLS URI insecure even with a valid server
		// certificate. Accept the node but never honor that TLS downgrade.
		if n.Protocol != "anytls" && (q.Get("allowInsecure") == "1" || q.Get("allow_insecure") == "1" || q.Get("insecure") == "1" || q.Get("insecure") == "true") {
			return n, errors.New("节点要求不安全 TLS，已拒绝")
		}
	} else {
		if q.Get("plugin") != "" {
			return n, errors.New("Shadowsocks 插件尚未支持")
		}
		if password, ok := u.User.Password(); ok {
			n.Method = u.User.Username()
			n.Password = password
		} else {
			d, err := decode(u.User.Username())
			if err != nil {
				return n, err
			}
			p := strings.SplitN(string(d), ":", 2)
			if len(p) != 2 {
				return n, errors.New("密钥格式无效")
			}
			n.Method = p[0]
			n.Password = p[1]
		}
	}
	if n.Password == "" {
		return n, errors.New("节点密钥为空")
	}
	n.Name = u.Fragment
	if n.Name == "" {
		n.Name = n.Host
	}
	hash := sha256.Sum256([]byte(line))
	n.ID = hex.EncodeToString(hash[:12])
	return n, nil
}
func Config(n Node) (string, error) {
	if n.Host == "" || n.Port < 1 || n.Port > 65535 || n.Password == "" {
		return "", errors.New("节点信息不完整")
	}
	if net.ParseIP(n.Host) == nil && strings.ContainsAny(n.Host, " /\\\r\n") {
		return "", errors.New("节点域名无效")
	}
	server := map[string]any{"address": n.Host, "port": n.Port, "password": n.Password}
	outbound := map[string]any{"tag": "proxy", "settings": map[string]any{"servers": []any{server}}}
	switch n.Protocol {
	case "anytls":
		if n.Network != "" && n.Network != "tcp" {
			return "", errors.New("AnyTLS 仅支持标准 TLS TCP")
		}
		sni := n.ServerName
		if sni == "" {
			sni = n.Host
		}
		outbound["protocol"] = "nailu-anytls"
		outbound["streamSettings"] = map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"serverName": sni, "allowInsecure": false}}
	case "trojan":
		outbound["protocol"] = "trojan"
		network := n.Network
		if network == "" {
			network = "tcp"
		}
		if network != "tcp" && network != "ws" {
			return "", errors.New("传输方式尚未支持")
		}
		sni := n.ServerName
		if sni == "" {
			sni = n.Host
		}
		stream := map[string]any{"network": network, "security": "tls", "tlsSettings": map[string]any{"serverName": sni, "allowInsecure": false}}
		if network == "ws" {
			path := n.Path
			if path == "" {
				path = "/"
			}
			host := n.WebSocketHost
			if host == "" {
				host = sni
			}
			stream["wsSettings"] = map[string]any{"path": path, "headers": map[string]any{"Host": host}}
		}
		outbound["streamSettings"] = stream
	case "ss":
		methods := map[string]bool{"aes-128-gcm": true, "aes-256-gcm": true, "chacha20-ietf-poly1305": true, "2022-blake3-aes-128-gcm": true, "2022-blake3-aes-256-gcm": true, "2022-blake3-chacha20-poly1305": true}
		if !methods[n.Method] {
			return "", fmt.Errorf("加密方式尚未支持：%s", n.Method)
		}
		outbound["protocol"] = "shadowsocks"
		server["method"] = n.Method
	default:
		return "", errors.New("协议尚未支持")
	}
	config := map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": []any{map[string]any{"tag": "tun-socks", "listen": "127.0.0.1", "port": 10808, "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}}}, "outbounds": []any{outbound}}
	b, err := json.Marshal(config)
	return string(b), err
}
