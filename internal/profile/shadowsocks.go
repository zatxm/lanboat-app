package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const DefaultSSMethod = "aes-256-cfb"

var SSObfuscations = []string{"none", "http", "tls", "ssl", "h1", "h2", "wss", "websocket", "httpupgrade", "xhttp"}
var SSPlugins = []string{"none", "kcptun", "v2ray-plugin", "cloak", "gost", "shadow-tls"}
var SSRObfuscations = []string{"plain", "http_simple", "http_post", "random_head", "tls1.2_ticket_auth", "tls1.2_ticket_fastauth"}
var SSRProtocols = []string{"origin", "verify_sha1", "verify_deflate", "auth_sha1", "auth_sha1_v2", "auth_sha1_v4", "auth_aes128_md5", "auth_aes128_sha1", "auth_chain_a", "auth_chain_b", "auth_chain_c", "auth_chain_d", "auth_chain_e", "auth_chain_f"}

func boolean(key, title, def string) Field {
	return Field{Key: key, Title: title, Kind: "switch", Default: def}
}

var ssBaseFields = []Field{boolean("ota", "一次性认证", "false"), boolean("tfo", "TCP 快速打开", "false"), boolean("udp", "UDP 转发", "true"), choose("uot-version", "UDP Over TCP", "0", "0", "1", "2"), choose("obfs", "混淆", "none", SSObfuscations...)}

func NodeFields(n Node) []Field {
	if n.Kind() == "ss" || n.Kind() == "ssr" {
		return ShadowsocksFields(n)
	}
	if DetailedProtocol(n.Kind()) {
		return detailedNodeFields(n)
	}
	p, _ := ProtocolByID(n.Kind())
	return p.Fields
}

func ShadowsocksFields(n Node) []Field {
	fields := ssBaseFields
	if n.Kind() == "ssr" {
		fields = combine(fields[:4], []Field{choose("protocol", "协议", "origin", SSRProtocols...), secret("protocol-param", "协议参数", false), choose("obfs", "混淆", "plain", SSRObfuscations...)})
		if obfs := n.TextOption("obfs"); obfs != "" && obfs != "plain" && obfs != "random_head" {
			fields = append(fields, f("obfs-param", "混淆参数", false))
		}
		return append(fields, ssPluginFields(n)...)
	}
	obfs := n.TextOption("obfs")
	if obfs != "" && obfs != "none" {
		switch obfs {
		case "http", "h1", "h2", "wss", "websocket", "httpupgrade", "xhttp":
			fields = append(fields, Field{Key: "obfs-path", Title: "路径", Default: "/"})
		}
		fields = append(fields, f("obfs-host", "Host", false))
		if obfs == "xhttp" {
			fields = append(fields, choose("xhttp-mode", "XHTTP 模式", "auto", "auto", "packet-up", "stream-up", "stream-one"))
			fields = append(fields, xhttpFields()...)
		}
	}
	return append(fields, ssPluginFields(n)...)
}

func SSPluginFields(plugin string) []Field {
	return ssPluginFields(Node{Plugin: plugin})
}

func ssPluginFields(n Node) []Field {
	key := func(s string) string { return "plugin-opts." + s }
	if n.Plugin == "" || n.Plugin == "none" || n.Plugin == "obfs-local" {
		return nil
	}
	if n.Plugin == "shadow-tls" {
		return shadowTLSFields("plugin-opts.")
	}
	fields := []Field{f(key("server"), "插件地址", false), number(key("port"), "插件端口", "")}
	text := func(k, title, def string) Field { return Field{Key: key(k), Title: title, Default: def} }
	var extra []Field
	switch n.Plugin {
	case "kcptun":
		extra = []Field{boolean(key("nocomp"), "禁用压缩", "true"), choose(key("crypt"), "加密", "aes", "none", "aes", "aes-128", "aes-192", "aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "salsa20", "blowfish", "twofish", "cast5", "3des", "tea", "xtea", "xor"), choose(key("mode"), "模式", "fast", "normal", "fast", "fast2", "fast3", "custom"), choose(key("version"), "版本", "1", "1", "2"), secret(key("key"), "密码", false), number(key("mtu"), "MTU", "1350"), number(key("sndwnd"), "发送窗口", "1024"), number(key("rcvwnd"), "接收窗口", "1024"), number(key("datashard"), "数据碎片", "10"), number(key("parityshard"), "奇偶碎片", "3"), number(key("dscp"), "DSCP", "0"), number(key("keepalive"), "心跳间隔", "10"), number(key("autoexpire"), "自动过期", "0")}
		if n.TextOption(key("mode")) == "custom" {
			extra = append(extra, number(key("nodelay"), "NoDelay", "0"), number(key("interval"), "间隔（毫秒）", "50"), number(key("resend"), "快速重传", "0"), boolean(key("nc"), "禁用拥塞控制", "false"))
		}
	case "v2ray-plugin":
		extra = []Field{choose(key("mode"), "模式", "websocket", "websocket", "http2", "mkcp", "quic", "grpc"), text("host", "Host", "cloudfront.com"), text("path", "路径", "/"), boolean(key("tls"), "TLS", "false"), boolean(key("tfo"), "TCP 快速打开", "false"), boolean(key("mux"), "多路复用", "true")}
	case "cloak":
		extra = []Field{choose(key("EncryptionMethod"), "加密方式", "chacha20-poly1305", "plain", "aes-gcm", "chacha20-poly1305"), choose(key("BrowserSig"), "浏览器", "chrome", "chrome", "firefox"), choose(key("Transport"), "传输方式", "direct", "direct", "cdn", "websocket"), text("ProxyMethod", "代理方式", "/"), text("ServerName", "服务器名称", "")}
		if n.TextOption(key("Transport")) == "websocket" {
			extra = append(extra, boolean(key("skip-cert-verify"), "允许不安全", "false"), text("sni", "SNI", ""))
		}
		extra = append(extra, secret(key("UID"), "UID", true), secret(key("PublicKey"), "公钥", true), number(key("StreamTimeout"), "流超时", "300"), text("AlternativeNames", "替代名称", ""))
	case "gost":
		extra = []Field{choose(key("mode"), "类型", "mws", "mws", "mwss", "mtls", "ws", "wss", "tls"), boolean(key("skip-cert-verify"), "允许不安全", "false"), text("sni", "SNI", ""), text("path", "路径", "/ws"), text("host", "Host", "")}
	}
	return append(fields, extra...)
}

func (n Node) validateShadowsocks() error {
	n = n.Clone()
	NormalizeSS(&n)
	if n.Plugin == "shadow-tls" {
		if err := ValidateShadowTLSOptions(n); err != nil {
			return err
		}
	}
	if n.Plugin != "" && n.Plugin != "obfs-local" && !slices.Contains(SSPlugins, n.Plugin) {
		return errors.New("无效的 Shadowsocks 插件")
	}
	if len(n.PluginOpts) > 4096 {
		return errors.New("插件参数过长")
	}
	for _, field := range NodeFields(n) {
		value := n.TextOption(field.Key)
		if n.Kind() == "ssr" && (field.Key == "obfs" || field.Key == "protocol") {
			value = strings.TrimSuffix(value, "_compatible")
		}
		if field.Required && value == "" {
			return fmt.Errorf("%s不能为空", field.Title)
		}
		if value == "" {
			continue
		}
		if len(value) > 32768 {
			return fmt.Errorf("%s参数过长", field.Title)
		}
		if field.Kind == "choice" && !slices.Contains(field.Choices, value) {
			return fmt.Errorf("%s选项无效", field.Title)
		}
		if field.Kind == "switch" && value != "true" && value != "false" {
			return fmt.Errorf("%s选项无效", field.Title)
		}
		if field.Kind == "number" {
			number, err := strconv.Atoi(value)
			if err != nil || number < 0 || number > 1<<30 {
				return fmt.Errorf("%s必须为有效的非负整数", field.Title)
			}
			if field.Key == "plugin-opts.port" && (number < 1 || number > 65535) {
				return errors.New("插件端口必须为 1–65535")
			}
			if field.Key == "plugin-opts.mtu" && (number < 576 || number > 1500) {
				return errors.New("KCP MTU 必须为 576–1500")
			}
			if field.Key == "plugin-opts.dscp" && number > 63 {
				return errors.New("DSCP 必须为 0–63")
			}
			if (field.Key == "plugin-opts.sndwnd" || field.Key == "plugin-opts.rcvwnd") && (number < 1 || number > 65535) {
				return errors.New("KCP 窗口必须为 1–65535")
			}
			if (field.Key == "plugin-opts.datashard" || field.Key == "plugin-opts.parityshard") && number > 255 {
				return errors.New("KCP 分片不能超过 255")
			}
		}
		if field.Key == "plugin-opts.server" && !ValidHost(value) {
			return errors.New("插件地址必须为域名或 IP")
		}
		if (strings.HasSuffix(field.Key, "host") || strings.HasSuffix(field.Key, "path") || field.Key == "plugin-opts.sni" || field.Key == "plugin-opts.ServerName") && strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s不能包含换行", field.Title)
		}
		if strings.HasSuffix(field.Key, "headers") || field.Key == "xhttp-extra" {
			if _, ok := n.Option(field.Key).(string); ok {
				var object map[string]any
				if json.Unmarshal([]byte(value), &object) != nil || object == nil {
					return fmt.Errorf("%s必须是 JSON 对象", field.Title)
				}
			}
		}
	}
	if n.TextOption("obfs") == "xhttp" {
		if _, err := n.XHTTPExtraSettings(); err != nil {
			return err
		}
	}
	return nil
}

// Convert older SIP003 options to the same fields used by the native form.
func NormalizeSS(n *Node) {
	if n.Kind() != "ss" && n.Kind() != "ssr" {
		return
	}
	if n.Kind() == "ssr" && n.Method == "" {
		n.Method = n.TextOption("cipher")
	}
	if n.Kind() == "ssr" {
		for _, key := range []string{"protocol", "obfs"} {
			value := n.TextOption(key)
			if strings.HasSuffix(value, "_compatible") {
				n.SetOption(key, strings.TrimSuffix(value, "_compatible"))
			}
		}
	}
	if n.Plugin == "none" {
		n.Plugin = ""
	}
	if n.Plugin == "v2ray-plugin" && n.TextOption("plugin-opts.mode") == "fast" {
		n.SetOption("plugin-opts.mode", "websocket")
	}
	if (n.Plugin == "gost" || n.Plugin == "gost-plugin") && n.TextOption("plugin-opts.mode") == "websocket" {
		n.SetOption("plugin-opts.mode", "ws")
	}
	if n.Plugin == "gost-plugin" {
		n.Plugin = "gost"
	}
	setMissing := func(key string, value any) {
		if n.Option(key) == nil {
			n.SetOption(key, value)
		}
	}
	for _, item := range strings.Split(n.PluginOpts, ";") {
		key, value, found := strings.Cut(item, "=")
		if !found {
			if key == "tls" || key == "mux" {
				setMissing("plugin-opts."+key, true)
			}
			continue
		}
		switch key {
		case "obfs":
			setMissing("obfs", value)
		case "obfs-host":
			setMissing("obfs-host", value)
		default:
			setMissing("plugin-opts."+key, value)
		}
	}
	if n.Plugin == "shadow-tls" {
		setMissing("plugin-opts.server", n.Server)
		setMissing("plugin-opts.port", n.Port)
		setMissing("plugin-opts.password", n.Password)
	}
	if n.Plugin == "obfs-local" {
		n.Plugin = ""
	}
	for _, key := range []string{"obfs-headers", "plugin-opts.headers", "xhttp-extra"} {
		if value, ok := n.Option(key).(map[string]any); ok {
			b, _ := json.Marshal(value)
			n.SetOption(key, string(b))
		}
	}
}
