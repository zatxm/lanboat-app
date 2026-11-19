package native

import (
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
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
	find("添加").click()
	pumpUI()
	v.inputs["group.name"].edit("Automatic")
	v.inputs["group.addPolicy"].edit("direct")
	pumpUI()
	// Choosing a URL preset/custom value edits the group draft, not global settings.
	u.field("group.type").SetText("url-test")
	u.render()
	pumpUI()
	globalURL := u.state.Config.Settings.TestTarget()
	find("修改").click()
	pumpUI()
	if u.popup == nil || u.popup.kind != "group-test-settings" {
		t.Fatal("group URL editor")
	}
	v.inputs["group.test.choice"].edit("custom")
	pumpUI()
	v.inputs["group.test.custom"].edit("https://example.com/latency")
	u.closePopup()
	pumpUI()
	if u.field("group.url").Text() != profile.TestURLs[0] {
		t.Fatal("cancel applied URL")
	}
	find("修改").click()
	pumpUI()
	v.inputs["group.test.choice"].edit("custom")
	pumpUI()
	v.inputs["group.test.custom"].edit("https://example.com/latency")
	// The dialog's Save button is distinct from the persistent page header Save.
	for _, c := range v.controls {
		if c.widget.text == "保存" && c.widget != v.save {
			c.widget.click()
			break
		}
	}
	pumpUI()
	if u.popup != nil || u.field("group.url").Text() != "https://example.com/latency" || u.state.Config.Settings.TestTarget() != globalURL {
		t.Fatal("group URL save changed global settings")
	}
	u.saveEditor()
	waitJob(t, u)
	pumpUI()
	saved, _ := u.routingFile()
	parsed, err := routing.Parse(saved.Content)
	if err != nil || len(parsed.Groups) != 3 || !strings.Contains(saved.Content, "https://example.com/latency") {
		t.Fatal("group URL not persisted", err, saved.Content)
	}
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
	if v.pagination.visible {
		t.Fatal("empty home pager remains visible")
	}
	u.openPage("rules")
	pumpUI()
	for _, c := range v.controls {
		if c.widget.text == "上一页" || c.widget.text == "下一页" {
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
