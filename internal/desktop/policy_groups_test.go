package desktop

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestSharedPolicyGroupLifecycle(t *testing.T) {
	a := testApp(t)
	group := func(name, action string) profile.PolicyGroup {
		return profile.PolicyGroup{Group: routing.Group{Name: name, Type: "select", Members: []string{action}, Selected: action}}
	}
	if err := a.SavePolicyGroup(group("Exit", "direct")); err != nil {
		t.Fatal(err)
	}
	exit := a.GetState().Config.PolicyGroups[0]
	if err := a.SavePolicyGroup(group("Nested", exit.Ref())); err != nil {
		t.Fatal(err)
	}
	nested := a.GetState().Config.PolicyGroups[1]
	for _, name := range []string{"one.conf", "two.conf"} {
		if err := a.SaveConfigFile("", name, "[Rule]\nFINAL,"+nested.Ref()); err != nil {
			t.Fatal(err)
		}
	}
	exit.Name = "Renamed"
	if err := a.SavePolicyGroup(exit); err != nil {
		t.Fatal(err)
	}
	c := a.GetState().Config
	for _, f := range c.Files[1:] {
		if err := a.ActivateConfigFile(f.ID); err != nil {
			t.Fatal(err)
		}
		p, err := profile.EffectiveRouting(a.GetState().Config)
		if err != nil || p.Final != nested.Ref() || len(p.Groups) != 2 || p.Groups[1].Name != exit.Ref() {
			t.Fatalf("shared target lost from %q: %+v (%v)", f.Name, p.Groups, err)
		}
	}
	if err := a.SaveRules([]profile.Rule{{Type: "domain", Value: "example.com", Action: exit.Ref()}}); err != nil {
		t.Fatal(err)
	}
	if err := a.DeletePolicyGroup(exit.ID); err == nil {
		t.Fatal("deleted a group referenced by a custom rule and parent group")
	}
	if err := a.DeletePolicyGroup(nested.ID); err == nil {
		t.Fatal("deleted a group referenced by a routing configuration")
	}
	cycle := exit
	cycle.Members, cycle.Selected = []string{nested.Ref()}, nested.Ref()
	if err := a.SavePolicyGroup(cycle); err == nil {
		t.Fatal("accepted a cyclic policy group")
	}

	_, content, err := a.exportPayload("config", c.Files[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := routing.Parse(string(content))
	if err != nil || p.Final != "Nested" || len(p.Groups) != 2 || p.Groups[0].Members[0] != exit.Name ||
		strings.Contains(string(content), nested.Ref()) {
		t.Fatalf("config export lost group definitions: %s (%v)", content, err)
	}
	b := testApp(t)
	if err := b.SaveConfigFile("", "imported.conf", string(content)); err != nil {
		t.Fatal(err)
	}
	cc := b.GetState().Config
	if len(cc.PolicyGroups) != 2 || cc.PolicyGroups[0].Name != "Nested" || !strings.Contains(cc.Files[1].Content, cc.PolicyGroups[0].Ref()) {
		t.Fatalf("config import lost group references: %+v", cc.PolicyGroups)
	}
	backup, err := a.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	path, err := a.ExportBackupTo(backup.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ImportBackup(path); err != nil {
		t.Fatal(err)
	}
	restored := b.GetState().Config
	if len(restored.PolicyGroups) != 2 || restored.PolicyGroups[0].ID != exit.ID || restored.PolicyGroups[0].Name != exit.Name ||
		len(restored.Rules) != 1 || restored.Rules[0].Action != exit.Ref() {
		t.Fatalf("backup did not preserve group IDs and references: %+v", restored.PolicyGroups)
	}
}

func TestSharedSubscriptionGroupsRemainIsolated(t *testing.T) {
	a := testApp(t)
	const content = "[Proxy Group]\nExit=select,origin\n[Rule]\nDOMAIN,private.example,Exit"
	for _, id := range []string{"one", "two"} {
		n := profile.Node{ID: id, Name: "origin", Server: id + ".example", Port: 443, Method: "aes-256-gcm", Password: "test", Source: id}
		sub := profile.Subscription{ID: id, Name: id, URL: "https://" + id + ".example", KeepRules: true}
		if _, err := a.commitImport([]importGroup{{sub: sub, nodes: []profile.Node{n}, rules: content}}); err != nil {
			t.Fatal(err)
		}
	}
	c := a.GetState().Config
	if len(c.PolicyGroups) != 2 || c.PolicyGroups[0].Name == c.PolicyGroups[1].Name {
		t.Fatalf("conflicting subscription names: %+v", c.PolicyGroups)
	}
	for i, n := range c.Nodes {
		c.Selected = n.ID
		p, err := profile.EffectiveRouting(c)
		if err != nil || len(p.Groups) != 1 || p.Rules[0].Action != c.PolicyGroups[i].Ref() ||
			!slices.Equal(p.Groups[0].Members, []string{"node:" + n.ID}) {
			t.Fatalf("incorrect group scope for %q: %+v (%v)", n.ID, p.Groups, err)
		}
	}
	_, bundle, err := a.exportPayload("nodes", "")
	if err != nil {
		t.Fatal(err)
	}
	b := testApp(t)
	if count, err := b.importNodeBundle(string(bundle)); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	imported := b.GetState().Config
	if len(imported.PolicyGroups) != 2 || imported.PolicyGroups[0].ID != c.PolicyGroups[0].ID ||
		len(imported.RuleOverlays) != 2 || imported.RuleOverlays[1].Content != c.RuleOverlays[1].Content {
		t.Fatal("node import lost subscription groups or attached rules")
	}
}

func TestSharedPolicyGroupRoutesTraffic(t *testing.T) {
	a := autoSelectApp(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()
	node := autoHTTPNode("upstream", upstream.URL)
	a.config.Nodes, a.config.Selected = []profile.Node{node}, node.ID
	a.config.Settings.Mode = "rule"
	group := profile.PolicyGroup{Group: routing.Group{Name: "Exit", Type: "select", Selected: "direct", Members: []string{"direct"}}}
	if err := a.SavePolicyGroup(group); err != nil {
		t.Fatal(err)
	}
	group = a.GetState().Config.PolicyGroups[0]
	if err := a.SaveConfigFile("", "shared.conf", "[Rule]\nFINAL,"+group.Ref()); err != nil {
		t.Fatal(err)
	}
	if err := a.ActivateConfigFile(a.GetState().Config.Files[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	proxy, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", a.config.Settings.Port))
	if err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}
	reached := func() bool {
		response, err := client.Get(origin.URL)
		if err != nil {
			return false
		}
		response.Body.Close()
		return response.StatusCode == http.StatusNoContent
	}
	if !reached() {
		t.Fatal("shared direct strategy did not forward traffic to the origin")
	}
	group.Members, group.Selected = []string{"proxy"}, "proxy"
	if err := a.SavePolicyGroup(group); err != nil {
		t.Fatal(err)
	}
	if reached() {
		t.Fatal("group edit did not switch live traffic to the proxy strategy")
	}
}
