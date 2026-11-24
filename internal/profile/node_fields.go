package profile

import (
	"strings"
	"sync"
)

// Form fields use the same keys as stored nodes and connection adapters.
var TLSFingerprints = []string{"Default", "Random", "Safari", "Safari15_3", "Safari15_5", "Safari16_0", "Safari26_3", "Chrome", "Chrome100", "Chrome110", "Chrome120", "Chrome140", "Chrome150", "Firefox", "Firefox109", "Firefox117", "Firefox120", "Firefox148"}
var TransportHeaders = []string{"none", "srtp", "utp", "wechat-video", "dtls", "wireguard", "dns"}

func DetailedProtocol(kind string) bool {
	switch kind {
	case "shadowtls", "vmess", "vless", "relay", "socks5", "socks5s", "http", "https", "http2", "http3", "trojan", "hysteria", "hysteria2", "anytls", "tuic", "juicity", "ssh", "wireguard", "openconnect", "trusttunnel", "masque", "snell", "mieru", "brook", "lua", "sudoku", "naive", "tailscale":
		return true
	}
	return false
}
func withGroup(group string, fields ...Field) []Field {
	for i := range fields {
		fields[i].Group = group
	}
	return fields
}
func placeholder(field Field, text string) Field { field.Placeholder = text; return field }
func textDefault(key, title, value string) Field {
	return Field{Key: key, Title: title, Default: value}
}
func pluginFields(n Node) []Field {
	fields := []Field{choose("plugin", "插件", "none", SSPlugins...)}
	return append(fields, withGroup("插件设置", ssPluginFields(n)...)...)
}
func transportNodeFields(n Node) []Field {
	choices := []string{"tcp", "http", "h2", "ws", "httpupgrade", "mkcp", "grpc", "quic", "xhttp"}
	if n.Kind() == "trojan" {
		choices = []string{"tcp", "ws", "grpc", "httpupgrade", "h2", "xhttp"}
	}
	fields := []Field{choose("network", "传输方式", "tcp", choices...)}
	var extra []Field
	switch n.TextOption("network") {
	case "http", "h2", "ws", "httpupgrade":
		extra = []Field{f("transport-host", "Host", false), textDefault("transport-path", "路径", "/")}
	case "mkcp":
		extra = []Field{choose("kcp.header", "类型", "none", TransportHeaders...), boolean("kcp.congestion", "启用拥塞控制", "false"), number("kcp.mtu", "MTU", "1350"), number("kcp.tti", "传输时间间隔", "50"), number("kcp.uplink-capacity", "上行链路容量", "5"), number("kcp.downlink-capacity", "下行链路容量", "20"), f("kcp.seed", "Seed", false)}
	case "grpc":
		extra = []Field{f("transport-host", "Host", false), f("service-name", "服务名称", false)}
	case "quic":
		extra = []Field{choose("quic.header", "类型", "none", TransportHeaders...), choose("quic.security", "安全", "none", "none")}
	case "xhttp":
		extra = append([]Field{f("transport-host", "Host", false), textDefault("transport-path", "路径", "/"), choose("xhttp-opts.mode", "XHTTP 模式", "auto", "auto", "packet-up", "stream-up", "stream-one")}, xhttpFields()...)
	}
	return append(fields, withGroup("传输设置", extra...)...)
}
func tlsNodeFields(n Node, forced, simple, hideHTTP2, mutual bool) []Field {
	fields := []Field{boolean("tls", "TLS", "false")}
	if forced {
		fields = nil
	}
	if !forced && n.TextOption("tls") != "true" {
		return fields
	}
	alpn := "h2,http/1.1"
	if simple {
		alpn = "hysteria"
	}
	extra := []Field{boolean("skip-cert-verify", "允许不安全", "false"), f("sni", "SNI", false), f("ech", "ECH", false), placeholder(f("alpn", "ALPN", false), alpn)}
	if !simple {
		if !hideHTTP2 {
			extra = append(extra, boolean("http2", "HTTP2", "false"))
		}
		extra = append(extra, f("reality-opts.public-key", "公钥", false), f("reality-opts.short-id", "短 ID", false), choose("client-fingerprint", "指纹", "Default", TLSFingerprints...), boolean("fragment", "片段", "false"))
	}
	fields = append(fields, withGroup("TLS 设置", extra...)...)
	if !simple && n.TextOption("fragment") == "true" {
		fields = append(fields, withGroup("TLS 设置/片段设置", number("fragment-opts.length-min", "Length 最小值", "40"), number("fragment-opts.length-max", "Length 最大值", "60"), number("fragment-opts.interval-min", "间隔最小值", "30"), number("fragment-opts.interval-max", "间隔最大值", "50"), textDefault("fragment-opts.packets", "数据包", "tlshello"))...)
	}
	fields = append(fields, withGroup("TLS 设置", f("tls-sha256", "SHA256", false), f("tls-pubkey-sha256", "TLS 公钥 SHA256", false))...)
	if mutual {
		fields = append(fields, withGroup("TLS 设置", boolean("mutual-auth", "相互认证", "false"))...)
		if n.TextOption("mutual-auth") == "true" {
			fields = append(fields, withGroup("TLS 设置/相互认证设置", secret("pkcs12-password", "密码", false), placeholder(secret("pkcs12", "PKCS12", true), "文件路径或 Base64"))...)
		}
	}
	return fields
}
func detailedNodeFields(n Node) []Field {
	var fields []Field
	switch n.Kind() {
	case "shadowtls":
		return shadowTLSFields("")
	case "vmess", "vless":
		fields = []Field{secret("uuid", "UUID", true)}
		if n.Kind() == "vmess" {
			alter := placeholder(number("alterId", "额外 ID", ""), "如果为0,AEAD启用")
			alter.Hint = "md5+aes"
			fields = append(fields, alter, choose("cipher", "算法", "auto", "none", "auto", "zero", "aes-128-gcm", "chacha20-poly1305"))
		} else {
			fields = append(fields, f("encryption", "加密", false), choose("flow", "流控", "none", "none", "xtls-rprx-vision"))
		}
		fields = append(fields, transportNodeFields(n)...)
		fields = append(fields, tlsNodeFields(n, false, false, false, false)...)
		fields = append(fields, boolean("mux", "多路复用", "false"), boolean("tfo", "TCP 快速打开", "false"), boolean("udp", "UDP 转发", "false"))
	case "relay", "socks5", "socks5s", "http", "https", "http2", "http3":
		fields = []Field{f("username", "用户", false)}
		switch n.Kind() {
		case "socks5", "socks5s":
			fields = append(fields, choose("auth-method", "算法", "auto", "auto", "strict", "random"))
		case "http", "https":
			fields = append(fields, choose("http-method", "算法", "auto", "auto", "CONNECT"))
		}
		if n.Kind() == "socks5s" || n.Kind() == "https" || n.Kind() == "http2" || n.Kind() == "http3" {
			fields = append(fields, tlsNodeFields(n, true, false, n.Kind() == "http2" || n.Kind() == "http3", true)...)
		}
		if n.Kind() != "http2" && n.Kind() != "http3" {
			fields = append(fields, pluginFields(n)...)
		}
		fields = append(fields, boolean("tfo", "TCP 快速打开", "false"))
		if n.Kind() == "relay" || n.Kind() == "socks5" || n.Kind() == "socks5s" {
			fields = append(fields, boolean("udp", "UDP 转发", "true"))
		}
		if n.Kind() == "https" || n.Kind() == "http2" || n.Kind() == "http3" {
			fields = append(fields, boolean("padding", "填充", "false"))
		}
		if n.Kind() == "http2" || n.Kind() == "http3" {
			fields = append(fields, choose("uot-version", "UDP Over TCP", "0", "0", "1", "2"))
		}
	case "trojan":
		fields = []Field{choose("trojan-protocol", "传输协议", "origin", "origin", "shadowsocks")}
		if n.TextOption("trojan-protocol") == "shadowsocks" {
			method := choose("trojan-ss.method", "算法", DefaultSSMethod, Methods...)
			if strings.HasPrefix(n.TextOption("trojan-ss.method"), "2022-") {
				method.Hint = "2022 内层加密需要配套的 Lanboat 服务端。"
			}
			fields = append(fields, withGroup("Shadowsocks 设置", method, secret("trojan-ss.password", "密码", true))...)
		}
		fields = append(fields, transportNodeFields(n)...)
		flow := choose("flow", "流控", "none", "none", "xtls-rprx-vision")
		flow.Hint = "Trojan Vision 需要配套的 Lanboat 服务端。"
		fields = append(fields, flow)
		fields = append(fields, tlsNodeFields(n, true, false, false, true)...)
		fields = append(fields, boolean("mux", "多路复用", "false"))
		fields = append(fields, pluginFields(n)...)
		fields = append(fields, boolean("tfo", "TCP 快速打开", "false"), boolean("udp", "UDP 转发", "true"))
	case "hysteria", "hysteria2":
		fields = []Field{secret("obfs-password", "混淆", false)}
		if n.Kind() == "hysteria" {
			fields[0].Key = "obfs"
			fields = append(fields, choose("protocol", "协议", "udp", "udp", "wechat-video"))
		}
		fields = append(fields, tlsNodeFields(n, true, true, true, false)...)
		fields = append(fields, boolean("udp", "UDP 转发", "true"), boolean("fast-open", "快速打开", "false"))
		speed := "100"
		if n.Kind() == "hysteria2" {
			speed = "auto"
		}
		fields = append(fields, placeholder(f("up", "上行速度", false), speed), placeholder(f("down", "下行速度", false), speed), placeholder(number("heartbeat", "心跳间隔", ""), "0"))
	default:
		fields = advancedNodeFields(n)
	}
	return fields
}

// Hidden sections remain editable later; build this allowlist once, not per node.
var knownFieldsOnce sync.Once
var knownFieldsByKind map[string][]Field

func knownNodeFields(n Node) []Field {
	if !DetailedProtocol(n.Kind()) {
		fields := NodeFields(n)
		if n.Kind() == "ss" {
			fields = append(fields, f("xhttp-extra", "XHTTP Extra JSON", false))
		}
		return fields
	}
	knownFieldsOnce.Do(func() {
		knownFieldsByKind = map[string][]Field{}
		for _, p := range Protocols {
			if !DetailedProtocol(p.ID) {
				continue
			}
			node := Node{Type: p.ID}
			node.SetOption("tls", true)
			node.SetOption("fragment", true)
			node.SetOption("mutual-auth", true)
			node.SetOption("trojan-protocol", "shadowsocks")
			node.SetOption("private-key-enabled", true)
			node.SetOption("http-mask", true)
			fields := combine(p.Fields, detailedNodeFields(node), tlsNodeFields(node, true, false, false, true), []Field{f("xhttp-extra", "XHTTP Extra JSON", false)})
			for _, obfs := range []string{"xt_wgobfs", "amneziawg"} {
				node.SetOption("obfs", obfs)
				fields = append(fields, advancedNodeFields(node)...)
			}
			for _, mode := range []string{"http", "tls"} {
				node.SetOption("obfs-opts.mode", mode)
				fields = append(fields, extraNodeFields(node)...)
			}
			node.SetOption("network", "ws")
			node.SetOption("lua-file", "custom")
			fields = append(fields, extraNodeFields(node)...)
			for _, network := range []string{"http", "mkcp", "grpc", "quic", "xhttp"} {
				node.SetOption("network", network)
				fields = append(fields, transportNodeFields(node)...)
			}
			for _, plugin := range SSPlugins {
				node.Plugin = plugin
				node.SetOption("plugin-opts.mode", "custom")
				node.SetOption("plugin-opts.Transport", "websocket")
				fields = append(fields, ssPluginFields(node)...)
			}
			seen := map[string]bool{}
			for _, field := range fields {
				if !seen[field.Key] {
					seen[field.Key] = true
					knownFieldsByKind[p.ID] = append(knownFieldsByKind[p.ID], field)
				}
			}
		}
	})
	return knownFieldsByKind[n.Kind()]
}

func NodeTLSEnabled(n Node) bool {
	switch n.Kind() {
	case "shadowtls", "socks5s", "https", "http2", "http3", "trojan", "hysteria", "hysteria2", "anytls", "tuic", "trusttunnel", "openconnect", "juicity":
		return true
	default:
		return n.TextOption("tls") == "true"
	}
}
