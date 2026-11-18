package native

import (
	"encoding/json"
	"fmt"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeWindowNetworkDataAndDNSQuery(t *testing.T) {
	if os.Getenv("LANBOAT_FLTK_TEST") != "1" {
		t.Skip("isolated display required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u := testUI(t)
	u.notify = requestIdle
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
	u.navigate("data")
	pump()
	for _, w := range v.widgets {
		if w.text == "服务器节点" || w.text == "代理" || w.text == "DNS" || strings.HasPrefix(w.text, "覆盖现有") {
			t.Fatal("removed data controls", w.text)
		}
	}
	find("记录代理日志").click()
	waitJob(t, u)
	pump()
	find("记录DNS日志").click()
	waitJob(t, u)
	pump()
	if !u.state.Config.Settings.ProxyLog || !u.state.Config.Settings.DNSLog {
		t.Fatal("recording switches")
	}
	find("完整请求日志（请求头和正文）").click()
	waitJob(t, u)
	pump()
	if !u.state.Config.Settings.ProxyLogFull {
		t.Fatal("full request logging switch was not persisted")
	}
	find("完整请求日志（请求头和正文）").click()
	waitJob(t, u)
	pump()
	if u.state.Config.Settings.ProxyLogFull {
		t.Fatal("full request logging switch was not disabled")
	}
	// Disk fixtures ensure the UI can reach records older than the previous 200-entry ring.
	dir := filepath.Join(u.state.DataDir, "logs")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var data []byte
	for i := 0; i < 617; i++ {
		b, _ := json.Marshal(traffic.Entry{Kind: "proxy", Time: time.Now().Format(time.RFC3339Nano), URL: fmt.Sprintf("request-%03d.invalid:443", i), Network: "TCP", Policy: "direct", Rule: "FINAL"})
		data = append(data, append(b, '\n')...)
	}
	if err = os.WriteFile(filepath.Join(dir, "proxy-2026-10-27.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	find("查看日志").click()
	waitJob(t, u)
	pump()
	if u.networkPage.Total != 617 || len(u.networkPage.Entries) != 40 || v.logContainer == nil || u.networkPolling.Load() != true {
		t.Fatal("network log first page")
	}
	body, list := v.body, v.logContainer
	find("下一页").click()
	waitJob(t, u)
	pump()
	if v.body != body || v.logContainer != list || u.networkPolling.Load() || u.networkPage.Number != 1 {
		t.Fatal("log pagination rebuilt page")
	}
	for i := 2; i < 16; i++ {
		find("下一页").click()
		waitJob(t, u)
		pump()
	}
	if u.networkPage.Entries[len(u.networkPage.Entries)-1].URL != "request-000.invalid:443" {
		t.Fatal("oldest log inaccessible")
	}
	find("日志详情").click()
	pump()
	if u.popup == nil || u.popup.kind != "network-log" {
		t.Fatal("log details")
	}
	u.closePopup()
	pump()
	u.back()
	pump()
	find("清空日志").click()
	pump()
	find("确认").click()
	waitJob(t, u)
	pump()
	if u.popup != nil || u.notice != "" {
		t.Fatal("delete success produced feedback")
	}
	page, err := u.backend.NetworkLogs("proxy", 0)
	if err != nil || page.Total != 0 {
		t.Fatal("clear logs", err)
	}
	u.navigate("settings")
	pump()
	u.openDNS()
	pump()
	u.field("dns.bootstrap").SetText("udp://9.9.9.9")
	find("查询诊断").click()
	pump()
	if u.editor != "" || u.detail != "dns-query" || v.title.text != "查询诊断" {
		t.Fatal("DNS query navigation", u.editor, u.detail)
	}
	ix, iy, iw, ih := v.inputs["dnsHost"].bounds()
	bx, by, _, bh := find("查询").bounds()
	if by != iy || bh != ih || bx <= ix+iw {
		t.Fatal("DNS query controls not aligned")
	}
	v.inputs["dnsHost"].edit("localhost")
	next := v.next
	find("查询").click()
	waitJob(t, u)
	pump()
	if u.result == "" || u.popup != nil || v.next != next {
		t.Fatal("DNS query refreshed page", u.err, u.result)
	}
	u.back()
	pump()
	if u.editor != "dns-settings" || u.field("dns.bootstrap").Text() != "udp://9.9.9.9" {
		t.Fatal("DNS query lost settings draft")
	}
	u.navigate("settings")
	pump()
	u.openProxy()
	pump()
	u.changeSettings(func(s *profile.Settings) {
		s.Proxy.AllowLAN = true
		s.Proxy.Authentication = true
		s.Proxy.Username = "shared-user"
		s.Proxy.Password = "shared-password"
	})
	waitJob(t, u)
	pump()
	found := false
	for _, w := range v.widgets {
		if w.kind == "entry" && w.nativeText() == "shared-password" {
			for p := w; p != nil; p = p.parent {
				if !p.visible {
					t.Fatal("password hidden")
				}
			}
			found = true
		}
	}
	if !found {
		t.Fatal("sharing password not shown as plaintext")
	}
	u.changeSettings(func(s *profile.Settings) { s.Language = "en"; s.Appearance = "dark" })
	waitJob(t, u)
	pump()
	assertEnglishWidgets(t, v)
	for i := 0; i < 45; i++ {
		u.state.Config.Files = append(u.state.Config.Files, profile.ConfigFile{ID: fmt.Sprintf("paged-%d", i), Name: fmt.Sprintf("paged-%d.conf", i), Content: "[Rule]\nFINAL,DIRECT"})
	}
	u.openPage("config")
	pump()
	if v.editorScroll == nil || u.field("page.config-files").Text() != "0" {
		t.Fatal("config file list is not paginated")
	}
	// English controls keep their source text for callbacks and tests.
	find("下一页").click()
	pump()
	found = false
	for _, w := range v.widgets {
		if w.text == "paged-44.conf" {
			found = true
		}
	}
	if !found || u.field("page.config-files").Text() != "1" {
		t.Fatal("last configuration inaccessible through pagination")
	}
}

func TestLogTimeAndCorePolicyLabels(t *testing.T) {
	if got := logTime("2026-01-02T03:04:05.006+0000"); !strings.HasPrefix(got, "2026-01-0") || !strings.HasSuffix(got, ":04:05.006") {
		t.Fatal("zap ISO 8601 time not parsed", got)
	}
	u := testUI(t)
	u.state.Config.Nodes = []profile.Node{{ID: "n1", Name: "Tokyo"}}
	for value, want := range map[string]string{"policy-node-n1": "Tokyo", "node:n1": "Tokyo", "policy-group-Streaming": "Streaming", "DIRECT": "直连", "reject": "拒绝"} {
		if got := u.policyLabel(value); got != want {
			t.Errorf("%s: got %q, want %q", value, got, want)
		}
	}
}
