package dnsconfig

import (
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/dnscheck"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func SystemBootstrap() string {
	for _, name := range []string{"/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"} {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 || f[0] != "nameserver" {
				continue
			}
			a, err := netip.ParseAddr(f[1])
			if err == nil && !a.IsLoopback() && !a.IsUnspecified() {
				return a.String()
			}
		}
	}
	return "1.1.1.1"
}
func Server(tag, address, detour string) map[string]any {
	a, _ := profile.ParseDNSAddress(address)
	out := map[string]any{"type": a.Type, "tag": tag}
	if a.Type == "local" {
		return out
	}
	out["server"], out["server_port"] = a.Server, a.Port
	if detour != "direct" {
		out["detour"] = detour
	}
	if _, err := netip.ParseAddr(a.Server); err != nil {
		out["domain_resolver"] = "bootstrap"
	}
	if a.Type == "https" || a.Type == "h3" {
		out["path"] = a.Path
	}
	if a.Type == "tls" || a.Type == "quic" || a.Type == "https" || a.Type == "h3" {
		out["tls"] = map[string]any{"enabled": true}
	}
	return out
}

// Both native cores use the same policy ordering.
func Build(c profile.Config, tun bool, bootstrap string, domains []string) map[string]any {
	s := profile.EffectiveDNS(c, tun)
	if bootstrap == "" {
		bootstrap = SystemBootstrap()
	}
	if s.Bootstrap == "system" || s.Bootstrap == "local" {
		s.Bootstrap = bootstrap
	}
	hasProxy := c.Selected != "" || tun
	if s.Proxy == "auto" {
		if hasProxy && c.Settings.Mode != "direct" {
			s.Proxy = "https://1.1.1.1/dns-query"
		} else {
			s.Proxy = "system"
		}
	}
	if s.Final == "auto" {
		s.Final = "proxy"
		if !hasProxy || c.Settings.Mode == "direct" {
			s.Final = "direct"
		} else if c.Settings.Mode == "rule" && c.Routing != nil && c.Routing.Final == "direct" {
			s.Final = "direct"
		}
	}
	detour := s.ProxyRoute
	if detour == "node" {
		detour = "dns-node"
	}
	if c.Settings.Mode == "direct" || !hasProxy {
		detour = "direct"
	}
	servers := []any{Server("bootstrap", s.Bootstrap, "direct"), Server("dns-direct", s.Direct, "direct"), Server("dns-proxy", s.Proxy, detour)}
	tags := map[string]string{"direct": "dns-direct", "proxy": "dns-proxy", "bootstrap": "bootstrap"}
	targetServer := func(target string) string {
		if target == "reject" {
			return ""
		}
		if tag := tags[target]; tag != "" {
			return tag
		}
		tag := fmt.Sprintf("dns-policy-%d", len(tags))
		outbound := "policy-target:" + target
		if s.ProxyRoute != "proxy" {
			outbound = detour
		}
		servers = append(servers, Server(tag, s.Proxy, outbound))
		tags[target] = tag
		return tag
	}
	var rules []any
	if tun {
		servers = append(servers, map[string]any{"type": "hosts", "tag": "dns-check", "predefined": map[string][]string{strings.TrimSuffix(dnscheck.Sentinel, "."): {"192.0.2.53", "2001:db8::53"}}})
		rules = append(rules, map[string]any{"inbound": []string{"tun-in"}, "domain": []string{strings.TrimSuffix(dnscheck.Sentinel, ".")}, "action": "route", "server": "dns-check"})
	}
	if len(domains) > 0 {
		rules = append(rules, map[string]any{"domain": domains, "action": "route", "server": "bootstrap"})
	}
	if c.Routing != nil && len(c.Routing.Hosts) > 0 {
		servers = append(servers, map[string]any{"type": "hosts", "tag": "hosts", "predefined": c.Routing.Hosts})
		hosts := make([]string, 0, len(c.Routing.Hosts))
		for h := range c.Routing.Hosts {
			hosts = append(hosts, h)
		}
		sort.Strings(hosts)
		rules = append(rules, map[string]any{"domain": hosts, "action": "route", "server": "hosts"})
	}
	add := func(kind, value, target string) {
		rule := map[string]any{kind: []string{value}, "action": "route", "server": targetServer(target)}
		if target == "reject" {
			delete(rule, "server")
			rule["action"] = "reject"
		}
		rules = append(rules, rule)
	}
	if s.FakeIP {
		for _, line := range strings.Split(s.FakeIPFilter, "\n") {
			host := strings.TrimSpace(line)
			if host == "" {
				continue
			}
			kind := "domain"
			if strings.HasPrefix(host, "*.") || strings.HasPrefix(host, "+.") || strings.HasPrefix(host, ".") {
				kind = "domain_suffix"
				host = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(host, "*."), "+."), ".")
			}
			add(kind, host, "direct")
		}
	}
	policies, _ := profile.ParseDNSPolicies(s.Policy)
	for _, r := range policies {
		add(r.Type, r.Value, r.Resolver)
	}
	if s.FollowRules && c.Settings.Mode == "rule" {
		if c.Routing != nil {
			for _, r := range c.Routing.Rules {
				if cond := routing.DomainCondition(r); cond != nil {
					cond["action"] = "route"
					cond["server"] = targetServer(r.Action)
					if r.Action == "reject" {
						delete(cond, "server")
						cond["action"] = "reject"
					}
					rules = append(rules, cond)
				}
			}
		} else {
			for _, r := range c.Rules {
				switch r.Type {
				case "domain", "domain_suffix", "domain_keyword", "domain_regex":
					add(r.Type, r.Value, r.Action)
				}
			}
		}
	}
	if s.FakeIP {
		servers = append(servers, map[string]any{"type": "fakeip", "tag": "dns-fakeip", "inet4_range": s.FakeIPv4, "inet6_range": s.FakeIPv6})
		if tun {
			rules = append(rules, map[string]any{"inbound": []string{"tun-in"}, "query_type": []string{"A", "AAAA"}, "action": "route", "server": "dns-fakeip"})
		}
	}
	out := map[string]any{"servers": servers, "rules": rules, "final": tags[s.Final], "timeout": duration(s.Timeout), "disable_cache": !s.Cache, "cache_capacity": s.Capacity, "optimistic": map[string]any{"enabled": s.Cache && s.Optimistic, "timeout": duration(s.StaleTTL)}, "reverse_mapping": true}
	if s.Strategy != "" {
		out["strategy"] = s.Strategy
	}
	return out
}

func duration(seconds int) string { return strconv.Itoa(seconds) + "s" }
