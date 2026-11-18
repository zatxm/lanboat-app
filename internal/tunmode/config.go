package tunmode

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/udpconfig"
)

const Interface = "lanboat-tun0"
const Table = 20245
const RuleStart = 9045
const RuleEnd = 9065

type Request struct {
	DNSPort       int                 `json:"dnsPort"`
	ProxyUsername string              `json:"proxyUsername,omitempty"`
	ProxyPassword string              `json:"proxyPassword,omitempty"`
	UDP           profile.UDPSettings `json:"udp,omitzero"`
	ProxyPort     int                 `json:"proxyPort"`
	ControlPort   int                 `json:"controlPort"`
	Secret        string              `json:"secret"`
	OwnerPID      int                 `json:"ownerPid"`
}

func (r Request) Validate() error {
	if len(r.ProxyUsername) > 255 || len(r.ProxyPassword) > 255 || strings.ContainsAny(r.ProxyUsername+r.ProxyPassword, "\x00\r\n") {
		return fmt.Errorf("invalid proxy authentication")
	}
	if err := r.UDP.Validate(); err != nil {
		return err
	}
	if r.DNSPort < 1024 || r.DNSPort > 65535 || r.DNSPort == r.ProxyPort || r.DNSPort == r.ControlPort {
		return fmt.Errorf("invalid TUN DNS port")
	}
	if r.ProxyPort < 1024 || r.ProxyPort > 65535 || r.ControlPort < 1024 || r.ControlPort > 65535 || r.ProxyPort == r.ControlPort {
		return fmt.Errorf("invalid TUN proxy or control port")
	}
	if len(r.Secret) < 32 || len(r.Secret) > 128 || strings.ContainsAny(r.Secret, "\r\n\x00") {
		return fmt.Errorf("invalid TUN session secret")
	}
	if r.OwnerPID < 2 {
		return fmt.Errorf("invalid TUN owner")
	}

	return nil
}

func (r Request) Config(ownerPath string) map[string]any {
	paths := []string{ownerPath}
	rules := []any{map[string]any{"process_path": paths, "action": "route", "outbound": "direct"}, map[string]any{"action": "sniff"}, map[string]any{"protocol": "dns", "action": "hijack-dns"}}
	rules = append(rules[:1], append(udpconfig.Rules(r.UDP, !r.UDP.RelayEnabled()), rules[1:]...)...)
	out := map[string]any{
		"log":          map[string]any{"level": "warn"},
		"inbounds":     []any{map[string]any{"type": "tun", "tag": "tun-in", "interface_name": Interface, "address": []string{"172.29.45.1/30", "fdfe:29:45::1/126"}, "mtu": 1500, "udp_timeout": strconv.Itoa(r.UDP.Seconds()) + "s", "auto_route": true, "strict_route": true, "dns_mode": "hijack", "iproute2_table_index": Table, "iproute2_rule_index": RuleStart}},
		"outbounds":    []any{map[string]any{"type": "direct", "tag": "direct"}, map[string]any{"type": "socks", "tag": "proxy", "server": "127.0.0.1", "server_port": r.ProxyPort, "version": "5", "username": r.ProxyUsername, "password": r.ProxyPassword}},
		"route":        map[string]any{"auto_detect_interface": true, "find_process": true, "rules": rules, "final": "proxy"},
		"dns":          map[string]any{"servers": []any{map[string]any{"type": "tcp", "tag": "gui-dns", "server": "127.0.0.1", "server_port": r.DNSPort}}, "final": "gui-dns", "disable_cache": true},
		"experimental": map[string]any{"clash_api": map[string]any{"external_controller": "127.0.0.1:" + strconv.Itoa(r.ControlPort), "secret": r.Secret, "access_control_allow_origin": []string{}}},
	}

	return out
}
