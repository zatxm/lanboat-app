package profile

import (
	"fmt"
)

func (n Node) UsesQUIC() bool {
	switch n.Kind() {
	case "http3", "tuic", "juicity", "hysteria", "hysteria2":
		return true
	case "vmess", "vless":
		return n.TextOption("network") == "quic"
	case "trusttunnel":
		return n.TextOption("quic") == "true"
	case "masque":
		return n.TextOption("http2") != "true" && n.TextOption("network") != "h2"
	}
	return false
}
func ValidateQUICTLSOptions(n Node) error {
	if !n.UsesQUIC() {
		return nil
	}
	if n.TextOption("fragment") == "true" {
		return fmt.Errorf("QUIC 使用 UDP，不能应用 TCP TLS 片段")
	}
	if n.TextOption("reality-opts.public-key") != "" {
		return fmt.Errorf("QUIC 握手不支持 TCP Reality 认证格式")
	}
	return nil
}
