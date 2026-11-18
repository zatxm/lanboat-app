package engine

import (
	"context"
	"encoding/json"
	allinone "github.com/zatxm/lanboat-allinone"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestLanboatRuleSetsWithCore(t *testing.T) {
	core := ""
	dir := filepath.Join(t.TempDir(), "runtime")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	c := profile.Default()
	profile.EnsureFiles(&c)
	c.Settings.RuleRegion = "cn"
	p, err := profile.EffectiveRouting(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = routing.ResolveSets(context.Background(), &p, core, dir); err != nil {
		t.Fatal(err)
	}
	c.Routing = &p
	cfg := BuildConfig(c, "127.0.0.1:19090", "secret")
	cfg["outbounds"] = []any{map[string]any{"type": "direct", "tag": "direct"}, map[string]any{"type": "socks", "tag": "proxy", "server": "127.0.0.1", "server_port": 1081, "version": "5"}}
	b, _ := json.Marshal(cfg)
	if err := allinone.Check(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	match := func(p routing.Parsed, host string, ip netip.Addr) string {
		req := routing.Request{Host: host, IP: ip}
		for _, r := range p.Rules {
			hit := false
			if r.SetPath != "" {
				var e error
				hit, e = routing.MatchBinary(context.Background(), core, r.SetPath, req)
				if e != nil {
					t.Fatal(e)
				}
			} else {
				hit = routing.MatchRule(r, req)
			}
			if hit {
				return r.Action
			}
		}
		return p.Final
	}
	for _, tc := range []struct{ host, ip, want string }{{"www.baidu.com", "", "direct"}, {"", "223.5.5.5", "direct"}, {"", "192.168.1.2", "direct"}, {"", "8.8.8.8", "proxy"}} {
		ip, _ := netip.ParseAddr(tc.ip)
		if got := match(p, tc.host, ip); got != tc.want {
			t.Errorf("%s %s: %s", tc.host, tc.ip, got)
		}
	}
	c.Settings.RuleRegion = "us"
	p, err = profile.EffectiveRouting(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = routing.ResolveSets(context.Background(), &p, core, dir); err != nil {
		t.Fatal(err)
	}
	if got := match(p, "", netip.MustParseAddr("8.8.8.8")); got != "proxy" {
		t.Fatalf("Google rule must take priority over region: %s", got)
	}
	if got := match(p, "", netip.MustParseAddr("4.2.2.2")); got != "direct" {
		t.Fatalf("US region did not apply: %s", got)
	}
}
