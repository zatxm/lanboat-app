package native

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNativeWindowSettingsRevision(t *testing.T) {
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
	button := func(text string) *widget {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == text || strings.HasPrefix(c.widget.text, text+"\t") {
				return c.widget
			}
		}
		t.Fatal("missing", text)
		return nil
	}
	u.navigate("settings")
	v.render()
	pump()
	for _, c := range v.controls {
		if strings.HasPrefix(c.widget.text, "诊断\t") {
			t.Fatal("diagnostics remains in settings")
		}
	}
	dns := button("DNS设置")
	if !strings.Contains(dns.classes, "settings-align") {
		t.Fatal("DNS row not aligned")
	}
	dx, _, _, _ := dns.bounds()
	if px, _, _, _ := button("代理").bounds(); px != dx {
		t.Fatal("DNS and proxy rows are not aligned")
	}
	button("关于").click()
	pump()
	found := false
	for _, w := range v.widgets {
		if w.text == "内存" {
			found = true
		}
	}
	if !found {
		t.Fatal("About lacks memory")
	}
	u.status.Memory.RSS = 123456789
	v.refresh()
	found = false
	for _, w := range v.widgets {
		if w.text == formatBytes(123456789) {
			found = true
		}
	}
	if !found {
		t.Fatal("About memory did not update")
	}
	if !strings.Contains(button(displayDataDir(u.state.DataDir)).classes, "flush-left") {
		t.Fatal("folder icon alignment")
	}
	folder := button(displayDataDir(u.state.DataDir))
	fx, _, _, _ := folder.bounds()
	for _, w := range v.widgets {
		if w.text == runtime.Version() {
			x, _, _, _ := w.bounds()
			if fx != x {
				t.Fatal("folder icon is not aligned with About values", fx, x)
			}
		}
	}
	u.closePopup()
	pump()
	u.openDNS()
	pump()
	for _, c := range v.controls {
		if c.widget.text == "高级设置" {
			t.Fatal("obsolete advanced switch")
		}
	}
	leak, query, reset := button("泄漏检测"), button("查询诊断"), button("还原默认")
	lx, _, lw, _ := leak.bounds()
	qx, _, qw, _ := query.bounds()
	rx, _, _, _ := reset.bounds()
	if qx-lx-lw > 8 || rx-qx-qw > 8 {
		t.Fatal("DNS toolbar actions too far apart")
	}
	u.field("dns.bootstrap").SetText("udp://9.9.9.9")
	u.resetDNSDefaults()
	pump()
	if u.notice != "" || u.field("dns.bootstrap").Text() != "system" {
		t.Fatal("DNS reset not quiet")
	}
	u.back()
	pump()
	if err = u.backend.SaveConfigFile("", "delete-me.conf", "[Rule]\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	u.openPage("config")
	pump()
	button("删除配置").click()
	pump()
	if u.popup == nil || u.popup.kind != "confirm" {
		t.Fatal("file delete must be a popup")
	}
	for _, name := range []string{"取消", "确认"} {
		if !strings.Contains(button(name).classes, map[string]string{"取消": "secondary", "确认": "primary"}[name]) {
			t.Fatal("confirmation button not filled", name)
		}
	}
	if !strings.Contains(button("确认").classes, "danger") {
		t.Fatal("delete confirmation must be red")
	}
	button("取消").click()
	pump()
	if len(u.backend.GetState().Config.Files) != 2 {
		t.Fatal("cancel deleted file")
	}
	button("删除配置").click()
	pump()
	button("确认").click()
	waitJob(t, u)
	pump()
	if u.err != "" || len(u.backend.GetState().Config.Files) != 1 || u.popup != nil {
		t.Fatal("confirmed deletion failed", u.err)
	}
	u.closePopup()
	pump()
	button("复制").click()
	waitJob(t, u)
	pump()
	if len(u.state.Config.Files) != 2 || u.notice != "" {
		t.Fatal("rule copy must refresh quietly", u.err, u.notice)
	}
	u.navigate("settings")
	pump()
	u.openBackups()
	pump()
	for _, c := range v.controls {
		if c.widget.text == "从文件还原" {
			t.Fatal("file restore action remains")
		}
	}
	button("立即备份").click()
	waitJob(t, u)
	pump()
	if len(u.state.Backups) != 1 || u.notice != "" {
		t.Fatal("settings backup")
	}
	button("导入备份")
	button("立即备份").click()
	waitJob(t, u)
	pump()
	if len(u.state.Backups) != 2 {
		t.Fatal("second settings backup")
	}
	button("删除备份").click()
	pump()
	if u.popup == nil || u.popup.kind != "confirm" {
		t.Fatal("backup delete must be a popup")
	}
	button("取消").click()
	pump()
	if len(u.backend.Backups()) != 2 {
		t.Fatal("cancel removed backup")
	}
	button("删除备份").click()
	pump()
	button("确认").click()
	waitJob(t, u)
	pump()
	if len(u.backend.Backups()) != 1 || u.notice != "" {
		t.Fatal("backup not deleted")
	}
}
