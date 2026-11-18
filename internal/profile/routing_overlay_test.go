package profile

import (
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestNodeSourceRulesAndPolicyIsolation(t *testing.T) {
	c := Default()
	EnsureFiles(&c)
	c.Files[0].Content = "[Rule]\nDOMAIN,system.test,DIRECT\nFINAL,DIRECT"
	c.Nodes = []Node{{ID: "one", Name: "same"}, {ID: "two", Name: "same"}, {ID: "local", Name: "local"}}
	c.Rules = []Rule{{Type: "domain", Value: "custom.test", Action: "reject"}}
	c.RuleOverlays = []RuleOverlay{
		{ID: "first", NodeIDs: []string{"one"}, Content: "[Proxy Group]\ngroup:proxy = select,same,DIRECT\n[Rule]\nDOMAIN,one.test,group:proxy"},
		{ID: "second", NodeIDs: []string{"two"}, Content: "[Proxy Group]\ngroup:proxy = select,same,DIRECT\n[Rule]\nDOMAIN,two.test,group:proxy"},
	}
	for _, tc := range []struct {
		id, first, group, target string
		priority                 int
	}{{"one", "one.test", "overlay:first:0", "node:one", 1}, {"two", "two.test", "overlay:second:0", "node:two", 1}, {"local", "custom.test", "", "", 0}} {
		c.Selected = tc.id
		p, err := EffectiveRouting(c)
		if err != nil {
			t.Fatal(err)
		}
		if err = InsertCustomRouting(&p, c.Rules); err != nil {
			t.Fatal(err)
		}
		if p.Rules[0].Value != tc.first || p.PriorityRules != tc.priority {
			t.Fatalf("%s: %+v", tc.id, p.Rules)
		}
		if tc.priority > 0 && (len(p.Groups) != 1 || p.Groups[0].Name != tc.group || p.Groups[0].Members[0] != tc.target || p.Rules[0].Action != tc.group || p.Rules[1].Value != "custom.test") {
			t.Fatalf("%s: %+v", tc.id, p)
		}
		if tc.priority == 0 && len(p.Groups) != 0 {
			t.Fatal("other node inherited groups")
		}
	}
}
func TestSourceFinalStopsFurtherSystemRules(t *testing.T) {
	c := Default()
	EnsureFiles(&c)
	c.Selected = "one"
	c.Nodes = []Node{{ID: "one", Name: "one"}}
	c.Rules = []Rule{{Type: "domain", Value: "custom.test", Action: "direct"}}
	c.RuleOverlays = []RuleOverlay{{ID: "one", NodeIDs: []string{"one"}, Content: "[Rule]\nDOMAIN,a.test,DIRECT\nFINAL,REJECT"}}
	p, err := EffectiveRouting(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = InsertCustomRouting(&p, c.Rules); err != nil {
		t.Fatal(err)
	}
	if !p.OverrideFinal || p.Final != "reject" || len(p.Rules) != 1 || p.Rules[0].Value != "a.test" {
		t.Fatal(p)
	}
}
func TestOverlayRejectsAmbiguousNodeSource(t *testing.T) {
	c := Default()
	EnsureFiles(&c)
	c.Selected = "a"
	c.RuleOverlays = []RuleOverlay{{ID: "one", NodeIDs: []string{"a"}, Content: "[Rule]\nDOMAIN,a.test,DIRECT"}, {ID: "two", NodeIDs: []string{"a"}, Content: "[Rule]\nDOMAIN,b.test,DIRECT"}}
	if _, err := EffectiveRouting(c); err == nil {
		t.Fatal("ambiguous owner accepted")
	}
	if _, err := routing.Parse(c.RuleOverlays[0].Content); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateSubscriptionRulesPreservesOldFile(t *testing.T) {
	c := Default()
	EnsureFiles(&c)
	c.Subscriptions = []Subscription{{ID: "sub", Name: "one", URL: "https://example.com", KeepRules: true}}
	c.Nodes = []Node{{ID: "one", Name: "node", Source: "sub"}}
	providerRule := (routing.Rule{Type: "RULE-SET", Value: "https://rules.example/private", Action: "proxy", SetInterval: 900, SetHeaders: map[string][]string{"Authorization": {"Bearer legacy-sub"}}}).Text()
	old := ConfigFile{ID: "subscription-sub", Name: "subscription-sub.conf", Content: "[Rule]\nDOMAIN,custom.test,REJECT\n" + providerRule + "\n" + routing.DefaultConfig}
	// Real legacy files already contain one rule section with default rules.
	old.Content = "[Rule]\nDOMAIN,custom.test,REJECT\n" + providerRule + "\n" + strings.TrimPrefix(routing.DefaultConfig, "# Lanboat default routing rules\n[Rule]\n")
	c.Files = append(c.Files, old)
	c.ActiveFile = old.ID
	changed, err := MigrateSubscriptionRules(&c)
	if err != nil || !changed || c.ActiveFile != "default" || len(c.RuleOverlays) != 1 || c.Files[1].Content != old.Content || c.Files[1].ID == old.ID {
		t.Fatal(c, err)
	}
	c.Selected = "one"
	p, err := EffectiveRouting(c)
	if err != nil || p.Rules[0].Value != "custom.test" {
		t.Fatal(p, err)
	}
	foundProvider := false
	for _, rule := range p.Rules {
		if rule.Value == "https://rules.example/private" {
			foundProvider = rule.SetInterval == 900 && rule.SetHeaders["Authorization"][0] == "Bearer legacy-sub"
		}
	}
	if !foundProvider {
		t.Fatal("legacy subscription migration lost provider headers or refresh interval", p.Rules)
	}
	changed, err = MigrateSubscriptionRules(&c)
	if err != nil || changed {
		t.Fatal("migration not idempotent", err)
	}
}
