package tunmode

import (
	"runtime"
	"testing"
)

func TestDesktopTUNInbound(t *testing.T) {
	in := Inbound(30)
	if in["tag"] != "tun-in" || in["auto_route"] != true || in["strict_route"] != true || in["udp_timeout"] != "30s" {
		t.Fatal(in)
	}
	if runtime.GOOS == "linux" && (in["interface_name"] != Interface || in["dns_mode"] != "routed" || in["iproute2_table_index"] != Table) {
		t.Fatal(in)
	}
}
