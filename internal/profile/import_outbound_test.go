package profile

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestImportSingAndV2Configs(t *testing.T) {
	cases := []struct{ name, content string }{
		{"sing-box", `{"outbounds":[{"type":"shadowsocks","tag":"node","server":"example.com","server_port":443,"method":"aes-256-gcm","password":"secret"},{"type":"direct","tag":"direct"}],"route":{"rules":[{"domain_suffix":["example.org"],"outbound":"direct"}],"final":"proxy"}}`},
		{"v2ray", `{"outbounds":[{"protocol":"shadowsocks","tag":"node","settings":{"servers":[{"address":"example.com","port":443,"method":"aes-256-gcm","password":"secret"}]}},{"protocol":"freedom","tag":"direct"}],"routing":{"rules":[{"type":"field","domain":["domain:example.org"],"outboundTag":"direct"}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, merged, err := ParseImportWithRules(tc.content, routing.DefaultConfig)
			if err != nil || len(n) != 1 || n[0].Server != "example.com" || n[0].Method != "aes-256-gcm" || !strings.Contains(merged, "DOMAIN-SUFFIX,example.org,DIRECT") {
				t.Fatal(n, merged, err)
			}
		})
	}
}

func TestImportClashIPRuleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "payload:\n  - 192.0.2.0/24\n  - 2001:db8::/32\n")
	}))
	defer server.Close()
	content := fmt.Sprintf("proxies:\n  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}\nrule-providers:\n  office:\n    type: http\n    behavior: ipcidr\n    format: yaml\n    url: %q\nrules:\n  - RULE-SET,office,DIRECT\n", server.URL)
	_, merged, err := ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil {
		t.Fatal(err)
	}
	if err := routing.ResolveSets(context.Background(), &p, "", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if !routing.MatchRule(p.Rules[0], routing.Request{IP: netip.MustParseAddr("192.0.2.6")}) || routing.MatchRule(p.Rules[0], routing.Request{IP: netip.MustParseAddr("198.51.100.6")}) {
		t.Fatal("IP provider matching incorrect")
	}
}

func TestImportClashInlineProviders(t *testing.T) {
	content := `proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}
rule-providers:
  local-domains: {type: inline, behavior: domain, payload: [+.example.org, .sub.example.org, exact.example.net]}
  local-ip: {type: inline, behavior: ipcidr, payload: [192.0.2.0/24]}
rules:
  - RULE-SET,local-domains,DIRECT
  - RULE-SET,local-ip,REJECT`
	_, merged, err := ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Rules) != 4 {
		t.Fatal(merged, err)
	}
	if !routing.MatchRule(p.Rules[0], routing.Request{Host: "example.org"}) || routing.MatchRule(p.Rules[1], routing.Request{Host: "sub.example.org"}) || !routing.MatchRule(p.Rules[1], routing.Request{Host: "a.sub.example.org"}) || !routing.MatchRule(p.Rules[3], routing.Request{IP: netip.MustParseAddr("192.0.2.5")}) {
		t.Fatal("inline provider semantics changed")
	}
}

func TestImportClashRuleProviderIntervalAndInvalidReferences(t *testing.T) {
	content := `proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}
rule-providers:
  office: {type: http, behavior: domain, format: yaml, url: https://example.org/rules.yaml, interval: 900}
rules:
  - RULE-SET,office,DIRECT`
	_, merged, err := ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Rules) != 1 || p.Rules[0].SetInterval != 900 {
		t.Fatal(merged, p, err)
	}
	if !strings.Contains(p.Rules[0].Text(), "interval=900") {
		t.Fatal("provider interval was lost when serializing its rule", p.Rules[0].Text())
	}

	withHeaders := `proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}
rule-providers:
  private: {type: http, behavior: domain, url: https://rules.example/private.yaml, interval: 600, header: {Authorization: "Bearer top-secret", X-Scope: ["one", "two"]}}
rules:
  - RULE-SET,private,PROXY`
	_, merged, err = ParseImportWithRules(withHeaders, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err = routing.Parse(merged)
	if err != nil || len(p.Rules) != 1 || p.Rules[0].SetHeaders["Authorization"][0] != "Bearer top-secret" || len(p.Rules[0].SetHeaders["X-Scope"]) != 2 {
		t.Fatal("HTTP provider headers did not survive import", merged, p, err)
	}
	roundTrip, err := routing.Parse("[Rule]\n" + p.Rules[0].Text())
	if err != nil || len(roundTrip.Rules) != 1 || roundTrip.Rules[0].SetHeaders["Authorization"][0] != "Bearer top-secret" {
		t.Fatal("HTTP provider headers did not survive rule editing", roundTrip, err)
	}

	for _, invalid := range []string{
		`proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}
rules:
  - RULE-SET,missing,DIRECT`,
		`proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}
rule-providers:
  office: {type: http, behavior: domain, url: https://example.org/rules.yaml, unknown-option: true}
rules:
  - RULE-SET,office,DIRECT`,
	} {
		if _, _, err := ParseImportWithRules(invalid, "[Rule]\n"); err == nil {
			t.Fatal("invalid provider reference or unsupported provider behavior was silently accepted")
		}
	}
}

func TestImportClashMRSProvider(t *testing.T) {
	data, err := os.ReadFile("../routing/testdata/github.mrs")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	content := fmt.Sprintf("proxies:\n  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}\nrule-providers:\n  github: {type: http, behavior: domain, format: mrs, url: %q}\nrules:\n  - RULE-SET,github,DIRECT\n", server.URL)
	_, merged, err := ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil {
		t.Fatal(err)
	}
	if err := routing.ResolveSets(context.Background(), &p, "", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if !routing.MatchRule(p.Rules[0], routing.Request{Host: "github.com"}) || routing.MatchRule(p.Rules[0], routing.Request{Host: "example.invalid"}) {
		t.Fatal("MRS provider matching incorrect")
	}
}

func TestImportSingBoxRuleSets(t *testing.T) {
	core := os.Getenv("LANBOAT_TEST_CORE")
	if core == "" {
		t.Skip("sing-box binary needed")
	}
	dir := t.TempDir()
	source, binary := filepath.Join(dir, "source.json"), filepath.Join(dir, "binary-data")
	if err := os.WriteFile(source, []byte(`{"version":4,"rules":[{"ip_cidr":["192.0.2.0/24"]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(core, "rule-set", "compile", source, "-o", binary).CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	content := fmt.Sprintf(`{"outbounds":[{"type":"shadowsocks","tag":"node","server":"example.com","server_port":443,"method":"aes-256-gcm","password":"secret"}],"route":{"rule_set":[{"type":"local","tag":"address-set","format":"binary","path":%q},{"type":"inline","tag":"inline-set","rules":[{"domain_suffix":["example.org"]}]}],"rules":[{"rule_set":"address-set","outbound":"direct"},{"rule_set":"inline-set","outbound":"direct"}]}}`, binary)
	_, merged, err := ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil {
		t.Fatal(err)
	}
	if err := routing.ResolveSets(context.Background(), &p, core, dir); err != nil {
		t.Fatal(err)
	}
	if len(p.Rules) != 2 || !routing.MatchRule(p.Rules[0], routing.Request{IP: netip.MustParseAddr("192.0.2.4")}) || !routing.MatchRule(p.Rules[1], routing.Request{Host: "foo.example.org"}) {
		t.Fatal("sing-box rule sets changed")
	}
}
func TestImportCombinedNativeRules(t *testing.T) {
	content := `{"outbounds":[{"type":"shadowsocks","server":"example.com","server_port":443,"method":"aes-256-gcm","password":"secret"}],"route":{"rules":[{"domain":["a.test"],"network":"udp","outbound":"direct"}]}}`
	_, merged, err := ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Rules) != 1 {
		t.Fatal(merged, err)
	}
	r := p.Rules[0]
	if !routing.MatchRule(r, routing.Request{Host: "a.test", Network: "udp"}) || routing.MatchRule(r, routing.Request{Host: "a.test", Network: "tcp"}) || routing.MatchRule(r, routing.Request{Host: "b.test", Network: "udp"}) {
		t.Fatal("combined condition changed semantics")
	}
}

func TestImportV2CombinedRules(t *testing.T) {
	content := `{"outbounds":[{"protocol":"shadowsocks","tag":"node","settings":{"servers":[{"address":"example.com","port":443,"method":"aes-256-gcm","password":"secret"}]}}],"routing":{"rules":[{"type":"field","domain":["domain:a.test","full:b.test"],"network":"tcp,udp","port":"443,8000-8002","outboundTag":"direct"}]}}`
	_, merged, err := ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Rules) != 1 {
		t.Fatal(merged, err)
	}
	r := p.Rules[0]
	if !routing.MatchRule(r, routing.Request{Host: "foo.a.test", Network: "tcp", Port: 443}) || routing.MatchRule(r, routing.Request{Host: "foo.a.test", Network: "tcp", Port: 80}) || routing.MatchRule(r, routing.Request{Host: "other.test", Network: "tcp", Port: 443}) {
		t.Fatal("V2Ray conjunction changed semantics")
	}
}

func TestImportRejectsUnsupportedRulesWithoutPartialImport(t *testing.T) {
	for _, content := range []string{
		`proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}
proxy-groups:
  - {name: proxy, type: load-balance, proxies: [HK, DIRECT]}
rules:
  - DOMAIN,example.org,proxy`,
		`{"outbounds":[{"type":"shadowsocks","server":"example.com","server_port":443,"method":"aes-256-gcm","password":"secret","detour":"another"}]}`,
	} {
		if _, _, err := ParseImportWithRules(content, routing.DefaultConfig); err == nil {
			t.Fatal("unsupported setting silently accepted")
		}
	}
}

func TestImportClashKeepsGroupsAndPolicies(t *testing.T) {
	content := `proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}
proxy-groups:
  - {name: proxy, type: select, proxies: [HK, DIRECT]}
rules:
  - DOMAIN,example.org,proxy
  - MATCH,proxy`
	_, merged, err := ParseImportWithRules(content, routing.DefaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Groups) != 1 || p.Groups[0].Name != "group:proxy" || p.Groups[0].Members[0] != "HK" || p.Rules[0].Action != "group:proxy" || p.Final != "group:proxy" {
		t.Fatal(p, err)
	}
}

func TestImportSingBoxKeepsSelectorsAndPolicies(t *testing.T) {
	content := `{"outbounds":[{"type":"shadowsocks","tag":"HK","server":"example.com","server_port":443,"method":"aes-256-gcm","password":"secret"},{"type":"direct","tag":"bypass"},{"type":"selector","tag":"proxy","outbounds":["HK","bypass"],"default":"HK"}],"route":{"rules":[{"domain_suffix":["example.org"],"outbound":"proxy"}],"final":"bypass"}}`
	_, merged, err := ParseImportWithRules(content, routing.DefaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Groups) != 1 || p.Groups[0].Name != "group:proxy" || p.Groups[0].Members[0] != "HK" || p.Groups[0].Members[1] != "direct" || p.Groups[0].Selected != "HK" || p.Rules[0].Action != "group:proxy" || p.Final != "direct" {
		t.Fatal(merged, p, err)
	}
}
func TestImportPreservesNamedNodeCase(t *testing.T) {
	content := `{"outbounds":[{"type":"shadowsocks","tag":"HongKong","server":"example.com","server_port":443,"method":"aes-256-gcm","password":"secret"}],"route":{"rules":[{"domain":["example.org"],"outbound":"HongKong"}],"final":"HongKong"}}`
	_, merged, err := ParseImportWithRules(content, routing.DefaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || p.Rules[0].Action != "HongKong" || p.Final != "HongKong" {
		t.Fatal(merged, err)
	}
}

func TestImportV2XHTTPOptions(t *testing.T) {
	input := `{"outbounds":[{"protocol":"vless","tag":"node","settings":{"vnext":[{"address":"example.com","port":443,"users":[{"id":"12345678-1234-4234-8234-123456789012","encryption":"none"}]}]},"streamSettings":{"network":"xhttp","security":"none","xhttpSettings":{"path":"/x","mode":"packet-up","extra":{"noGRPCHeader":true,"sessionPlacement":"query","sessionKey":"session","xPaddingBytes":"100-300","xmux":{"maxConcurrency":"0-0"}}}}}]}`
	nodes, _, err := ParseImportWithRules(input, routing.DefaultConfig)
	if err != nil || len(nodes) != 1 {
		t.Fatal(nodes, err)
	}
	n := nodes[0]
	extra, err := n.XHTTPExtraSettings()
	if err != nil || n.TextOption("xhttp-opts.mode") != "packet-up" || !extra.NoGRPCHeader || extra.SessionPlacement != "query" || extra.SessionKey != "session" || extra.PaddingMax != 300 {
		t.Fatal(n, extra, err)
	}
}

func TestImportV2XHTTPDownload(t *testing.T) {
	input := `{"outbounds":[{"protocol":"vless","tag":"node","settings":{"vnext":[{"address":"example.com","port":443,"users":[{"id":"12345678-1234-4234-8234-123456789012","encryption":"none"}]}]},"streamSettings":{"network":"xhttp","security":"tls","xhttpSettings":{"path":"/x","extra":{"downloadSettings":{"address":"down.example.com","port":8443,"network":"xhttp","security":"tls","tlsSettings":{"serverName":"sni.example.com"},"xhttpSettings":{"path":"/x","host":"host.example.com"}}}}}}]}`
	nodes, _, err := ParseImportWithRules(input, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	extra, err := nodes[0].XHTTPExtraSettings()
	if err != nil || extra.DownloadTargetHost != "down.example.com" || extra.DownloadTargetPort != "8443" || extra.DownloadServerName != "sni.example.com" || extra.DownloadHTTPHost != "host.example.com" {
		t.Fatal(extra, err)
	}
}

func TestImportClashXHTTPAndTransportOptions(t *testing.T) {
	input := `proxies:
  - name: xhttp
    type: vless
    server: example.com
    port: 443
    uuid: 12345678-1234-4234-8234-123456789012
    network: xhttp
    tls: true
    servername: tls.example.com
    client-fingerprint: Chrome150
    xhttp-opts:
      path: /x
      host: front.example.com
      mode: packet-up
      extra:
        noGRPCHeader: true
        sessionPlacement: query
        sessionKey: sid
        xPaddingBytes: 100-300
        xmux:
          maxConcurrency: 2
`
	nodes, err := ParseImport(input)
	if err != nil || len(nodes) != 1 {
		t.Fatal(nodes, err)
	}
	n := nodes[0]
	extra, err := n.XHTTPExtraSettings()
	if err != nil || n.TextOption("obfs") != "" || n.TextOption("network") != "xhttp" || n.TextOption("transport-path") != "/x" || n.TextOption("transport-host") != "front.example.com" || n.TextOption("sni") != "tls.example.com" || n.TextOption("xhttp-opts.mode") != "packet-up" || !extra.NoGRPCHeader || extra.SessionKey != "sid" || extra.PaddingMax != 300 || extra.MaxConcurrency != "2" {
		t.Fatal(n, extra, err)
	}

	input = `proxies:
  - {name: ws, type: vmess, server: example.com, port: 443, uuid: 12345678-1234-4234-8234-123456789012, network: ws, ws-opts: {path: /socket, headers: {Host: front.example.com}}}
`
	nodes, err = ParseImport(input)
	if err != nil || len(nodes) != 1 || nodes[0].TextOption("transport-path") != "/socket" || nodes[0].TextOption("transport-host") != "front.example.com" {
		t.Fatal(nodes, err)
	}
	input = `proxies:
  - {name: bad, type: vless, server: example.com, port: 443, uuid: 12345678-1234-4234-8234-123456789012, unsupported-behavior: true}
`
	if _, err = ParseImport(input); err == nil || !strings.Contains(err.Error(), "unsupported-behavior") {
		t.Fatal("unsupported Clash node field was silently retained", err)
	}
}

func TestImportClashAndV2TransportHeaders(t *testing.T) {
	clash := `proxies:
  - name: ws
    type: vmess
    server: ws.example.com
    port: 443
    uuid: 12345678-1234-4234-8234-123456789012
    network: ws
    ws-opts: {path: /ws, headers: {Host: front.example.com, Authorization: ws-token}}
  - name: h2
    type: vmess
    server: h2.example.com
    port: 443
    uuid: 12345678-1234-4234-8234-123456789012
    network: h2
    h2-opts: {path: /h2, headers: {Authorization: h2-token}}
  - name: http-upgrade
    type: vmess
    server: up.example.com
    port: 443
    uuid: 12345678-1234-4234-8234-123456789012
    network: httpupgrade
    httpupgrade-opts: {path: /up, headers: {Authorization: up-token}}`
	nodes, err := ParseImport(clash)
	if err != nil || len(nodes) != 3 {
		t.Fatal(nodes, err)
	}
	for i, network := range []string{"ws", "h2", "httpupgrade"} {
		headers, _ := nodes[i].Option(network + "-opts.headers").(map[string]any)
		if headers["Authorization"] == nil {
			t.Fatalf("%s request header was lost: %#v", network, nodes[i].Options)
		}
	}
	if nodes[0].TextOption("transport-host") != "front.example.com" {
		t.Fatal("WebSocket Host was not mapped")
	}
	v2 := `{"outbounds":[{"protocol":"vmess","tag":"http","settings":{"vnext":[{"address":"example.com","port":443,"users":[{"id":"12345678-1234-4234-8234-123456789012"}]}]},"streamSettings":{"network":"http","httpSettings":{"path":"/v2","method":"PATCH","headers":{"Host":"front.example.com","Authorization":"v2-token"}}}}]}`
	imported, err := ParseImport(v2)
	if err != nil || len(imported) != 1 || imported[0].TextOption("transport-method") != "PATCH" || imported[0].TextOption("transport-host") != "front.example.com" {
		t.Fatal(imported, err)
	}
	headers, _ := imported[0].Option("http-opts.headers").(map[string]any)
	if headers["Authorization"] != "v2-token" {
		t.Fatal("V2Ray HTTP request header was lost", imported[0].Options)
	}
}
