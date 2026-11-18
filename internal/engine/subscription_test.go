package engine

import (
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestSubscriptionUDPAndFastOpenReachCores(t *testing.T) {
	c := profile.Default()
	n := profile.Node{ID: "sub", Type: "ss", Name: "sub", Server: "example.com", Port: 443, Method: "aes-256-gcm", Password: "secret"}
	n.SetOption("udp", false)
	n.SetOption("tfo", true)
	c.Nodes, c.Selected = []profile.Node{n}, n.ID
	sing := BuildConfig(c, "127.0.0.1:19090", "test")
	out := sing["outbounds"].([]any)[1].(map[string]any)
	if out["network"] != "tcp" || out["tcp_fast_open"] != true {
		t.Fatal("sing-box ignored subscription options")
	}
	found := false
	for _, raw := range sing["route"].(map[string]any)["rules"].([]any) {
		rule := raw.(map[string]any)
		if rule["action"] == "reject" && rule["network"] == "udp" {
			found = true
		}
	}
	if !found {
		t.Fatal("UDP forwarding still allowed")
	}
}
