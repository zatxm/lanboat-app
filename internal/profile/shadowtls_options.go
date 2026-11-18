package profile

import (
	"fmt"
	"strconv"
	"strings"
)

var ShadowTLSFingerprints = []string{"none", "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized", "Chrome150"}

// The standalone transport and Shadowsocks plugin use the same TLS options.
func shadowTLSFields(prefix string) []Field {
	key := func(s string) string { return prefix + s }
	var fields []Field
	if prefix != "" {
		port := number(key("port"), "端口", "")
		port.Required = true
		port.Placeholder = "1–65535"
		fields = []Field{placeholder(f(key("server"), "地址", true), "域名或IP"), port, secret(key("password"), "密码", false)}
	}
	fields = append(fields, choose(key("version"), "版本", "1", "1", "2", "3"))
	fields = append(fields, withGroup("TLS 设置",
		f(key("host"), "Host", false),
		placeholder(f(key("alpn"), "应用层协议协商", false), "逗号分隔，如 h2,http/1.1"),
		placeholder(multilineSecret(key("certificate"), "证书", false), "PEM 证书或本地文件路径"),
		boolean(key("skip-cert-verify"), "允许不安全", "false"),
		choose(key("client-fingerprint"), "uTLS 指纹", "none", ShadowTLSFingerprints...),
	)...)
	return fields
}

func ShadowTLSPrefix(n Node) string {
	if n.Kind() == "shadowtls" {
		return ""
	}
	return "plugin-opts."
}

func ValidateShadowTLSOptions(n Node) error {
	prefix := ShadowTLSPrefix(n)
	if host := n.TextOption(prefix + "host"); host != "" && !ValidHost(host) {
		return fmt.Errorf("ShadowTLS Host 需为域名或 IP")
	}
	if _, err := CertificatePEM(n.TextOption(prefix + "certificate")); err != nil {
		return err
	}
	alpn := n.TextOption(prefix + "alpn")
	if strings.ContainsAny(alpn, "\r\n\x00") {
		return fmt.Errorf("应用层协议协商不能包含换行或空字符")
	}
	if alpn != "" {
		total := 0
		for _, protocol := range strings.Split(alpn, ",") {
			protocol = strings.TrimSpace(protocol)
			if len(protocol) == 0 || len(protocol) > 255 || strings.ContainsAny(protocol, "\r\n\x00") {
				return fmt.Errorf("应用层协议协商需为逗号分隔的协议名，每项 1–255 字节")
			}
			total += len(protocol) + 1
		}
		if total > 65535 {
			return fmt.Errorf("应用层协议协商列表过长")
		}
	}
	if prefix != "" {
		if !ValidHost(n.TextOption(prefix + "server")) {
			return fmt.Errorf("ShadowTLS 地址需为域名或 IP")
		}
		port, err := strconv.Atoi(n.TextOption(prefix + "port"))
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("ShadowTLS 端口必须为 1–65535")
		}
	}
	if n.EffectiveOption(prefix+"version", "1") == "3" && n.TextOption(prefix+"client-fingerprint") == "android" {
		return fmt.Errorf("android 指纹仅支持 TLS 1.2，请在 ShadowTLS 3 中选择其他指纹")
	}
	return nil
}
