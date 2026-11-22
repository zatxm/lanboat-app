package tunmode

import (
	"runtime"
	"strconv"
)

const Interface = "lanboat-tun0"
const Table = 20245
const RuleStart = 9045
const RuleEnd = 9065

// Inbound matches the system device opened by the privileged desktop broker.
func Inbound(seconds int) map[string]any {
	in := map[string]any{
		"type": "tun", "tag": "tun-in", "address": []string{"172.29.45.1/30", "fdfe:29:45::1/126"},
		"mtu": 1500, "auto_route": true, "strict_route": true, "dns_mode": "native",
		"udp_timeout": strconv.Itoa(seconds) + "s",
	}
	switch runtime.GOOS {
	case "linux":
		in["interface_name"], in["dns_mode"] = Interface, "routed"
		in["iproute2_table_index"], in["iproute2_rule_index"] = Table, RuleStart
	case "windows":
		in["interface_name"] = "Lanboat"
	}
	return in
}
