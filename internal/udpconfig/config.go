package udpconfig

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"strings"
)

// Rules are shared by the public proxy and the system TUN, before user routing.
func Rules(s profile.UDPSettings, direct bool) []any {
	rules := []any{}
	if strings.TrimSpace(s.Ports) == "0" {
		rules = append(rules, map[string]any{"network": "udp", "action": "reject"})
	} else if ranges, _ := s.PortRanges(); len(ranges) > 0 {
		rules = append(rules, map[string]any{"type": "logical", "mode": "and", "rules": []any{map[string]any{"network": "udp"}, map[string]any{"port_range": ranges, "invert": true}}, "action": "reject"})
	}
	if s.BlockSTUN {
		rules = append(rules, map[string]any{"network": "udp", "action": "sniff", "sniffer": []string{"stun"}}, map[string]any{"network": "udp", "protocol": []string{"stun"}, "action": "reject"})
	}
	if direct {
		rules = append(rules, map[string]any{"network": "udp", "action": "route", "outbound": "direct"})
	}
	return rules
}
