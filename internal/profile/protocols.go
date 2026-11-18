package profile

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

type Field struct {
	Key, Title, Kind, Default string
	Group, Placeholder, Hint  string
	Required, Secret          bool
	Choices                   []string
}
type Protocol struct {
	ID, Label, Core string
	Fields          []Field
}

func f(key, title string, required bool) Field {
	return Field{Key: key, Title: title, Required: required}
}
func secret(key, title string, required bool) Field {
	v := f(key, title, required)
	v.Secret = true
	return v
}
func multilineSecret(key, title string, required bool) Field {
	v := secret(key, title, required)
	v.Kind = "multiline"
	return v
}
func choose(key, title, def string, values ...string) Field {
	return Field{Key: key, Title: title, Kind: "choice", Default: def, Choices: values}
}
func number(key, title, def string) Field {
	return Field{Key: key, Title: title, Kind: "number", Default: def}
}

var tlsFields = []Field{f("sni", "TLS 服务器名称 SNI", false), choose("skip-cert-verify", "跳过证书验证", "false", "false", "true"), f("alpn", "ALPN（逗号分隔）", false), choose("client-fingerprint", "TLS 指纹", "", "", "chrome", "firefox", "safari", "ios", "edge", "random", "Chrome150")}
var transportFields = []Field{choose("network", "传输方式", "tcp", "tcp", "ws", "grpc", "h2", "http", "httpupgrade", "xhttp"), f("transport-host", "传输 Host", false), f("transport-path", "传输路径", false), f("service-name", "gRPC 服务名", false)}

func combine(groups ...[]Field) []Field {
	var result []Field
	for _, g := range groups {
		result = append(result, g...)
	}
	return result
}

var Protocols = []Protocol{
	{"ss", "Shadowsocks", "lanboat-allinone", nil},
	{"ssr", "ShadowsocksR", "lanboat-allinone", []Field{choose("cipher", "加密方式", "aes-256-cfb", Methods...), choose("protocol", "协议", "origin", SSRProtocols...), secret("protocol-param", "协议参数", false), choose("obfs", "混淆", "plain", SSRObfuscations...), f("obfs-param", "混淆参数", false)}},
	{"shadowtls", "ShadowTLS", "lanboat-allinone", shadowTLSFields("")},
	{"vmess", "VMess", "lanboat-allinone", combine([]Field{secret("uuid", "UUID", true), choose("cipher", "加密方式", "auto", "auto", "aes-128-gcm", "chacha20-poly1305", "none"), number("alterId", "Alter ID", "0"), choose("tls", "TLS", "false", "false", "true")}, transportFields, tlsFields)},
	{"vless", "VLESS", "lanboat-allinone", combine([]Field{secret("uuid", "UUID", true), choose("flow", "Flow", "", "", "xtls-rprx-vision"), choose("tls", "TLS", "true", "false", "true"), f("reality-opts.public-key", "REALITY 公钥", false), f("reality-opts.short-id", "REALITY Short ID", false)}, transportFields, tlsFields)},
	{"trojan", "Trojan", "lanboat-allinone", combine(transportFields, tlsFields)},
	{"socks5", "Socks5", "lanboat-allinone", []Field{f("username", "用户名", false)}},
	{"socks5s", "Socks5 Over TLS", "lanboat-allinone", combine([]Field{f("username", "用户名", false)}, tlsFields)},
	{"http", "HTTP", "lanboat-allinone", []Field{f("username", "用户名", false)}},
	{"https", "HTTPS", "lanboat-allinone", combine([]Field{f("username", "用户名", false), choose("naive", "NaiveProxy", "false", "false", "true")}, tlsFields)},
	{"http2", "HTTP2", "lanboat-allinone", combine([]Field{f("username", "用户名", false), f("path", "连接路径", false), choose("naive", "NaiveProxy", "false", "false", "true")}, tlsFields)},
	{"http3", "HTTP3", "lanboat-allinone", []Field{f("username", "用户名", true), f("sni", "服务器名称 SNI", false)}},
	{"hysteria", "Hysteria", "lanboat-allinone", combine([]Field{secret("obfs", "混淆密码", false), number("up", "上传 Mbps", "10"), number("down", "下载 Mbps", "50"), choose("protocol", "传输协议", "udp", "udp", "wechat-video", "faketcp")}, tlsFields)},
	{"hysteria2", "Hysteria2", "lanboat-allinone", combine([]Field{choose("obfs", "混淆", "", "", "salamander"), secret("obfs-password", "混淆密码", false), f("ports", "跳跃端口范围", false)}, tlsFields)},
	{"anytls", "AnyTLS", "lanboat-allinone", tlsFields},
	{"tuic", "TUIC", "lanboat-allinone", combine([]Field{secret("uuid", "UUID", false), secret("token", "TUIC v4 Token", false), choose("congestion-controller", "拥塞控制", "cubic", "cubic", "bbr", "new_reno"), choose("udp-relay-mode", "UDP 转发方式", "native", "native", "quic")}, tlsFields)},
	{"ssh", "SSH", "lanboat-allinone", []Field{f("username", "用户名", true), multilineSecret("private-key", "私钥 PEM（新输入可见，保存后隐藏）", false), secret("private-key-passphrase", "私钥口令", false), f("host-key", "主机公钥（推荐填写）", false)}},
	{"wireguard", "WireGuard", "lanboat-allinone", []Field{secret("private-key", "本地私钥", true), f("public-key", "对端公钥", true), secret("pre-shared-key", "预共享密钥", false), f("ip", "本地 IPv4", true), f("ipv6", "本地 IPv6", false), f("reserved", "Reserved（如 1,2,3）", false), number("mtu", "MTU", "1408")}},
	{"snell", "Snell", "lanboat-allinone", []Field{choose("version", "协议版本", "3", "1", "2", "3", "4", "5"), choose("obfs-opts.mode", "混淆方式", "", "", "http", "tls"), f("obfs-opts.host", "混淆 Host", false)}},
	{"mieru", "Mieru", "lanboat-allinone", []Field{f("username", "用户名", true), choose("transport", "传输协议", "TCP", "TCP", "UDP"), f("port-range", "端口范围", false), choose("multiplexing", "多路复用", "MULTIPLEXING_LOW", "MULTIPLEXING_OFF", "MULTIPLEXING_LOW", "MULTIPLEXING_MIDDLE", "MULTIPLEXING_HIGH")}},
	{"relay", "Relay", "lanboat-allinone", []Field{f("username", "用户名", false)}},
	{"trusttunnel", "TrustTunnel", "lanboat-allinone", combine([]Field{f("username", "用户名", true), choose("quic", "QUIC", "false", "false", "true")}, tlsFields)},
	{"masque", "MASQUE", "lanboat-allinone", combine([]Field{secret("private-key", "客户端私钥（Base64 DER）", true), f("public-key", "服务器公钥（Base64 DER）", true), f("ip", "本地 IPv4", true), f("ipv6", "本地 IPv6", false), f("uri", "连接 URI", false), choose("network", "传输协议", "h3", "h3", "h2", "h3-l4proxy")}, tlsFields)},
	{"sudoku", "Sudoku", "lanboat-allinone", extraNodeFields(Node{Type: "sudoku"})},
	{"naive", "NaiveProxy", "lanboat-allinone", extraNodeFields(Node{Type: "naive"})},
	{"openconnect", "OpenConnect", "lanboat-allinone", combine([]Field{f("username", "用户名", false), choose("flavor", "服务端类型", "anyconnect", "anyconnect", "gp", "fortinet", "f5", "pulse", "nc"), f("authgroup", "认证组", false), secret("cookie", "登录 Cookie", false)}, []Field{f("sni", "TLS 服务器名称 SNI", false), choose("skip-cert-verify", "跳过证书验证", "false", "false", "true")})},
	{"juicity", "Juicity", "lanboat-allinone", combine([]Field{secret("uuid", "UUID", true), f("pinned-certchain-sha256", "证书链 SHA256", false), choose("congestion-controller", "拥塞控制", "bbr", "bbr", "cubic", "new_reno")}, []Field{f("sni", "TLS 服务器名称 SNI", false), choose("skip-cert-verify", "跳过证书验证", "false", "false", "true")})},
	{"brook", "Brook", "lanboat-allinone", []Field{choose("network", "传输协议", "tcp", "tcp", "ws", "wss"), f("path", "WebSocket 路径", false)}},
	{"lua", "Lua", "lanboat-allinone", combine([]Field{f("username", "用户名", false), choose("tls", "TLS", "false", "false", "true"), multilineSecret("script", "Lua 脚本（公开 backend 回调 API）", true)}, tlsFields[:2])},
	{"tailscale", "Tailscale", "lanboat-allinone", extraNodeFields(Node{Type: "tailscale"})},
}

func (n Node) Kind() string {
	if n.Type == "" {
		return "ss"
	}
	return n.Type
}
func ProtocolByID(id string) (Protocol, bool) {
	if id == "" {
		id = "ss"
	}
	for _, p := range Protocols {
		if p.ID == id {
			if DetailedProtocol(id) {
				p.Fields = detailedNodeFields(Node{Type: id})
			}
			return p, true
		}
	}
	return Protocol{}, false
}

// Options contain JSON maps/lists and scalar values; copy mutable containers directly.
func cloneOption(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = cloneOption(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = cloneOption(item)
		}
		return result
	case []string:
		return append([]string(nil), value...)
	default:
		return value
	}
}
func (n Node) Clone() Node {
	if n.Options != nil {
		n.Options = cloneOption(n.Options).(map[string]any)
	}
	return n
}
func (n Node) Option(key string) any {
	var value any = n.Options
	for {
		part, rest, more := strings.Cut(key, ".")
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[part]
		if !more {
			return value
		}
		key = rest
	}
}
func (n Node) TextOption(key string) string {
	v := n.Option(key)
	if v == nil {
		return ""
	}
	if list, ok := v.([]string); ok {
		return strings.Join(list, ",")
	}
	if list, ok := v.([]any); ok {
		parts := make([]string, len(list))
		for i, x := range list {
			parts[i] = fmt.Sprint(x)
		}
		return strings.Join(parts, ",")
	}
	return fmt.Sprint(v)
}
func (n *Node) SetOption(key string, value any) {
	if n.Options == nil {
		n.Options = map[string]any{}
	}
	object := n.Options
	for {
		part, rest, more := strings.Cut(key, ".")
		if !more {
			object[part] = value
			return
		}
		next, ok := object[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			object[part] = next
		}
		object = next
		key = rest
	}
}
func SecretOption(key string) bool {
	for _, p := range Protocols {
		for _, field := range p.Fields {
			if field.Key == key && field.Secret {
				return true
			}
		}
	}
	return false
}
func (n Node) Masked() Node {
	n.DialProxy = nil
	n.TCP = nil
	original := n
	n.Options = nil
	n.Password = ""
	if len(original.Options) == 0 {
		return n
	}
	for _, field := range knownNodeFields(original) {
		if value := original.Option(field.Key); value != nil {
			if field.Secret {
				n.SetOption(field.Key, "")
			} else {
				n.SetOption(field.Key, cloneOption(value))
			}
		}
	}
	if n.Kind() == "lua" {
		n.SetOption("lua-file", original.LuaFile())
	}
	return n
}

func (n Node) validateProtocol() error {
	_, ok := ProtocolByID(n.Type)
	if !ok {
		return errors.New("未知节点协议")
	}
	if n.Kind() == "lua" {
		if _, err := n.LuaScript(); err != nil {
			return err
		}
	}
	if len(n.Password) > 4096 {
		return errors.New("密码过长")
	}
	needsPassword := map[string]bool{"ssr": true, "trojan": true, "hysteria2": true, "anytls": true, "snell": true, "lua": true, "mieru": true, "trusttunnel": true, "juicity": true, "brook": true, "naive": true, "http3": true}
	if needsPassword[n.Type] && n.Password == "" {
		return errors.New("此协议需要密码或密钥")
	}
	for _, field := range NodeFields(n) {
		value := n.TextOption(field.Key)
		if len(value) > 32768 {
			return fmt.Errorf("%s过长", field.Title)
		}
		if field.Required && strings.TrimSpace(value) == "" {
			return fmt.Errorf("请填写%s", field.Title)
		}
		if value == "" {
			continue
		}
		if field.Kind == "number" {
			i, err := strconv.Atoi(value)
			if err != nil || i < 0 || i > 1<<30 {
				return fmt.Errorf("%s必须是有效的非负整数", field.Title)
			}
		}
		if field.Kind == "choice" {
			valid := false
			for _, x := range field.Choices {
				valid = valid || x == value
			}
			if field.Key == "client-fingerprint" && validLegacyFingerprint(value) {
				valid = true
			}
			if field.Key == "flow" && value == "" {
				valid = true
			}
			if field.Key == "congestion-controller" && value == "new_reno" {
				valid = true
			}
			if n.Kind() == "mieru" && field.Key == "transport" && (value == "TCP" || value == "UDP") {
				valid = true
			}
			if n.Kind() == "brook" && field.Key == "network" && value == "tcp" {
				valid = true
			}
			if !valid {
				return fmt.Errorf("%s选项无效", field.Title)
			}
		}
	}
	if err := validateNodeOptions(n); err != nil {
		return err
	}
	for _, key := range []string{"uuid"} {
		if value := n.TextOption(key); value != "" {
			v := strings.ReplaceAll(value, "-", "")
			if len(v) != 32 {
				return errors.New("UUID 格式无效")
			}
			for _, c := range strings.ToLower(v) {
				if !strings.ContainsRune("0123456789abcdef", c) {
					return errors.New("UUID 格式无效")
				}
			}
		}
	}
	if n.TextOption("naive") == "true" && (n.Password == "" || n.TextOption("username") == "") {
		return errors.New("NaiveProxy 需要用户名和密码")
	}
	if n.TextOption("naive") == "true" && n.TextOption("skip-cert-verify") == "true" {
		return errors.New("NaiveProxy 使用系统证书校验，不支持跳过验证")
	}
	if n.Type == "tuic" && n.TUICVersion() == 5 && n.Password == "" {
		return errors.New("TUIC v5 需要密码")
	}
	if n.Type == "tuic" && n.TUICVersion() == 4 && n.Password == "" && n.TextOption("token") == "" {
		return errors.New("TUIC v4 需要密码 Token")
	}
	if n.Type == "openconnect" && n.TextOption("cookie") == "" && (n.Password == "" || n.TextOption("username") == "") {
		return errors.New("OpenConnect 需要用户名和密码，或登录 Cookie")
	}
	if n.Type == "ssh" && n.Password == "" && n.TextOption("private-key") == "" {
		return errors.New("SSH 需要密码或私钥")
	}
	if n.Type == "wireguard" {
		for _, key := range []string{"private-key", "public-key"} {
			b, err := base64.StdEncoding.DecodeString(n.TextOption(key))
			if err != nil || len(b) != 32 {
				return errors.New("WireGuard 密钥必须为 32 字节 Base64")
			}
		}
		if _, err := netip.ParseAddr(strings.Split(n.EffectiveOption("ip", "10.0.2.2"), "/")[0]); err != nil {
			return errors.New("本地 IP 地址无效")
		}
	}
	return nil
}
