package profile

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/goccy/go-json"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
)

func validLegacyFingerprint(s string) bool {
	switch s {
	case "", "chrome", "firefox", "safari", "ios", "edge", "random", "randomized", "chrome_psk", "chrome_pq":
		return true
	}
	return false
}
func validateNodeOptions(n Node) error {
	if method := n.TextOption("transport-method"); method != "" {
		if _, err := http.NewRequest(method, "http://localhost/", nil); err != nil {
			return fmt.Errorf("HTTP 传输方法无效: %w", err)
		}
	}
	for _, key := range []string{"transport-host", "transport-path", "service-name"} {
		if strings.ContainsAny(n.TextOption(key), "\r\n\x00") {
			return fmt.Errorf("%s 含无效字符", key)
		}
	}
	for _, network := range []string{"ws", "h2", "http", "httpupgrade", "grpc"} {
		raw := n.Option(network + "-opts.headers")
		if raw == nil {
			continue
		}
		headers, ok := raw.(map[string]any)
		if text, isText := raw.(string); isText {
			if json.Unmarshal([]byte(text), &headers) != nil {
				return fmt.Errorf("%s 请求头格式无效", network)
			}
			ok = headers != nil
		}
		if !ok {
			return fmt.Errorf("%s 请求头格式无效", network)
		}
		if network == "grpc" && len(headers) > 0 {
			return fmt.Errorf("gRPC 传输不支持自定义 HTTP 请求头")
		}
		if err := validateImportedHeaders(headers); err != nil {
			return fmt.Errorf("%s 请求头：%w", network, err)
		}
	}
	if n.Kind() == "shadowtls" || n.Plugin == "shadow-tls" {
		if err := ValidateShadowTLSOptions(n); err != nil {
			return err
		}
	}
	if err := validateExtraOptions(n); err != nil {
		return err
	}
	if err := ValidateQUICTLSOptions(n); err != nil {
		return err
	}
	if n.Kind() == "openconnect" && n.TextOption("http2") == "true" {
		return fmt.Errorf("OpenConnect 的 CSTP 隧道需要 HTTP/1.1")
	}
	if n.Kind() == "snell" {
		version := n.EffectiveOption("version", "1")
		obfs := n.TextOption("obfs-opts.mode")
		if version == "6" {
			if len(n.Password) < 12 || len(n.Password) > 255 {
				return fmt.Errorf("Snell v6 密码需为 12–255 字节")
			}
			if (obfs != "" && obfs != "none") || n.TextOption("obfs-opts.host") != "" || n.TextOption("obfs-opts.path") != "" {
				return fmt.Errorf("Snell v6 使用模式选项，旧版混淆仅适用于 Snell 1–5")
			}
		} else if n.TextOption("mode") != "" {
			return fmt.Errorf("此模式仅适用于 Snell v6")
		}
		if (version == "4" || version == "5") && obfs == "tls" {
			return fmt.Errorf("Snell 4/5 不支持 TLS 混淆，请改用 HTTP 混淆或 Shadow-TLS 插件")
		}
		if (version == "1" || version == "3") && n.TextOption("mux") == "true" {
			return fmt.Errorf("Snell 1/3 不支持连接复用，请使用版本 2 或 4–6")
		}
	}
	if n.Kind() == "vless" {
		if _, err := ParseVLESSEncryption(n.TextOption("encryption")); err != nil {
			return err
		}
	}
	if n.Kind() == "anytls" || n.Kind() == "snell" {
		if _, _, _, err := n.IdleTimeout(); err != nil {
			return err
		}
	}
	if n.Kind() == "wireguard" || n.Kind() == "masque" {
		for _, key := range []string{"ip", "ipv6"} {
			if value := n.TextOption(key); value != "" {
				if _, err := netip.ParseAddr(strings.Split(value, "/")[0]); err != nil {
					return fmt.Errorf("%s 地址无效", key)
				}
			}
		}
		for _, value := range strings.Split(n.EffectiveOption("dns", "1.1.1.1"), ",") {
			if _, err := netip.ParseAddr(strings.TrimSpace(value)); err != nil {
				return fmt.Errorf("DNS 地址无效")
			}
		}
		if value := n.TextOption("mtu"); value != "" {
			mtu, err := strconv.Atoi(value)
			if err != nil || mtu < 576 || mtu > 65535 {
				return fmt.Errorf("MTU 需为 576–65535")
			}
		}
		if value := n.TextOption("reserved"); value != "" {
			parts := strings.Split(value, ",")
			if len(parts) != 3 {
				return fmt.Errorf("保留位需为三个 0–255 的整数")
			}
			for _, part := range parts {
				if _, err := strconv.ParseUint(strings.TrimSpace(part), 10, 8); err != nil {
					return fmt.Errorf("保留位需为三个 0–255 的整数")
				}
			}
		}
	}
	if n.Kind() == "wireguard" {
		if err := validateWireguardOptions(n); err != nil {
			return err
		}
	}
	for _, field := range NodeFields(n) {
		value := n.TextOption(field.Key)
		if field.Kind == "switch" && value != "" && value != "true" && value != "false" {
			return fmt.Errorf("%s选项无效", field.Title)
		}
	}
	for _, key := range []string{"up", "down"} {
		value := n.TextOption(key)
		if value == "auto" && n.Kind() == "hysteria" {
			return fmt.Errorf("Hysteria 速度必须为正整数")
		}
		if value == "" || value == "auto" {
			continue
		}
		i, err := strconv.Atoi(value)
		if err != nil || i < 1 || i > 1000000 {
			return fmt.Errorf("%s速度需为 1–1000000 或 auto", key)
		}
	}
	for _, key := range []string{"kcp.mtu", "kcp.tti", "kcp.uplink-capacity", "kcp.downlink-capacity", "heartbeat"} {
		if key == "heartbeat" && n.Kind() == "wireguard" {
			continue
		}
		if strings.HasPrefix(key, "kcp.") && n.TextOption("network") != "mkcp" {
			continue
		}
		value := n.TextOption(key)
		if value == "" {
			continue
		}
		i, err := strconv.Atoi(value)
		min, max := 0, 1000000
		if key == "kcp.mtu" {
			min, max = 576, 1460
		}
		if key == "kcp.tti" {
			min, max = 1, 1000
		}
		if err != nil || i < min || i > max {
			return fmt.Errorf("%s需在 %d–%d 之间", key, min, max)
		}
	}
	if NodeTLSEnabled(n) && n.TextOption("fragment") == "true" {
		for _, stem := range []string{"length", "interval"} {
			lo, hi := n.TextOption("fragment-opts."+stem+"-min"), n.TextOption("fragment-opts."+stem+"-max")
			if lo == "" && hi == "" {
				continue
			}
			a, ea := strconv.Atoi(lo)
			b, eb := strconv.Atoi(hi)
			min := 0
			if stem == "length" {
				min = 1
			}
			if ea != nil || eb != nil || a < min || b < a || b > 65535 {
				return fmt.Errorf("片段 %s 范围无效", stem)
			}
		}
		packet := n.TextOption("fragment-opts.packets")
		if packet != "" && packet != "tlshello" {
			a, b, ok := strings.Cut(packet, "-")
			lo, ea := strconv.Atoi(a)
			hi, eb := strconv.Atoi(b)
			if !ok || ea != nil || eb != nil || lo < 1 || hi < lo || hi > 1000 {
				return fmt.Errorf("片段数据包需为 tlshello 或正整数范围")
			}
		}
	}
	if NodeTLSEnabled(n) {
		if key := n.TextOption("reality-opts.public-key"); key != "" {
			value, err := base64.RawURLEncoding.DecodeString(key)
			if err != nil || len(value) != 32 {
				return fmt.Errorf("Reality 公钥需为 32 字节 Base64URL")
			}
		}
		if short := n.TextOption("reality-opts.short-id"); short != "" {
			value, err := hex.DecodeString(short)
			if err != nil || len(value) > 8 {
				return fmt.Errorf("短 ID 需为最多 16 位十六进制")
			}
		}
		for _, pin := range []string{"tls-sha256", "tls-pubkey-sha256"} {
			if hash := n.TextOption(pin); hash != "" {
				if _, err := DecodeSHA256(hash); err != nil {
					return err
				}
			}
		}
		if _, err := ClientCertificate(n); err != nil {
			return err
		}
	}
	if n.TextOption("network") == "xhttp" {
		if _, err := n.XHTTPExtraSettings(); err != nil {
			return err
		}
	}
	if n.Plugin != "" {
		return n.validateShadowsocks()
	}
	return nil
}
