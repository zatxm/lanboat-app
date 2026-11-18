package native

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/routing"
)

func TestNativeWindowRoutingControls(t *testing.T) {
	if os.Getenv("LANBOAT_FLTK_TEST") != "1" {
		t.Skip("isolated display required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
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
		find("下一页").click()
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
	find("代理分组").click()
	pump()
	for _, c := range v.controls {
		if c.widget.text == "添加" || c.widget.text == "删除" {
			t.Fatal("default group actions", c.widget.text)
		}
	}
	u.back()
	pump()
	var fileIndex int
	for i, f := range u.state.Config.Files {
		if f.Name == "visual.conf" {
			fileIndex = i
		}
	}
	u.openRoutingFile(u.state.Config.Files[fileIndex])
	pump()
	find("添加").click()
	pump()
	if v.title.text != "添加规则" || v.inputs["policy"] == nil || v.inputs["policy"].kind != "select" {
		t.Fatal("rule title or policy dropdown")
	}
	v.inputs["policy"].edit("direct")
	v.inputs["route.value"].edit("second.invalid")
	u.saveEditor()
	waitJob(t, u)
	pump()
	f, _ := u.routingFile()
	p, err := routing.Parse(f.Content)
	if err != nil || len(p.Rules) != 2 || !strings.Contains(f.Content, "[Host]\nkeep.invalid=192.0.2.1") || u.editor != "" {
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
	find("代理分组").click()
	pump()
	find("添加").click()
	pump()
	if v.inputs["group.addPolicy"] == nil || v.inputs["group.addPolicy"].kind != "select" {
		t.Fatal("group policy dropdown missing")
	}
	v.inputs["group.addPolicy"].edit("direct")
	pump()
	if u.field("group.members").Text() != "proxy\ndirect" {
		t.Fatal("group strategy dropdown duplicated existing member")
	}
	v.inputs["group.name"].edit("Manual group")
	u.saveEditor()
	waitJob(t, u)
	pump()
	f, _ = u.routingFile()
	p, err = routing.Parse(f.Content)
	if err != nil || len(p.Groups) != 1 || p.Groups[0].Name != "Manual group" || u.editor != "" {
		t.Fatal("group save", err)
	}
	u.back()
	pump()
	final := v.inputs["routingFinal"]
	next := v.next
	scroll := v.editorScroll
	final.edit("Manual group")
	waitJob(t, u)
	pump()
	f, _ = u.routingFile()
	p, _ = routing.Parse(f.Content)
	if v.next != next || v.editorScroll != scroll {
		t.Fatal("final policy navigated or rebuilt the page")
	}
	if p.Final != "Manual group" {
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
