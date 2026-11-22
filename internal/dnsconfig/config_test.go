package dnsconfig

import (
	"context"
	"fmt"
	"github.com/goccy/go-json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestDNSNativeConfigMatrix(t *testing.T) {
	core := os.Getenv("LANBOAT_TEST_CORE")
	if core == "" {
		t.Skip("native core required")
	}
	for _, address := range []string{"system", "udp://1.1.1.1", "tcp://1.1.1.1", "https://cloudflare-dns.com/dns-query", "tls://dns.google", "quic://dns.adguard-dns.com", "h3://cloudflare-dns.com/dns-query"} {
		t.Run(address, func(t *testing.T) {
			s := profile.DefaultDNS()
			s.Direct = address
			s.Proxy = address
			if address == "system" {
				s.ProxyRoute = "direct"
			}
			s.FakeIP = true
			s.Optimistic = true
			s.Policy = "DOMAIN-SUFFIX,example.com,direct\nQUERY-TYPE,HTTPS,reject"
			c := profile.Default()
			c.Settings.DNS = &s
			c.Settings.Mode = "global"
			config := map[string]any{"dns": Build(c, true, "1.1.1.1", []string{"node.example"}), "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}, map[string]any{"type": "socks", "tag": "proxy", "server": "127.0.0.1", "server_port": 1080}}, "route": map[string]any{"default_domain_resolver": "bootstrap"}}
			raw, _ := json.Marshal(config)
			file := filepath.Join(t.TempDir(), "dns.json")
			os.WriteFile(file, raw, 0600)
			if data, err := exec.Command(core, "check", "-c", file).CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, data)
			}
		})
	}
}
func dnsPeer(t *testing.T, answer string) (string, *atomic.Int32) {
	t.Helper()
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hits := new(atomic.Int32)
	server := &dns.Server{PacketConn: packet, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		hits.Add(1)
		r := new(dns.Msg)
		r.SetReply(q)
		for _, question := range q.Question {
			if question.Qtype == dns.TypeA {
				r.Answer = append(r.Answer, &dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: net.ParseIP(answer)})
			}
		}
		w.WriteMsg(r)
	})}
	go server.ActivateAndServe()
	t.Cleanup(func() { server.Shutdown() })
	return "udp://" + packet.LocalAddr().String(), hits
}
func TestDNSNativePolicyBootstrapCacheAndFakeIP(t *testing.T) {
	core := os.Getenv("LANBOAT_TEST_CORE")
	if core == "" {
		t.Skip("native core required")
	}
	direct, dh := dnsPeer(t, "192.0.2.10")
	remote, rh := dnsPeer(t, "198.51.100.20")
	bootstrap, bh := dnsPeer(t, "127.0.0.1")
	// A hostname upstream proves bootstrap resolution uses its own native DNS server.
	_, remotePort, _ := net.SplitHostPort(remote[len("udp://"):])
	remote = "udp://resolver.fixture:" + remotePort
	s := profile.DefaultDNS()
	s.Bootstrap = bootstrap
	s.Direct = direct
	s.Proxy = remote
	s.ProxyRoute = "direct"
	s.FakeIP = true
	s.FollowRules = true
	s.Policy = "DOMAIN,priority.fixture,proxy\nDOMAIN,blocked.fixture,reject\nDOMAIN-REGEX,^local[0-9]{1,3}\\.fixture$,direct"
	s.FakeIPFilter = "*.lan"
	s.Final = "proxy"
	s.Policy += "\nDOMAIN,cached.fixture,proxy"
	c := profile.Default()
	c.Settings.Mode = "rule"
	c.Settings.DNS = &s
	c.Routing = &routing.Parsed{Rules: []routing.Rule{{Type: "DOMAIN", Value: "priority.fixture", Action: "direct"}, {Type: "DOMAIN", Value: "route.fixture", Action: "direct"}, {Type: "DOMAIN-REGEX", Value: "^route[0-9]+\\.fixture$", Action: "direct"}, {Type: "RULE-SET", Action: "direct", SetRules: []map[string]any{{"domain": []string{"set.fixture"}}, {"ip_cidr": []string{"198.18.0.0/15"}}}}}}
	listener, _ := net.ListenPacket("udp", "127.0.0.1:0")
	port := listener.LocalAddr().(*net.UDPAddr).Port
	listener.Close()
	config := map[string]any{"log": map[string]any{"level": "error"}, "dns": Build(c, true, "1.1.1.1", []string{"node.fixture"}), "inbounds": []any{map[string]any{"type": "direct", "tag": "tun-in", "listen": "127.0.0.1", "listen_port": port, "network": "udp"}}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"default_domain_resolver": "bootstrap", "rules": []any{map[string]any{"inbound": []string{"tun-in"}, "action": "hijack-dns"}}}}
	raw, _ := json.Marshal(config)
	dir := t.TempDir()
	file := filepath.Join(dir, "dns.json")
	os.WriteFile(file, raw, 0600)
	logfile, _ := os.Create(filepath.Join(dir, "core.log"))
	defer logfile.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, core, "run", "-c", file)
	cmd.Stdout = logfile
	cmd.Stderr = logfile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); cmd.Wait() })
	client := &dns.Client{Net: "udp", Timeout: time.Second}
	target := fmt.Sprintf("127.0.0.1:%d", port)
	query := func(name string) *dns.Msg {
		t.Helper()
		q := new(dns.Msg)
		q.SetQuestion(name+".", dns.TypeA)
		r, _, err := client.Exchange(q, target)
		if err != nil {
			b, _ := os.ReadFile(logfile.Name())
			t.Fatalf("%s: %v %s", name, err, b)
		}
		return r
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		q := new(dns.Msg)
		q.SetQuestion("ready.fixture.", dns.TypeA)
		if _, _, err := client.Exchange(q, target); err == nil {
			break
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(logfile.Name())
			t.Fatalf("DNS startup: %s", b)
		}
		time.Sleep(10 * time.Millisecond)
	}
	ip := func(name string) string {
		r := query(name)
		if len(r.Answer) != 1 {
			t.Fatal(name, r)
		}
		return r.Answer[0].(*dns.A).A.String()
	}
	if ip("priority.fixture") != "198.51.100.20" || ip("route.fixture") != "192.0.2.10" || ip("local12.fixture") != "192.0.2.10" || ip("node.fixture") != "127.0.0.1" {
		t.Fatal("wrong DNS policy result")
	}
	if ip("route12.fixture") != "192.0.2.10" || ip("set.fixture") != "192.0.2.10" {
		t.Fatal("shared domain policy lost regex or rule-set projection")
	}
	if query("blocked.fixture").Rcode != dns.RcodeRefused {
		t.Fatal("reject did not return REFUSED")
	}
	before := rh.Load()
	if ip("cached.fixture") != "198.51.100.20" || ip("cached.fixture") != "198.51.100.20" || rh.Load() != before+1 {
		t.Fatal("native cache ineffective")
	}
	fake := net.ParseIP(ip("fake.example"))
	if !net.ParseIP("198.18.0.0").Equal(fake.Mask(net.CIDRMask(15, 32))) {
		t.Fatal("FakeIP allocation failed", fake)
	}
	if ip("host.lan") != "192.0.2.10" {
		t.Fatal("FakeIP exclusion ignored")
	}
	q6 := new(dns.Msg)
	q6.SetQuestion("fake.example.", dns.TypeAAAA)
	r6, _, err := client.Exchange(q6, target)
	if err != nil || len(r6.Answer) != 1 {
		t.Fatal("FakeIP AAAA", r6, err)
	}
	_, prefix, _ := net.ParseCIDR(s.FakeIPv6)
	if !prefix.Contains(r6.Answer[0].(*dns.AAAA).AAAA) {
		t.Fatal("FakeIP IPv6 range", r6)
	}

	if dh.Load() == 0 || bh.Load() == 0 || rh.Load() == 0 {
		t.Fatal("one DNS layer was bypassed", dh.Load(), bh.Load(), rh.Load())
	}
}

func TestDNSAutoAndHighPriorityExclusions(t *testing.T) {
	c := profile.Default()
	c.Settings.Mode = "rule"
	c.Selected = "node"
	c.Routing = &routing.Parsed{Final: "proxy", Rules: []routing.Rule{{Type: "DOMAIN-REGEX", Value: "^rx[0-9]+\\.test$", Action: "direct"}, {Type: "IP-CIDR", Value: "192.0.2.0/24", Action: "proxy"}}}
	d := profile.DefaultDNS()
	d.FakeIP = true
	d.Policy = "DOMAIN-SUFFIX,lan,proxy"
	c.Settings.DNS = &d
	got := Build(c, true, "192.0.2.53", nil)
	if _, exists := got["strategy"]; exists {
		t.Fatal("auto strategy must be omitted")
	}
	servers := got["servers"].([]any)
	remote := servers[2].(map[string]any)
	if remote["type"] != "https" || remote["detour"] != "proxy" || got["final"] != "dns-proxy" {
		t.Fatal("proxy defaults", got)
	}
	rules := got["rules"].([]any)
	if rules[1].(map[string]any)["server"] != "dns-direct" {
		t.Fatal("exclusion did not precede user policy", rules)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "ip_cidr") || !strings.Contains(string(raw), "domain_regex") {
		t.Fatal("domain projection", string(raw))
	}
	c.Selected = ""
	got = Build(c, false, "192.0.2.53", nil)
	if got["final"] != "dns-direct" || got["servers"].([]any)[2].(map[string]any)["type"] != "local" {
		t.Fatal("disconnected auto resolver", got)
	}
}
