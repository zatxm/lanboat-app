package profile

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestSubscriptionFilterLimitAndOverrides(t *testing.T) {
	var input strings.Builder
	for i := range 1100 {
		fmt.Fprintln(&input, URI(Node{Name: fmt.Sprintf("HK-%04d", i), Server: "example.com", Port: 443, Method: "aes-256-gcm", Password: "secret"}))
	}
	sub := Subscription{Name: "test", URL: "https://example.com", MaxNodes: 7, Filter: "hk-0", UDP: "false", Insecure: "true", FastOpen: "true"}
	nodes, _, err := ParseSubscription(base64.RawURLEncoding.EncodeToString([]byte(input.String())), sub, routing.DefaultConfig)
	if err != nil || len(nodes) != 7 {
		t.Fatal(len(nodes), err)
	}
	last := ""
	for _, n := range nodes {
		if n.Name <= last || !strings.HasPrefix(n.Name, "HK-0") || n.TextOption("udp") != "false" || n.TextOption("skip-cert-verify") != "true" || n.TextOption("tfo") != "true" {
			t.Fatal("filter, sample order or overrides lost", n.Name)
		}
		last = n.Name
	}
	sub.Filter = "no match"
	if _, _, err := ParseSubscription(input.String(), sub, routing.DefaultConfig); err == nil {
		t.Fatal("empty filter result accepted")
	}
	sub.Filter = "["
	if err := sub.Validate(); err == nil {
		t.Fatal("invalid regular expression accepted")
	}
}

func TestSubscriptionRulesAndDefaultOptions(t *testing.T) {
	content := `proxies:
  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret, udp: false, tfo: true}
rules:
  - DOMAIN,custom.lanboat.test,DIRECT
  - DOMAIN,custom.lanboat.test,DIRECT
  - DOMAIN-SUFFIX,proxy.lanboat.test,PROXY
  - MATCH,PROXY
`
	sub := Subscription{Name: "test", URL: "https://example.com", KeepRules: true}
	nodes, merged, err := ParseSubscription(content, sub, routing.DefaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	if nodes[0].TextOption("udp") != "false" || nodes[0].TextOption("tfo") != "true" {
		t.Fatal("defaults overwrote provider options")
	}
	p, err := routing.Parse(merged)
	base, _ := routing.Parse(routing.DefaultConfig)
	if err != nil || len(p.Rules) != len(base.Rules)+2 || p.Rules[0].Value != "custom.lanboat.test" || p.Rules[1].Action != "proxy" || p.Final != base.Final {
		t.Fatal("incorrect rule merge", err)
	}
	if strings.Contains(routing.DefaultConfig, "custom.lanboat.test") {
		t.Fatal("default modified")
	}
	content = strings.Replace(content, "DOMAIN,custom.lanboat.test,DIRECT", "RULE-SET,unavailable,DIRECT", 1)
	if _, _, err := ParseSubscription(content, sub, routing.DefaultConfig); err == nil {
		t.Fatal("unsupported retained rule silently ignored")
	}
	sub.KeepRules = false
	if _, merged, err := ParseSubscription(content, sub, routing.DefaultConfig); err != nil || merged != "" {
		t.Fatal("disabled rules parsed", err)
	}
}

func TestRetainedRulesTakePriorityWithExistingDefault(t *testing.T) {
	base := "[rule]\nDOMAIN-SUFFIX,example.com,REJECT\nDOMAIN,allow.example.com,DIRECT\nFINAL,DIRECT"
	merged, err := mergeSubscriptionRulesKnown(base, []string{"DOMAIN,allow.example.com,DIRECT"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || p.Rules[0].Value != "allow.example.com" || p.Final != "direct" {
		t.Fatal("subscription priority or original default lost", err)
	}
}

func TestRetainedRuleSetHeadersSurviveSubscriptionMerge(t *testing.T) {
	headers := map[string][]string{"Authorization": {"Bearer subscription"}}
	line := (routing.Rule{Type: "RULE-SET", Value: "https://rules.example/private.yaml", Action: "proxy", SetInterval: 900, SetHeaders: headers}).Text()
	merged, err := mergeSubscriptionRulesKnown("[Rule]\nFINAL,DIRECT\n", []string{line}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Rules) != 1 || p.Rules[0].SetInterval != 900 || p.Rules[0].SetHeaders["Authorization"][0] != "Bearer subscription" {
		t.Fatal("subscription merge lost rule provider settings", p, err)
	}
}

func TestSubscriptionOverridesConflictsAndFinal(t *testing.T) {
	base := "[General]\nipv6=true\n[Rule]\nDOMAIN,conflict.example,DIRECT\nDOMAIN-SUFFIX,keep.example,DIRECT\nFINAL,DIRECT\n[Host]\nlocalhost=127.0.0.1"
	merged, err := mergeSubscriptionRulesKnown(base, []string{"DOMAIN,conflict.example,Provider Group", "MATCH,REJECT"}, []Node{{Name: "Provider Group"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(merged)
	if err != nil || len(p.Rules) != 2 || p.Rules[0].Action != "Provider Group" || p.Final != "reject" || p.General["ipv6"] != "true" || len(p.Hosts["localhost"]) != 1 {
		t.Fatal("subscription did not replace default conflict", err)
	}
	if strings.Contains(merged, "DOMAIN,conflict.example,DIRECT") || strings.Contains(merged, "FINAL,DIRECT") {
		t.Fatal("conflicting default rules retained")
	}
}

func TestSubscriptionMaximum100000(t *testing.T) {
	s := Subscription{Name: "max", URL: "https://example.com", MaxNodes: 100000}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.MaxNodes = 100001
	if err := s.Validate(); err == nil {
		t.Fatal("maximum above 100000 accepted")
	}
	var content strings.Builder
	for i := range 100000 {
		fmt.Fprintf(&content, "ss://YWVzLTI1Ni1jZmI6cA@example.com:443#node-%d\n", i)
	}
	s.MaxNodes = 100000
	nodes, _, err := ParseSubscription(content.String(), s, routing.DefaultConfig)
	if err != nil || len(nodes) != 100000 {
		t.Fatal("100000 subscription nodes rejected", len(nodes), err)
	}
}
