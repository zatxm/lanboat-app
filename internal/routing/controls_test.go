package routing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestVisualEditsPreserveOtherSectionsAndDisabledRules(t *testing.T) {
	original := "# keep comment\n[General]\ndns-server=system\n[Rule]\n# rule comment\nDOMAIN,a.invalid,DIRECT\nFINAL,PROXY\n[Host]\na.invalid=127.0.0.1\n[URL Rewrite]\n^http://old/ https://new/ 302\n"
	entries, err := Entries(original)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	edited, err := ReplaceLine(original, entries[0].Rule.Line, DisabledPrefix+entries[0].Rule.Text())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(edited)
	if err != nil || len(p.Rules) != 0 || len(p.Hosts) != 1 || len(p.Rewrites) != 1 {
		t.Fatal("disabled or unrelated sections", err)
	}
	entries, err = Entries(edited)
	if err != nil || len(entries) != 1 || !entries[0].Disabled {
		t.Fatal(entries, err)
	}
	edited, err = InsertSectionLine(edited, "Rule", "DST-PORT,443,node:example")
	if err != nil {
		t.Fatal(err)
	}
	edited, err = SetFinal(edited, "direct")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(edited, "# keep comment\n[General]\ndns-server=system") || !strings.Contains(edited, "[Host]\na.invalid=127.0.0.1") {
		t.Fatal("unrelated content changed")
	}
	p, _ = Parse(edited)
	if p.Final != "direct" || len(p.Rules) != 1 || p.Rules[0].Type != "DST-PORT" {
		t.Fatal("insert/final", p)
	}
}
func TestGroupsAndRegexOptionsRoundTrip(t *testing.T) {
	text := "[Proxy Group]\nGroup with space = select,proxy,node:abc,policy-select-name=node:abc\nFast = url-test,proxy,node:abc,url=https://example.com,interval=120,tolerance=20\n[Rule]\nDOMAIN-REGEX,^x{1,3}\\.example$,Group with space,no-resolve\nFINAL,Fast"
	p, err := Parse(text)
	if err != nil || len(p.Groups) != 2 || !p.Rules[0].NoResolve || p.Rules[0].Value != `^x{1,3}\.example$` {
		t.Fatal(p, err)
	}
	for _, g := range p.Groups {
		parsed, err := parseGroup(g.Text())
		if err != nil || parsed.Name != g.Name || len(parsed.Members) != len(g.Members) {
			t.Fatal("roundtrip", g, err)
		}
	}
	for _, line := range []string{"DIRECT = select,proxy", "G = url-test,proxy,url=ftp://bad", "G = select,reject", "G = select,proxy,interval=0", "G = select,proxy,policy-select-name=missing"} {
		if _, err := Parse("[Proxy Group]\n" + line); err == nil {
			t.Fatal("invalid group", line)
		}
	}
}
func TestRuleSetsAndRequestMatchingUseResolvedContents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/domains" {
			w.Write([]byte(".service.invalid\nexact.invalid\n"))
		} else {
			w.Write([]byte(`{"version":3,"rules":[{"type":"logical","mode":"and","rules":[{"domain_suffix":["mixed.invalid"]},{"port":[8443]}]}]}`))
		}
	}))
	defer server.Close()
	p, err := Parse("[Rule]\nRULE-SET," + server.URL + "/rules,proxy\nDOMAIN-SET," + server.URL + "/domains,reject\nIP-CIDR,192.0.2.0/24,direct,no-resolve\nNETWORK,udp,proxy\nFINAL,direct")
	if err != nil {
		t.Fatal(err)
	}
	if err = ResolveSets(context.Background(), &p, "", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		req  Request
		want string
	}{{Request{Host: "x.mixed.invalid", Port: 8443}, "proxy"}, {Request{Host: "x.mixed.invalid", Port: 443}, "direct"}, {Request{Host: "x.service.invalid", Port: 443}, "reject"}, {Request{Host: "exact.invalid"}, "reject"}, {Request{IP: netip.MustParseAddr("192.0.2.2"), Network: "udp"}, "direct"}, {Request{Network: "udp"}, "proxy"}}
	for _, tc := range cases {
		if got, _ := p.MatchRequest(tc.req); got != tc.want {
			t.Fatal(tc.req, got, tc.want)
		}
	}
}

func TestUserAgentSlashAndLiteralClasses(t *testing.T) {
	p, err := Parse("[Rule]\nUSER-AGENT,Mozilla*,proxy\nUSER-AGENT,App[1]*,reject\nFINAL,direct")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ agent, want string }{{"Mozilla/5.0", "proxy"}, {"App[1]/2", "reject"}, {"App1/2", "direct"}} {
		got, _ := p.MatchRequest(Request{Agent: tc.agent})
		if got != tc.want {
			t.Fatal(tc.agent, got)
		}
	}
}
