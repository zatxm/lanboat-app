package profile

import (
	"fmt"
	"strconv"
	"strings"
)

type UDPSettings struct {
	Relay     *bool  `json:"relay,omitempty"`
	BlockSTUN bool   `json:"blockSTUN,omitempty"`
	Ports     string `json:"ports,omitempty"`
	Timeout   int    `json:"timeout,omitempty"`
}

func (s UDPSettings) RelayEnabled() bool { return s.Relay == nil || *s.Relay }
func (s UDPSettings) Seconds() int {
	if s.Timeout == 0 {
		return 30
	}
	return s.Timeout
}
func (s UDPSettings) PortRanges() ([]string, error) {
	raw := strings.TrimSpace(s.Ports)
	if raw == "" || raw == "0" {
		return nil, nil
	}
	var out []string
	for _, v := range strings.Split(strings.ReplaceAll(raw, "，", ","), ",") {
		v = strings.TrimSpace(v)
		a, b, has := strings.Cut(strings.ReplaceAll(v, ":", "-"), "-")
		if !has {
			b = a
		}
		lo, e := strconv.Atoi(a)
		hi, e2 := strconv.Atoi(b)
		if e != nil || e2 != nil || lo < 1 || hi > 65535 || lo > hi {
			return nil, fmt.Errorf("运行端口需为1–65535、端口范围或逗号分隔的列表；0表示全部禁止")
		}
		out = append(out, fmt.Sprintf("%d:%d", lo, hi))
	}
	return out, nil
}
func (s UDPSettings) Validate() error {
	if s.Timeout < 0 || s.Timeout > 3600 {
		return fmt.Errorf("UDP超时需在1–3600秒之间")
	}
	_, err := s.PortRanges()
	return err
}
func (s UDPSettings) DirectOnly(kind string) bool {
	if !s.RelayEnabled() {
		return true
	}
	switch kind {
	case "ss", "ssr", "socks5", "socks5s", "vmess", "hysteria", "hysteria2", "relay", "trojan", "vless":
		return false
	}
	return true
}
