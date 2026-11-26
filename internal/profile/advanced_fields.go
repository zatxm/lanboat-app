package profile

import (
	"fmt"
	"strconv"
	"strings"
)

func NodePasswordField(kind string) bool {
	return kind != "vmess" && kind != "vless" && kind != "wireguard" && kind != "masque" && kind != "sudoku" && kind != "tailscale"
}

func advancedNodeFields(n Node) []Field {
	congestion := func() Field {
		return choose("congestion-controller", "拥塞控制", "none", "none", "reno", "cubic", "bbr")
	}
	udp := func(enabled string) Field { return boolean("udp", "UDP 转发", enabled) }
	heartbeat := func() Field { return placeholder(number("heartbeat", "心跳间隔", ""), "30") }
	var fields []Field
	switch n.Kind() {
	case "anytls":
		fields = []Field{placeholder(f("idle-timeout", "空闲超时", false), "30,30,1")}
		fields = append(fields, tlsNodeFields(n, true, false, false, true)...)
		fields = append(fields, udp("true"), boolean("tfo", "快速打开", "false"))
	case "tuic", "juicity":
		if n.Kind() == "tuic" {
			fields = append(fields, choose("version", "版本", strconv.Itoa(n.TUICVersion()), "4", "5"))
		}
		fields = append(fields, secret("uuid", "UUID", n.TextOption("token") == ""), congestion())
		fields = append(fields, tlsNodeFields(n, true, true, true, false)...)
		fields = append(fields, udp("false"))
	case "ssh":
		fields = []Field{f("username", "用户", false), boolean("private-key-enabled", "私钥", strconv.FormatBool(n.SSHPrivateKeyEnabled()))}
		if n.SSHPrivateKeyEnabled() {
			fields = append(fields, withGroup("私钥设置", f("private-key-name", "名称", false), secret("private-key-passphrase", "密码", false), multilineSecret("private-key", "私钥", true))...)
		}
		udpField := udp("false")
		udpField.Hint = "UDP 转发需要 Lanboat SSH 服务端；普通 OpenSSH 仅支持 TCP 转发。"
		fields = append(fields, f("host-key", "指纹", false), heartbeat(), udpField)
	case "wireguard", "masque":
		fields = []Field{secret("private-key", "私钥", true), f("public-key", "公钥", true)}
		if n.Kind() == "wireguard" {
			fields = append(fields, secret("pre-shared-key", "预共享密钥", false))
		}
		fields = append(fields, placeholder(f("ip", "子网 IP", false), "10.0.2.2"), placeholder(f("dns", "DNS", false), "1.1.1.1"), placeholder(number("mtu", "MTU", ""), "1420"))
		if n.Kind() == "masque" {
			fields = append(fields, f("sni", "SNI", false), heartbeat(), boolean("http2", "HTTP2", "false"), congestion())
		} else {
			keepalive := heartbeat()
			obfs := choose("obfs", "混淆", "none", "none", "xt_wgobfs", "amneziawg")
			if n.TextOption("obfs") == "amneziawg" {
				keepalive.Kind = ""
				obfs.Hint = "支持 AmneziaWG 3.1；S1–S4、H1–H4 和头部保护密钥必须与服务端一致。心跳间隔可填写范围。"
			}
			fields = append(fields, keepalive, textDefault("reserved", "保留位", "0,0,0"), obfs)
			switch n.TextOption("obfs") {
			case "xt_wgobfs":
				fields = append(fields, withGroup("混淆设置", secret("obfs-password", "密码", false))...)
			case "amneziawg":
				var extra []Field
				for _, key := range AmneziaPacketOptions {
					title := strings.ToUpper(key)
					switch key[0] {
					case 'j':
						title = "垃圾数据包 " + title
					case 's':
						title = "消息填充 " + title
					case 'h':
						title = "消息头部 " + title
					case 'i':
						title = "自定义签名数据包 " + title
					}
					extra = append(extra, f("amnezia-wg-option."+key, title, false))
				}
				extra = append(extra, secret("amnezia-wg-option.header-protection-key", "头部保护密钥", false))
				for _, param := range []struct{ key, title string }{
					{"content-padding-addition", "内容填充范围"}, {"rekey-after-time", "重新握手间隔范围"},
					{"rekey-timeout", "握手超时范围"}, {"reject-after-time", "连接失效时间范围"},
					{"keepalive-timeout", "保活超时范围"}, {"max-handshake-attempts", "握手重试次数范围"},
				} {
					extra = append(extra, f("amnezia-wg-option."+param.key, param.title, false))
				}
				extra = append(extra, boolean("amnezia-wg-option.random-trailers", "随机尾部填充", "false"), boolean("amnezia-wg-option.disable-cookies", "禁用 Cookie 回复", "false"))
				fields = append(fields, withGroup("混淆设置", extra...)...)
			}
		}
		fields = append(fields, udp("true"))
	case "openconnect", "trusttunnel":
		fields = []Field{f("username", "用户", false)}
		fields = append(fields, tlsNodeFields(n, true, false, false, false)...)
		if n.Kind() == "openconnect" {
			fields = append(fields, placeholder(f("authgroup", "分组", false), "all"))
		}
		fields = append(fields, udp("true"))
	default:
		fields = extraNodeFields(n)
	}
	return fields
}

func (n Node) SSHPrivateKeyEnabled() bool {
	return n.TextOption("private-key-enabled") == "true" || n.Option("private-key-enabled") == nil && n.TextOption("private-key") != ""
}

func (n Node) TUICVersion() int {
	if n.TextOption("version") == "4" || n.TextOption("version") == "" && n.TextOption("token") != "" {
		return 4
	}
	return 5
}

// Values displayed as placeholders are resolved at connection time, never persisted.
func (n Node) EffectiveOption(key, fallback string) string {
	if value := strings.TrimSpace(n.TextOption(key)); value != "" {
		return value
	}
	return fallback
}

func (n Node) IdleTimeout() (check, timeout, minimum int, err error) {
	values := strings.Split(n.EffectiveOption("idle-timeout", "30,30,1"), ",")
	if n.Kind() == "snell" && len(values) == 1 {
		values = []string{"30", values[0], "1"}
	}
	if len(values) != 3 {
		return 0, 0, 0, fmt.Errorf("空闲超时需为检查间隔,超时秒数,最少空闲连接数")
	}
	parsed := [3]int{}
	for i, value := range values {
		parsed[i], err = strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed[i] < 0 || parsed[i] > 86400 || i < 2 && parsed[i] == 0 || i == 2 && parsed[i] > 1000 {
			return 0, 0, 0, fmt.Errorf("空闲超时参数无效")
		}
	}
	return parsed[0], parsed[1], parsed[2], nil
}
