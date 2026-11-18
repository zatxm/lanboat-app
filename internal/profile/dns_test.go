package profile

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDNSSettingsValidation(t *testing.T) {
	s := DefaultDNS()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"system", "1.1.1.1", "127.0.0.1:5353", "udp://[::1]:53", "https://dns.example/dns-query", "tls://dns.example:853", "quic://dns.example", "h3://dns.example/dns-query"} {
		if _, err := ParseDNSAddress(address); err != nil {
			t.Fatal(address, err)
		}
	}
	for _, address := range []string{"", "https://user:pass@dns.example/dns-query", "udp://1.1.1.1:65536", "file:///etc/passwd", "udp://dns.example/wrong", "https://dns.example/?token=secret", "0.0.0.0"} {
		if _, err := ParseDNSAddress(address); err == nil {
			t.Fatal("invalid address accepted", address)
		}
	}
	for _, change := range []func(*DNSSettings){func(d *DNSSettings) { d.Bootstrap = "https://recursive.example/dns-query" }, func(d *DNSSettings) { d.Proxy = "system" }, func(d *DNSSettings) { d.Final = "unknown" }, func(d *DNSSettings) { d.Capacity = 10000000 }, func(d *DNSSettings) { d.Timeout = 0 }, func(d *DNSSettings) { d.Policy = "DOMAIN,a.invalid,missing" }, func(d *DNSSettings) { d.FakeIP = true; d.FakeIPv4 = "not-a-prefix" }, func(d *DNSSettings) { d.FakeIPFilter = "https://invalid" }} {
		d := s
		change(&d)
		if d.Validate() == nil {
			t.Fatalf("invalid settings accepted: %+v", d)
		}
	}
	rules, err := ParseDNSPolicies("# comment\nDOMAIN-REGEX,^a{1,3}\\.test$,proxy\nQUERY-TYPE,aaaa,reject")
	if err != nil || len(rules) != 2 || rules[0].Value != "^a{1,3}\\.test$" || rules[1].Value != "AAAA" {
		t.Fatal(rules, err)
	}
	if _, err := ParseDNSPolicies(strings.Repeat("x", 16385)); err == nil {
		t.Fatal("unbounded policy")
	}
}

func TestDNSDefaultsAndExplicitSettings(t *testing.T) {
	c := Default()
	d := EffectiveDNS(c, false)
	if d.Direct != "system" || d.Proxy != "auto" || d.Final != "auto" || d.ProxyRoute != "proxy" || !d.FollowRules || d.Strategy != "" || d.StaleTTL != 1800 || d.FakeIPv6 != "2001:2::/48" || d.FakeIP || d.Persist {
		t.Fatalf("incorrect defaults: %+v", d)
	}
	d.Direct = "tcp://192.0.2.1"
	d.StaleTTL = 60
	c.Settings.DNS = &d
	if got := EffectiveDNS(c, true); got != d {
		t.Fatal("explicit user settings changed")
	}
	for _, mutate := range []func(*DNSSettings){func(s *DNSSettings) { s.Timeout = 31 }, func(s *DNSSettings) { s.Capacity = 1023 }, func(s *DNSSettings) { s.ProxyRoute = "node" }} {
		bad := DefaultDNS()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid DNS limits accepted")
		}
	}
}

func TestLoadLegacyDNSLimitsWithoutRewriting(t *testing.T) {
	c := Default()
	d := DefaultDNS()
	d.Timeout = 60
	d.Capacity = 64
	c.Settings.DNS = &d
	raw, _ := json.Marshal(c)
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, raw, 0600)
	loaded, err := Load(path)
	if err != nil || loaded.Settings.DNS.Timeout != 30 || loaded.Settings.DNS.Capacity != 1024 {
		t.Fatal(loaded.Settings.DNS, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("load rewrote the user's profile")
	}
}

func TestSystemResolverCannotClaimANodeDetour(t *testing.T) {
	d := DefaultDNS()
	d.Proxy = "system"
	d.ProxyRoute = "node"
	d.ProxyNode = "fixture"
	if d.Validate() == nil {
		t.Fatal("system DNS silently ignored a requested outbound")
	}
}
