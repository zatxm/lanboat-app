package native

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestNativeWindowRoutingControls(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	if err := u.backend.SaveConfigFile("", "visual.conf", "[Rule]\nDOMAIN,first.invalid,DIRECT\nFINAL,PROXY\n[Host]\nkeep.invalid=192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	clickPage := pageClicker(t, v)
	active = v
	defer func() { active = nil; v.dispose() }()
	find := func(text string) *widget {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == text {
				return c.widget
			}
		}
		t.Fatal("missing", text)
		return nil
	}
	u.openPage("config")
	pump()
	// The built-in default is compact; use a large fixture to exercise pagination and scrolling.
	var rules strings.Builder
	rules.WriteString("[Rule]\n")
	for i := 0; i < 205; i++ {
		fmt.Fprintf(&rules, "DOMAIN,rule%d.example,DIRECT\n", i)
	}
	rules.WriteString("FINAL,PROXY\n")
	u.state.Config.Files[0].Content = rules.String()
	u.openRoutingFile(u.state.Config.Files[0])
	pump()
	for _, c := range v.controls {
		if c.widget.text == "添加" {
			t.Fatal("default shows Add")
		}
	}
	if find("复制后编辑").expand || v.editorScroll == nil || v.scroll.scrollPosition(-1) != 0 {
		t.Fatal("default toolbar or list scroll")
	}
	entries, _ := routing.Entries(u.state.Config.Files[0].Content)
	rows := 0
	for _, w := range v.widgets {
		if w.classes == "node-row" {
			rows++
		}
	}
	if rows != pageSize || len(entries) <= 200 {
		t.Fatal("truncated default rules", rows, len(entries))
	}
	for n := 1; n < (len(entries)+pageSize-1)/pageSize; n++ {
		clickPage("Next page")
		pump()
	}
	foundLast := false
	for _, w := range v.widgets {
		if w.kind == "label" && w.text == entries[len(entries)-1].Rule.Value {
			foundLast = true
		}
	}
	if !foundLast {
		t.Fatal("last rule is inaccessible through pagination")
	}
	u.field("page.routing").SetText("0")
	u.render()
	pump()
	v.editorScroll.scrollPosition(10000)
	pump()
	if v.editorScroll.scrollPosition(-1) == 0 || v.scroll.scrollPosition(-1) != 0 {
		t.Fatal("rules must scroll internally")
	}
	for _, c := range v.controls {
		if c.widget.text == "策略组" {
			t.Fatal("routing control has a strategy-group shortcut")
		}
	}
	var fileIndex int
	for i, f := range u.state.Config.Files {
		if f.Name == "visual.conf" {
			fileIndex = i
		}
	}
	u.openRoutingFile(u.state.Config.Files[fileIndex])
	pump()
	search := v.inputs["routingSearch"]
	search.edit("first.invalid")
	if u.field("routingSearch").Text() != "" {
		t.Fatal("typing applied the rule search before Enter")
	}
	search.dispatch("submit", "")
	pump()
	if u.field("routingSearch").Text() != "first.invalid" {
		t.Fatal("Enter did not submit the rule search")
	}
	v.inputs["routingSearch"].edit("")
	v.inputs["routingSearch"].dispatch("submit", "")
	pump()
	find("添加").click()
	pump()
	if u.popup == nil || u.popup.title != "添加规则" || v.inputs["policy"] == nil || v.inputs["policy"].kind != "select" {
		t.Fatal("rule title or policy dropdown")
	}
	v.inputs["policy"].edit("direct")
	v.inputs["route.value"].edit("second.invalid")
	find("确定").click()
	waitJob(t, u)
	pump()
	f, _ := u.routingFile()
	p, err := routing.Parse(f.Content)
	if err != nil || len(p.Rules) != 2 || !strings.Contains(f.Content, "[Host]\nkeep.invalid=192.0.2.1") || u.popup != nil {
		t.Fatal("visual add", err)
	}
	find("停用").click()
	waitJob(t, u)
	pump()
	f, _ = u.routingFile()
	p, _ = routing.Parse(f.Content)
	entries, _ = routing.Entries(f.Content)
	if len(p.Rules) != 1 || len(entries) != 2 || !entries[0].Disabled {
		t.Fatal("disable was not stored")
	}
	u.back()
	pump()
	find("策略组").click()
	pump()
	find("创建策略组").click()
	pump()
	if u.popup == nil || u.popup.title != "创建" || v.inputs["group.selected"] == nil {
		t.Fatal("missing creation dialog or default policy")
	}
	name := v.inputs["group.name"]
	choice := v.inputs["group.selected"]
	if name.pointer.Bounds().X != choice.pointer.Bounds().X {
		t.Fatal("dialog fields are not aligned")
	}
	choice.edit("direct")
	name.edit("Manual group")
	find("确定").click()
	waitJob(t, u)
	pump()
	groups := u.state.Config.PolicyGroups
	if len(groups) != 1 || groups[0].Name != "Manual group" || groups[0].Selected != "direct" || u.popup != nil {
		t.Fatal("shared group not saved", groups)
	}
	u.back()
	pump()
	u.openRoutingFile(u.state.Config.Files[fileIndex])
	pump()
	final := v.inputs["routingFinal"]
	if final.tooltip != "当流量没有匹配任何分流规则时，使用此策略。" {
		t.Fatal("fallback policy tooltip")
	}
	next := v.next
	scroll := v.editorScroll
	final.edit(groups[0].Ref())
	waitJob(t, u)
	pump()
	f, _ = u.routingFile()
	p, _ = routing.Parse(f.Content)
	if v.next != next || v.editorScroll != scroll {
		t.Fatal("final policy navigated or rebuilt the page")
	}
	if p.Final != groups[0].Ref() {
		t.Fatal("final group policy not applied", p.Final)
	}
	if err := u.backend.ActivateConfigFile(f.ID); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	u.openDetail("rule-test")
	pump()
	host := v.inputs["matchHost"]
	host.edit("nothing.invalid")
	next = v.next
	find("测试规则").click()
	waitJob(t, u)
	pump()
	if !strings.Contains(u.result, "Manual group") || v.inputs["matchHost"] != host || v.next != next {
		t.Fatal("rule test rebuilt form or missed policy", u.result)
	}

}

func TestStrategyGroupPagination(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	for i := range pageSize + 1 {
		u.state.Config.PolicyGroups = append(u.state.Config.PolicyGroups, profile.PolicyGroup{
			ID: fmt.Sprint(i), Group: routing.Group{Name: fmt.Sprintf("Test group %d", i), Type: "select", Members: []string{"proxy"}},
		})
	}
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	clickPage := pageClicker(t, v)
	active = v
	defer func() { active = nil; v.dispose() }()
	u.openPage("config")
	u.openDetail("routing-groups")
	pump()
	visible := func(name string) bool {
		for _, w := range v.widgets {
			if w.text == name {
				return true
			}
		}
		return false
	}
	last := fmt.Sprintf("Test group %d", pageSize)
	if visible(last) {
		t.Fatal("all groups rendered on the first page")
	}
	clickPage("Next page")
	pump()
	if !visible(last) || visible("Test group 0") {
		t.Fatal("next page did not show the remaining group")
	}
	for _, w := range v.widgets {
		if hasClass(w.classes, "list-frame") {
			_, _, _, height := w.bounds()
			if height < listRowHeight || height > listRowHeight+4 {
				t.Fatalf("a one-row page does not fit its content: %d", height)
			}
			return
		}
	}
	t.Fatal("missing list frame")
}
