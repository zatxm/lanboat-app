package native

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowPreferences(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	pump()
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for u.busy && time.Now().Before(deadline) {
			pump()
			time.Sleep(time.Millisecond)
		}
		pump()
		if u.busy || u.err != "" {
			t.Fatalf("job: busy=%v error=%s", u.busy, u.err)
		}
	}
	u.editNode(profile.Node{Type: "ss"})
	u.field("name").SetText("连接")
	u.field("server").SetText("localhost")
	u.field("password").SetText("secret")
	u.saveEditor()
	wait()
	id := u.state.Config.Nodes[0].ID
	u.delays[id] = 188
	u.render()
	pump()
	list, search, delay, next := v.nodeContainer, v.inputs["search"], v.delayLabels[id], v.next
	u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
	wait()
	if v.next != next || list != v.nodeContainer || search != v.inputs["search"] || delay != v.delayLabels[id] {
		t.Fatal("language rebuilt home")
	}
	if v.nav["settings"].nativeText() != "Settings" || delay.nativeText() != "188 ms" {
		t.Fatal("language or latency not refreshed")
	}
	found := false
	for _, w := range v.widgets {
		if w.raw && w.text == "连接" {
			found = true
			if w.nativeText() != "连接" {
				t.Fatal("user node name translated")
			}
		}
	}
	if !found {
		t.Fatal("node name missing")
	}
	for _, mode := range []string{"direct", "global", "rule"} {
		v.routing.focus()
		u.settings(mode)
		v.refresh()
		wait()
		if v.inputs["search"].hasFocus() {
			t.Fatal("saving a route moved focus to search")
		}
		if v.next != next || delay != v.delayLabels[id] {
			t.Fatal("route rebuilt list")
		}
	}
	u.toggleAutoSelect("")
	wait()
	if v.next != next {
		t.Fatal("auto select rebuilt home")
	}
	// Data-page settings update their own row, not the page.
	u.navigate("data")
	pump()
	next = v.next
	u.changeSettings(func(s *profile.Settings) { s.ProxyLog = true; s.DNSLog = true; s.LogRetention = 7 })
	wait()
	if v.next != next {
		t.Fatal("data preference rebuilt page")
	}
	for _, w := range v.widgets {
		if w.text == "服务器节点" || strings.HasPrefix(w.text, "覆盖现有") || strings.HasPrefix(w.text, "自动重命名") {
			t.Fatal("removed node data controls remain")
		}
	}
	u.navigate("settings")
	pump()
	u.openProxy()
	pump()
	u.editProxy("port")
	v.refresh()
	pump()
	input := v.inputs["proxy.port"]
	input.edit("1088")
	input.focus()
	v.scroll.scrollPosition(100)
	pump()
	scrollOffset := v.scroll.scrollPosition(-1)
	next = v.next
	u.changeSettings(func(s *profile.Settings) { s.Language = "zh"; s.Appearance = "dark"; s.UpdateOnStart = true })
	wait()
	if v.next != next || v.inputs["proxy.port"] != input || input.nativeText() != "1088" || v.theme != "dark" || !input.hasFocus() || v.scroll.scrollPosition(-1) != scrollOffset || v.nav["settings"].nativeText() != "设置" {
		t.Fatal("settings refresh lost controls or unsaved input")
	}
	u.saveProxySection("port")
	wait()
	if u.detail != "proxy-settings" || u.state.Config.Settings.Port != 1088 {
		t.Fatal("port save rebuilt settings")
	}
	// Every form uses English metadata while editable values remain untouched.
	u.changeSettings(func(s *profile.Settings) { s.Language = "en"; s.Appearance = "light" })
	wait()
	for _, page := range []string{"home", "config", "data", "settings"} {
		u.navigate(page)
		pump()
		assertEnglishWidgets(t, v)
		if u.traffic.Load() != (page == "data") {
			t.Fatal("traffic polling does not follow visible data page")
		}
	}
	for _, proto := range profile.Protocols {
		u.navigate("home")
		u.editNode(profile.Node{Type: proto.ID})
		pump()
		assertEnglishWidgets(t, v)
	}
	u.navigate("home")
	u.editNode(profile.Node{Type: "ss"})
	pump()
	input = v.inputs["name"]
	input.edit("保存")
	next = v.next
	u.changeSettings(func(s *profile.Settings) { s.Language = "zh" })
	wait()
	if input != v.inputs["name"] || input.nativeText() != "保存" || next != v.next {
		t.Fatal("language lost draft input")
	}
}

func assertEnglishWidgets(t *testing.T, v *nativeView) {
	t.Helper()
	for _, w := range v.widgets {
		if w.raw || w.kind == "box" || w.kind == "icon" {
			continue
		}
		text := w.nativeText()
		if strings.ContainsFunc(text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			t.Errorf("untranslated interface %q => %q", w.text, text)
		}
	}
}
