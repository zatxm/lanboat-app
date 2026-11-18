package desktop

import (
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestRuleRequestIncludesCustomPriorityDisabledAndPort(t *testing.T) {
	a := testApp(t)
	rules := []profile.Rule{{Type: "domain", Value: "example.com", Action: "reject", Disabled: true}, {Type: "dst_port", Value: "8443", Action: "reject"}, {Type: "domain", Value: "example.com", Action: "direct"}}
	if err := a.SaveRules(rules); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		port int
		want string
	}{{8443, "REJECT"}, {443, "DIRECT"}} {
		result, err := a.TestRuleRequest(routing.Request{Host: "example.com", Port: tc.port, Network: "tcp"})
		if err != nil || !strings.HasPrefix(result, tc.want) {
			t.Fatal(result, err)
		}
	}
	state := a.GetState()
	rules = state.Config.Rules
	rules[0].Disabled = false
	if err := a.SaveRules(rules); err != nil {
		t.Fatal(err)
	}
	result, err := a.TestRule("example.com", "")
	if err != nil || !strings.HasPrefix(result, "REJECT") {
		t.Fatal(result, err)
	}
	if _, err = a.TestRule("example.com", "bad-ip"); err == nil {
		t.Fatal("invalid IP accepted")
	}
}
