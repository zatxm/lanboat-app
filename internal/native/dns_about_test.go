package native

import (
	"github.com/zatxm/lanboat-app/internal/appmeta"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeWindowDNSAndAbout(t *testing.T) {
	if os.Getenv("LANBOAT_FLTK_TEST") != "1" {
		t.Skip("isolated display required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
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
	v.inputs["dns.bootstrap"].edit("udp://9.9.9.9")
	button("还原默认").click()
	pump()
	if u.field("dns.bootstrap").Text() != "system" || u.field("dns.proxy").Text() != "auto" || u.notice != "" {
		t.Fatal("restore DNS defaults", u.field("dns.bootstrap").Text(), u.notice)
	}
	v.inputs["dns.bootstrap"].edit("https://loop.invalid/dns-query")
	u.saveDNS()
	pump()
	if (u.popup == nil || u.popup.kind != "message") || u.backend.GetState().Config.Settings.DNS != nil {
		t.Fatal("invalid draft was saved")
	}
	u.closePopup()
	v.inputs["dns.bootstrap"].edit("1.1.1.1")

	for _, w := range v.widgets {
		if w.kind == "select" && w.value == "auto" {
			for _, o := range w.options {
				if o.value == "custom" {
					w.event("change", "custom")
					break
				}
			}
		}
	}
	pump()
	changed := false
	for _, w := range v.widgets {
		if w.kind == "select" && w.value == "https" {
			w.event("change", "tcp")
			changed = true
			break
		}
	}
	pump()
	if !changed || u.field("dns.proxy").Text() != "tcp://1.1.1.1" {
		t.Fatal("protocol change retained an HTTP path", u.field("dns.proxy").Text())
	}
	u.field("dns.proxy").SetText("auto")
	u.prepareDNSResolver("proxy")
	u.render()
	pump()
	if u.field("dns.bootstrap").Text() != "1.1.1.1" {
		t.Fatal("resolver selection lost draft")
	}

	button("添加").click()
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
	if u.backend.GetState().Config.Settings.DNS == nil || u.editor != "" {
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
	if update.enabled {
		t.Fatal("updater placeholder enabled")
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
