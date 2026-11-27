package native

import (
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowAsyncAndPagination(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	if err := u.backend.SaveConfigFile("", "compact.conf", "[Proxy Group]\none=select,PROXY,DIRECT\ntwo=select,DIRECT,PROXY\n[Rule]\nDOMAIN,example.com,DIRECT\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	pumpUI := func() { u.drain(); v.flush(); pump() }
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
	u.navigate("data")
	pumpUI()
	firstY := -1
	for _, w := range []*widget{v.uptime, v.connections, v.totalDown, v.download} {
		_, y, _, _ := w.parent.bounds()
		if firstY < 0 {
			firstY = y
		}
		if y != firstY {
			t.Fatal("wide stats are not four columns")
		}
	}
	release := make(chan struct{})
	b := v.button("异步测试", "primary icon-check", false, false, func() { u.savingSettings = true; u.submit(func() error { <-release; return nil }, nil) })
	v.pack(v.body, b)
	v.show()
	pump()
	b.click()
	pumpUI()
	if !u.busy || !b.loading || b.enabled || u.busyControl != b.id {
		close(release)
		t.Fatal("clicked action has no busy feedback")
	}
	close(release)
	waitJob(t, u)
	pumpUI()
	if b.loading || !b.enabled || b.text != "异步测试" {
		t.Fatal("action did not restore after completion")
	}
	var file profile.ConfigFile
	for _, f := range u.state.Config.Files {
		if f.Name == "compact.conf" {
			file = f
		}
	}
	u.openPage("config")
	u.openRoutingFile(file)
	pumpUI()
	u.openDetail("routing-groups")
	pumpUI()
	find("创建策略组").click()
	pumpUI()
	v.inputs["group.name"].edit("Automatic")
	v.inputs["group.type"].edit("url-test")
	pumpUI()
	// Choosing a URL preset/custom value edits the group draft, not global settings.
	globalURL := u.state.Config.Settings.TestTarget()
	custom := v.inputs["group.test.custom"]
	if custom == nil || custom.visible || u.popup == nil || u.popup.kind != "routing-group" {
		t.Fatal("group URL selector or initial custom field visibility")
	}
	v.inputs["group.test.choice"].edit("custom")
	pumpUI()
	if !custom.visible || u.popup.kind != "routing-group" {
		t.Fatal("custom URL did not open inline")
	}
	if got, want := custom.pointer.Bounds(), v.inputs["group.test.choice"].pointer.Bounds(); got.X != want.X || got.W != want.W {
		t.Fatalf("custom URL should align below the selector: %v, %v", got, want)
	}
	for _, w := range v.widgets {
		if w.text == "自定义URL" || w.text == "测速节点" && w.kind == "button" {
			t.Fatalf("unexpected group field decoration: %s", w.text)
		}
	}
	custom.edit("https://example.com/latency")
	if u.popup.kind != "routing-group" || u.state.Config.Settings.TestTarget() != globalURL {
		t.Fatal("changing group settings opened a nested dialog or changed global settings")
	}
	find("确定").click()
	waitJob(t, u)
	pumpUI()
	groups := u.state.Config.PolicyGroups
	if len(groups) != 3 || groups[2].URL != "https://example.com/latency" || u.popup != nil {
		t.Fatal("group URL not persisted", groups)
	}
	find("删除").click()
	pumpUI()
	if u.popup == nil || u.popup.kind != "confirm" || u.popup.title != "策略组" {
		t.Fatal("group delete dialog has the wrong title")
	}
	u.closePopup()
	u.navigate("data")
	pumpUI()
	u.openNetworkLogs()
	waitJob(t, u)
	pumpUI()
	if v.logPager.visible {
		t.Fatal("empty logs have a pager")
	}

	u.navigate("home")
	pumpUI()
	u.openPage("rules")
	pumpUI()
	for _, c := range v.controls {
		if c.widget.classes == "page-bar" {
			t.Fatal("empty rules show pagination")
		}
	}
	u.navigate("data")
	pumpUI()
	u.openNetworkLogs()
	waitJob(t, u)
	pumpUI()
	// A short log page also hides the pager and preserves the current list body.
	u.networkPage.Total = 1
	u.networkPage.Revision++
	u.networkPage.Entries = nil
	v.refreshNetworkLogs()
	pump()
	if v.logPager.visible {
		t.Fatal("one-page logs show pagination")
	}
}
