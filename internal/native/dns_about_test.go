package native

import (
	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/desktop"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeWindowDNSAndAbout(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	bin := t.TempDir()
	capture := filepath.Join(bin, "opened")
	t.Setenv("LANBOAT_OPEN_CAPTURE", capture)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "xdg-open"), []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$LANBOAT_OPEN_CAPTURE\"\n"), 0700)
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
		deadline := time.Now().Add(3 * time.Second)
		for u.busy && time.Now().Before(deadline) {
			pump()
			time.Sleep(time.Millisecond)
		}
		pump()
		if u.busy || u.err != "" {
			t.Fatal("job", u.err)
		}
	}
	button := func(text string) *widget {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == text || strings.HasPrefix(c.widget.text, text+"\t") {
				return c.widget
			}
		}
		t.Fatal("missing button", text)
		return nil
	}
	u.navigate("settings")
	pump()
	button("DNS设置").click()
	pump()
	if u.editor != "dns-settings" || v.title.text != "DNS设置" || !v.save.visible {
		t.Fatal("DNS navigation")
	}
	for _, name := range []string{"引导解析", "解析策略", "解析器", "解析路由", "缓存与虚拟地址"} {
		found := false
		for _, w := range v.widgets {
			if w.text == name {
				found = true
			}
		}
		if !found {
			t.Fatal("missing layer", name)
		}
	}
	for _, w := range v.controls {
		if w.widget.text == "网页检测" {
			t.Fatal("DNS page still links the browser leak test")
		}
	}
	if u.field("dns.bootstrap").Text() != "system" {
		t.Fatal("DNS page defaults", u.field("dns.bootstrap").Text())
	}
	choose := func(current, value string) {
		t.Helper()
		for _, w := range v.widgets {
			if w.kind == "select" && w.value == current {
				for _, o := range w.options {
					if o.value == value {
						w.edit(value)
						pump()
						return
					}
				}
			}
		}
		t.Fatalf("missing selector %q with option %q", current, value)
	}
	if v.inputs["dns.bootstrap"] != nil || v.editorScroll == nil {
		t.Fatal("system bootstrap should hide the address and DNS must scroll internally")
	}
	controlFillsRow := func(control *widget) {
		t.Helper()
		x, _, width, _ := control.bounds()
		left, _, rowWidth, _ := control.parent.bounds()
		if width == 0 || x+width != left+rowWidth {
			t.Fatal("DNS control does not fill the row", control.value, x, width, left, rowWidth)
		}
	}
	for _, w := range v.widgets {
		if w.kind == "select" && w.value == "system" && len(w.options) == 4 {
			_, _, width, _ := w.bounds()
			_, _, rowWidth, _ := w.parent.bounds()
			if width == 0 || width >= rowWidth {
				t.Fatal("standalone bootstrap selector should have a compact width", width, rowWidth)
			}
		}
	}
	scroll := v.editorScroll
	scroll.scrollPosition(250)
	pump()
	if scroll.scrollPosition(-1) <= 0 || v.scroll.scrollPosition(-1) != 0 {
		t.Fatal("DNS content should scroll independently of the page", scroll.scrollPosition(-1), v.scroll.scrollPosition(-1), scroll.pointer.Bounds(), scroll.content.Bounds())
	}
	scroll.scrollPosition(0)
	pump()
	choose("system", "local")
	if u.field("dns.bootstrap").Text() != "local" || v.inputs["dns.bootstrap"] != nil {
		t.Fatal("local bootstrap should hide the address")
	}
	choose("local", "protocol")
	controlFillsRow(v.inputs["dns.bootstrap"])
	v.inputs["dns.bootstrap"].edit("udp://9.9.9.9")
	button("还原默认").click()
	pump()
	if u.field("dns.bootstrap").Text() != "system" || u.field("dns.proxy").Text() != "auto" || u.notice != "" || v.inputs["dns.bootstrap"] != nil {
		t.Fatal("restore DNS defaults", u.field("dns.bootstrap").Text(), u.notice)
	}
	choose("system", "protocol")
	v.inputs["dns.bootstrap"].edit("https://loop.invalid/dns-query")
	u.saveDNS()
	pump()
	if u.popup == nil || u.popup.kind != "message" || u.backend.GetState().Config.Settings.DNS != nil {
		t.Fatal("invalid draft was saved")
	}
	u.closePopup()
	choose("protocol", "ip")
	if v.inputs["dns.bootstrap"].placeholder != dnsBootstrapIP {
		t.Fatal("missing IP example")
	}
	v.inputs["dns.bootstrap"].edit("udp://1.1.1.1")
	u.saveDNS()
	pump()
	if u.popup == nil || u.backend.GetState().Config.Settings.DNS != nil {
		t.Fatal("plain IP mode accepted a URL")
	}
	u.closePopup()
	v.inputs["dns.bootstrap"].edit("")

	choose("auto", "custom")
	choose("https", "tcp")
	if u.field("dns.proxy").Text() != "tcp://1.1.1.1" {
		t.Fatal("protocol change retained an HTTP path", u.field("dns.proxy").Text())
	}
	proxy := v.inputs["dns.proxyServer"]
	timeout := v.inputs["dns.timeout"]
	px, _, pw, _ := proxy.bounds()
	tx, _, tw, _ := timeout.bounds()
	if px != tx || pw <= tw || tw == 0 {
		t.Fatal("Resolver and route should share the left edge and resolver should be wider", px, pw, tx, tw)
	}
	_, _, available, _ := proxy.parent.bounds()
	if pw >= available {
		t.Fatal("resolver field took the whole column", pw, available)
	}
	for _, w := range v.widgets {
		if w.kind == "select" && w.parent.parent != nil && w.parent.parent.parent == proxy.parent.parent.parent {
			x, _, width, _ := w.bounds()
			if x != px || width != pw {
				t.Fatal("resolver selector and input are misaligned", x, width, px, pw)
			}
		}
	}
	for _, id := range []string{"dns.fakeIPv4", "dns.fakeIPv6", "dns.filter"} {
		control := v.inputs[id]
		controlFillsRow(control)
		if bounds := control.pointer.Bounds(); bounds.X != float32(px) {
			t.Fatal("FakeIP fields lost their common alignment", id, bounds)
		}
		if id == "dns.filter" {
			for _, w := range v.widgets {
				if w.text == "FakeIP排除域名" && w.parent.pointer.Bounds().Y != control.pointer.Bounds().Y {
					t.Fatal("FakeIP exclusions label is not top aligned")
				}
			}
		}
	}
	u.field("dns.proxy").SetText("auto")
	u.prepareDNSResolver("proxy")
	u.render()
	pump()

	addPolicy := button("添加DNS策略")
	var format, order *widget
	for _, w := range v.widgets {
		switch {
		case w.text == "域名策略":
			t.Fatal("redundant domain policy label")
		case strings.HasSuffix(w.text, "或者点击"):
			format = w
		case strings.HasPrefix(w.text, "从上到下匹配"):
			order = w
		}
	}
	if format == nil || order == nil || format.parent != addPolicy.parent {
		t.Fatal("missing policy format, rule order or inline add button")
	}
	helpBox, addBox, orderBox := format.pointer.Bounds(), addPolicy.pointer.Bounds(), order.pointer.Bounds()
	if addBox.X <= helpBox.X+helpBox.W || orderBox.Y < addBox.Y+addBox.H || !hasClass(addPolicy.classes, "icon-only") {
		t.Fatal("policy help is out of order or add button is misplaced")
	}
	addPolicy.click()
	pump()
	if u.popup == nil || u.popup.kind != "dns-rule" {
		t.Fatal("policy popup")
	}
	v.inputs["dnsRuleValue"].edit("fixture.invalid")
	u.addDNSPolicy()
	pump()
	if u.popup != nil || !strings.Contains(u.field("dns.policy").Text(), "DOMAIN-SUFFIX,fixture.invalid,direct") {
		t.Fatal("policy builder")
	}
	u.saveDNS()
	wait()
	if saved := u.backend.GetState().Config.Settings.DNS; saved == nil || saved.Bootstrap != dnsBootstrapIP || u.editor != "" {
		t.Fatal("DNS save")
	}
	u.navigate("settings")
	pump()
	button("关于").click()
	pump()
	if u.popup == nil || u.popup.kind != "about" || v.popupRoot == nil {
		t.Fatal("About must be a popup")
	}
	update := button("检查更新")
	if !update.enabled {
		t.Fatal("update check is unavailable")
	}
	versions := map[string]bool{runtime.Version(): false, toolkitVersion(): false, appmeta.Name: false, appmeta.Version: false}
	for _, w := range v.widgets {
		if _, ok := versions[w.text]; ok {
			versions[w.text] = true
		}
		if strings.Contains(w.text, "项目目录") {
			t.Fatal("project path remains")
		}
	}
	for value, found := range versions {
		if !found {
			t.Fatal("missing About value", value)
		}
	}
	button(displayDataDir(u.state.DataDir)).click()
	wait()
	deadline := time.Now().Add(time.Second)
	var opened string
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(capture)
		opened = string(b)
		if strings.Contains(opened, u.state.DataDir) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(opened, u.state.DataDir+"\n") {
		t.Fatal("file-manager opener", opened)
	}
	about := u.popup
	committed := time.Date(2026, 11, 23, 2, 43, 2, 0, time.UTC)
	release := desktop.AppRelease{Tag: "v26.1123.104302", CommitTime: committed, Page: "https://github.com/zatxm/lanboat-app/releases/tag/v26.1123.104302"}
	u.showAppRelease(about, release, committed)
	pump()
	if u.popup.kind != "message" || u.popup.link != "当前没有可用的更新。" || u.popup.previous != about {
		t.Fatal("same commit should be reported as current")
	}
	u.closePopup()
	u.showAppRelease(about, release, committed.Add(-time.Second))
	pump()
	if u.popup.kind != "app-update" || u.popup.releaseTag != release.Tag || u.popup.previous != about {
		t.Fatal("new release dialog")
	}
	button("查看 GitHub 发布页").click()
	wait()
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(capture)
		opened = string(b)
		if strings.Contains(opened, release.Page+"\n") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(opened, release.Page+"\n") {
		t.Fatal("GitHub release opener", opened)
	}
	button("稍后").click()
	pump()
	if u.popup != about {
		t.Fatal("leaving the update dialog should return to About")
	}
	home, _ := os.UserHomeDir()
	if displayDataDir(filepath.Join(home, ".lanboat")) != "~/.lanboat" {
		t.Fatal("data directory abbreviation")
	}
	u.closePopup()
	pump()
	u.field("dns.report").SetText(strings.Repeat("DNS诊断结果\n", 80))
	u.popup = &popup{kind: "dns-report", title: "DNS路径检测"}
	u.notify()
	pump()
	scrollable := false
	for _, w := range v.widgets {
		if w.kind == "scroll" && w.expand && w != v.scroll && w != v.editorScroll {
			scrollable = true
		}
	}
	if !scrollable {
		t.Fatal("long diagnostic report must have an internal scroll area")
	}
	button("网页检测").click()
	opened = ""
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		b, _ := os.ReadFile(capture)
		if opened = string(b); strings.Contains(opened, "https://browserleaks.com/dns\n") {
			break
		}
	}
	if !strings.Contains(opened, "https://browserleaks.com/dns\n") {
		t.Fatal("diagnostic report browser link", opened)
	}
	u.closePopup()
	pump()
	if u.editor != "" || u.detail != "" || u.page != "settings" {
		t.Fatal("About changed navigation")
	}
}
