package routing

import (
	"net/netip"
	"strings"
	"testing"
)

func TestDefaultAndPriority(t *testing.T) {
	p, err := Parse(DefaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rules) != 13 || p.Final != "proxy" || p.Rules[11].Value != "lanboat://geosite/region" || p.Rules[12].Value != "lanboat://geoip/region" {
		t.Fatalf("unexpected default rules: %+v", p.Rules)
	}
	for i, name := range []string{"cloudflare", "cloudfront", "facebook", "fastly", "google", "netflix", "telegram", "twitter", "tor"} {
		if p.Rules[i+2].Value != "lanboat://geoip/"+name || p.Rules[i+2].Action != "proxy" {
			t.Fatalf("unexpected provider rule: %+v", p.Rules[i+2])
		}
	}
	p, err = Parse("[Rule]\nDOMAIN-SUFFIX,baidu.com,DIRECT\nIP-CIDR,192.168.0.0/16,DIRECT\nFINAL,PROXY")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ host, ip, action string }{{"baidu.com", "", "direct"}, {"google.com", "", "proxy"}, {"foo.baidu.com", "", "direct"}, {"nomatch.invalid", "192.168.2.3", "direct"}, {"nomatch.invalid", "", "proxy"}}
	for _, c := range cases {
		ip, _ := netip.ParseAddr(c.ip)
		if got, _ := p.Match(c.host, ip); got != c.action {
			t.Errorf("%s %s: %s", c.host, c.ip, got)
		}
	}
	p, err = Parse("[Rule]\nDOMAIN,first.example,DIRECT\nUSER-AGENT,Line*,REJECT\nFINAL,PROXY")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.RequestPolicy("first.example", "Line 1"); ok {
		t.Fatal("UA bypassed earlier domain rule")
	}
	if action, ok := p.RequestPolicy("other.example", "Line 1"); !ok || action != "reject" {
		t.Fatal("UA not applied")
	}
	for _, kind := range []string{"GEOIP", "IP-ASN"} {
		value := "CN"
		if kind == "IP-ASN" {
			value = "132203"
		}
		prefix := netip.MustParsePrefix(Prefixes(kind, value)[0])
		p, _ = Parse("[Rule]\n" + kind + "," + value + ",DIRECT\nFINAL,REJECT")
		if got, _ := p.Match("", prefix.Addr()); got != "direct" {
			t.Fatal(kind)
		}
	}
}
func TestInvalidConfig(t *testing.T) {
	for _, s := range []string{"[Rule]\nDOMAIN,a,bad=policy", "[Rule]\nGEOIP,ZZ,DIRECT", "[Rule]\nIP-ASN,0,DIRECT", "[Rule]\nIP-CIDR,xyz,DIRECT", "[Rule]\nSCRIPT,x,DIRECT", "[URL Rewrite]\n[ https://a 302", strings.Repeat("x", MaxConfig+1)} {
		if _, err := Parse(s); err == nil {
			t.Fatalf("accepted %.50s", s)
		}
	}
}

func TestUserAgentRespectsEarlierBinarySet(t *testing.T) {
	p := Parsed{Rules: []Rule{{Type: "RULE-SET", Value: "lanboat://geosite/cn", SetPath: "fixture.srs", Action: "direct"}, {Type: "USER-AGENT", Value: "Example*", Action: "reject"}}}
	for _, tc := range []struct{ hit, want bool }{{true, false}, {false, true}} {
		action, ok := p.HTTPPolicyWithMatcher("example.com", 80, "Example/1", func(_ Rule, _ Request) (bool, error) { return tc.hit, nil })
		if ok != tc.want || ok && action != "reject" {
			t.Fatalf("hit %v: %s %v", tc.hit, action, ok)
		}
	}
	p.Rules[0] = Rule{Type: "GEOIP", Value: "CN", SetPath: "fixture.srs", Action: "direct"}
	if _, ok := p.HTTPPolicyWithMatcher("example.com", 80, "Example/1", func(_ Rule, _ Request) (bool, error) { t.Fatal("must defer DNS-dependent IP rule"); return false, nil }); ok {
		t.Fatal("IP rule bypassed")
	}
}
