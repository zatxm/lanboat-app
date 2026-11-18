package routing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedDomainPolicyAndMixedRuleSet(t *testing.T) {
	var hits atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`{"version":4,"rules":[{"domain_suffix":["domain.test"]},{"ip_cidr":["192.0.2.0/24"]},{"type":"logical","mode":"and","rules":[{"domain":["conditional.test"]},{"port":[443]}]},{"type":"logical","mode":"or","rules":[{"domain_regex":["^rx[0-9]{1,3}\\.test$"]},{"process_name":["browser"]}]}]}`))
	}))
	defer peer.Close()
	p, err := Parse("[Rule]\nDOMAIN-REGEX,^rx[0-9]{1,3}\\.test$,DIRECT\nRULE-SET," + peer.URL + ",PROXY\nFINAL,DIRECT")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = ResolveSets(context.Background(), &p, "", dir); err != nil {
		t.Fatal(err)
	}
	if action, _ := p.Match("rx12.test", netip.Addr{}); action != "direct" {
		t.Fatal("regex route mismatch")
	}
	projected := DomainCondition(p.Rules[1])
	raw, _ := json.Marshal(projected)
	var got map[string]any
	json.Unmarshal(raw, &got)
	rules := got["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("IP or conditional predicates entered DNS policy: %s", raw)
	}
	if len(p.Rules[1].SetRules) != 4 {
		t.Fatal("traffic conditions lost")
	}
	if err = ResolveSets(context.Background(), &p, "", dir); err != nil || hits.Load() != 1 {
		t.Fatal("cache", err, hits.Load())
	}
}

func TestRemoteRuleSetUsesConfiguredRefreshInterval(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("payload:\n  - example.org\n"))
	}))
	defer server.Close()
	dir := t.TempDir()
	if _, err := readSet(context.Background(), server.URL, dir, time.Hour); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
	stale := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(filepath.Join(dir, entries[0].Name()), stale, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := readSet(context.Background(), server.URL, dir, time.Second); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 {
		t.Fatalf("provider interval did not refresh the expired cache: requests=%d", hits.Load())
	}
}

func TestRemoteRuleSetHeadersAndCredentialScopedCache(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var domain string
		switch r.Header.Get("Authorization") {
		case "Bearer alpha":
			domain = "alpha.private.test"
		case "Bearer beta":
			domain = "beta.private.test"
		default:
			http.Error(w, "missing authorization", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprintf(w, "payload:\n  - %s\n", domain)
	}))
	defer server.Close()

	ruleA := (Rule{Type: "RULE-SET", Value: server.URL + "/private", Action: "proxy", SetHeaders: map[string][]string{"Authorization": {"Bearer alpha"}}}).Text()
	ruleB := (Rule{Type: "RULE-SET", Value: server.URL + "/private", Action: "direct", SetHeaders: map[string][]string{"Authorization": {"Bearer beta"}}}).Text()
	p, err := Parse("[Rule]\n" + ruleA + "\n" + ruleB)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = ResolveSets(context.Background(), &p, "", dir); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 2 || !MatchRule(p.Rules[0], Request{Host: "alpha.private.test"}) || !MatchRule(p.Rules[1], Request{Host: "beta.private.test"}) || MatchRule(p.Rules[0], Request{Host: "beta.private.test"}) {
		t.Fatalf("provider headers or credential-scoped caches were mixed: hits=%d rules=%#v", hits.Load(), p.Rules)
	}
	if err = ResolveSets(context.Background(), &p, "", dir); err != nil || hits.Load() != 2 {
		t.Fatalf("same credential should reuse its own cache: hits=%d err=%v", hits.Load(), err)
	}
}

func TestRuleSetHeadersRejectInjectionAndRoundTrip(t *testing.T) {
	rule := Rule{Type: "RULE-SET", Value: "https://rules.example/private", Action: "proxy", SetHeaders: map[string][]string{"X-Token": {"a,b"}}}
	p, err := Parse("[Rule]\n" + rule.Text())
	if err != nil || len(p.Rules) != 1 || p.Rules[0].SetHeaders["X-Token"][0] != "a,b" {
		t.Fatal("header values with commas did not round-trip", p, err)
	}
	for _, bad := range []map[string][]string{{"Bad Header": {"x"}}, {"X-Test": {"ok\r\nInjected: yes"}}} {
		data, _ := json.Marshal(bad)
		encoded := base64.RawURLEncoding.EncodeToString(data)
		if _, err := Parse("[Rule]\nRULE-SET,https://rules.example/list,PROXY,headers=" + encoded); err == nil {
			t.Fatalf("invalid request headers were accepted: %#v", bad)
		}
	}
}

func TestNativeSRSRuleSetProjection(t *testing.T) {
	core := os.Getenv("LANBOAT_TEST_CORE")
	if core == "" {
		t.Skip("native core required")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "rules.json")
	binary := filepath.Join(dir, "rules.srs")
	os.WriteFile(source, []byte(`{"version":4,"rules":[{"domain_suffix":["set.test"]},{"ip_cidr":["192.0.2.0/24"]}]}`), 0600)
	if output, err := exec.Command(core, "rule-set", "compile", source, "-o", binary).CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	p := Parsed{Rules: []Rule{{Type: "RULE-SET", Value: binary, Action: "proxy"}}}
	if err := ResolveSets(context.Background(), &p, core, dir); err != nil {
		t.Fatal(err)
	}
	projected := DomainCondition(p.Rules[0])
	if len(projected["rules"].([]any)) != 1 || !RuleSetNeedsIP(p.Rules[0].SetRules) {
		t.Fatal("SRS domain and IP conditions were not separated")
	}
}

func TestSetDomainFieldsUseAlternativeMatching(t *testing.T) {
	p, err := parseSet([]byte(`{"version":2,"rules":[{"domain":["exact.test"],"domain_suffix":["suffix.test"],"domain_regex":"^regex[0-9]+\\.test$"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for host, want := range map[string]bool{"exact.test": true, "www.suffix.test": true, "regex42.test": true, "other.test": false} {
		if got := matchCondition(p[0], Request{Host: host}); got != want {
			t.Errorf("%s: %v", host, got)
		}
	}
}
