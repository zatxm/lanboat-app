package native

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowRoutingLayout(t *testing.T) {
	u := testUI(t)
	if err := u.backend.SaveConfigFile("", "custom.conf", "[Rule]\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	pumpUI := func() {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			pump()
			if !u.busy {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("UI action timed out")
			}
			time.Sleep(time.Millisecond)
		}
		if u.err != "" {
			t.Fatal(u.err)
		}
	}
	find := func(text string) *widget {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == text || strings.HasPrefix(c.widget.text, text+"\t") {
				return c.widget
			}
		}
		t.Fatalf("missing control %q", text)
		return nil
	}
	v.render()
	pumpUI()
	for _, w := range v.widgets {
		if (w.kind == "icon" && w.text == "sail") || strings.Contains(w.classes, "brand-title") {
			t.Fatal("branding still occupies the sidebar")
		}
	}
	// Opening the XHTTP options makes the compact row layout exceed the viewport.
	n := profile.Node{Type: "vmess"}
	n.SetOption("network", "xhttp")
	u.editNode(n)
	pumpUI()
	scroll := v.editorScroll
	if scroll == nil {
		t.Fatal("node editor still uses page scrolling")
	}
	sx, sy, sw, sh := scroll.bounds()
	px, py, pw, ph := scroll.parent.bounds()
	if sx <= px || sy <= py || sx+sw >= px+pw || sy+sh >= py+ph || px+pw-sx-sw > 8 {
		t.Fatal("form scrollbar is not inside the content panel")
	}
	scroll.scrollPosition(250)
	pumpUI()
	before := scroll.scrollPosition(-1)
	if before == 0 {
		t.Fatal("long node form does not scroll")
	}
	v.inputs["opt.tls"].click()
	pumpUI()
	if v.editorScroll.scrollPosition(-1) != before || v.scroll.scrollPosition(-1) != 0 {
		t.Fatal("conditional fields moved the form or outer page")
	}
	u.navigate("settings")
	pumpUI()
	if len(v.nav) != 3 || v.nav["config"] != nil {
		t.Fatal("configuration still appears in the sidebar")
	}
	find("路由规则").click()
	pumpUI()
	if u.page != "config" || !strings.Contains(v.nav["settings"].classes, "active") {
		t.Fatal("routing rules did not open under settings")
	}
	var regionLabel, regionSelect, summary *widget
	for _, w := range v.widgets {
		if w.text == "国家和地区" && w.kind == "label" {
			regionLabel = w
		}
		if strings.Contains(w.classes, "region-select") {
			regionSelect = w
		}
		if strings.Contains(w.classes, "rule-update-summary") {
			summary = w
		}
		for _, removed := range []string{"规则库", "自动更新", "恢复默认配置", "规则模块", "本地文件", "[General]", "圆点代表", "正在使用"} {
			if strings.Contains(w.text, removed) {
				t.Fatal("obsolete copy remains", w.text)
			}
		}
		if w.parent != nil && strings.Contains(w.parent.classes, "node-row") && w.kind == "button" && !strings.Contains(w.classes, "icon-only") {
			t.Fatal("file action still displays text", w.text)
		}
	}
	if regionLabel == nil || regionSelect == nil || summary == nil || summary.text != "每天" {
		t.Fatal("rule database settings layout missing or incorrect")
	}
	labelBox := regionLabel.pointer.Bounds()
	selectBox := regionSelect.pointer.Bounds()
	centerDelta := selectBox.Y + selectBox.H/2 - (labelBox.Y + labelBox.H/2)
	if math.Abs(float64(centerDelta)) > 1 {
		t.Fatalf("country label and selector are on different lines: center offset=%g", centerDelta)
	}
	rx, _, rw, _ := find("default.conf").bounds()
	sx, _, sw, _ = find("default.conf").parent.bounds()
	if rx != sx || rw != sw {
		t.Fatalf("selected row leaves a gap: row=%d,%d scroll=%d,%d", rx, rw, sx, sw)
	}
	if !strings.Contains(find("default.conf").classes, "node-selected") {
		t.Fatal("active rules are not selected across the row")
	}
	find("custom.conf").click()
	pumpUI()
	if !strings.Contains(find("custom.conf").classes, "node-selected") || strings.Contains(find("default.conf").classes, "node-selected") {
		t.Fatal("whole-row activation failed")
	}
	find("查看").click()
	pumpUI()
	checkEditor := func(readOnly bool) {
		t.Helper()
		if !v.back.visible || v.save.visible == readOnly {
			t.Fatal("configuration editor navigation or save visibility")
		}
		_, y, _, height := v.inputs["fileContent"].bounds()
		_, bodyY, _, bodyH := v.body.bounds()
		if height < bodyH/2 || bodyY+bodyH-y-height > 32 || v.scroll.scrollPosition(-1) != 0 {
			t.Fatal("configuration content does not fill the available height", y, height, bodyY, bodyH)
		}
		for _, w := range v.widgets {
			if strings.HasPrefix(w.text, "默认配置为只读") || strings.HasPrefix(w.text, "保留 .conf") || w.text == "取消" {
				t.Fatal("obsolete editor footer", w.text)
			}
		}
	}
	checkEditor(true)
	beforeContent := u.field("fileContent").Text()
	v.inputs["fileContent"].edit("[Rule]\nFINAL,REJECT")
	if u.field("fileContent").Text() != beforeContent {
		t.Fatal("default rules became writable")
	}
	u.back()
	pumpUI()
	find("编辑").click()
	pumpUI()
	checkEditor(false)
	v.inputs["fileContent"].edit("[Rule]\nDOMAIN,fixture.invalid,REJECT\nFINAL,DIRECT")
	v.save.click()
	pumpUI()
	if u.editor != "" || u.page != "config" || !strings.Contains(u.state.Config.Files[1].Content, "fixture.invalid") {
		t.Fatal("rule editor did not save and return to the list")
	}
	find("新增规则配置").click()
	pumpUI()
	if u.editor != "config-import" || v.title.text != "新增规则配置" || !v.back.visible || u.field("importMethod").Text() != "manual" {
		t.Fatal("dedicated rule import page missing")
	}
	content := "[Rule]\nDOMAIN,preview.invalid,REJECT\nFINAL,DIRECT"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.Write([]byte("<html>error</html>"))
			return
		}
		w.Write([]byte(content))
	}))
	defer server.Close()
	v.inputs["importMethod"].edit("url")
	pumpUI()
	v.inputs["importURL"].edit(server.URL + "/preview.conf")
	find("解析").click()
	pumpUI()
	if u.field("fileContent").Text() != content || len(u.state.Config.Files) != 2 {
		t.Fatal("URL preview did not populate the content without saving")
	}
	v.inputs["importURL"].edit(server.URL + "/bad")
	find("解析").click()
	deadline := time.Now().Add(3 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if u.busy || (u.popup == nil || u.popup.kind != "message") || u.field("fileContent").Text() != content || u.editor != "config-import" {
		t.Fatal("bad preview lost draft or failed to show error")
	}
	u.closePopup()
	pump()
	v.inputs["importMethod"].edit("file")
	pumpUI()
	if !strings.Contains(find("选择文件").classes, "icon-upload") {
		t.Fatal("file upload must use the upward upload icon")
	}
	file := filepath.Join(t.TempDir(), "uploaded.conf")
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	u.previewConfig(file, "uploaded.conf")
	pumpUI()
	if v.inputs["fileContent"].nativeText() != content {
		t.Fatal("file preview did not populate the editor")
	}
	v.inputs["fileName"].edit("imported.conf")
	v.save.click()
	pumpUI()
	if u.editor != "" || len(u.state.Config.Files) != 3 {
		t.Fatal("preview could not be saved")
	}
	find("自定义规则").click()
	pumpUI()
	if v.routing != nil {
		t.Fatal("custom rules still contain route mode")
	}
	find("添加").click()
	pumpUI()
	for _, c := range v.controls {
		if c.widget.text == "取消" && c.widget.visible {
			t.Fatal("add rule still has cancel")
		}
	}
	v.inputs["ruleValue"].edit("custom.invalid")
	v.save.click()
	pumpUI()
	if u.editor != "" || u.page != "rules" || len(u.state.Config.Rules) != 1 || u.notice != "" {
		t.Fatal("add rule did not return quietly to the list")
	}
	for _, c := range v.controls {
		if c.widget.text == "上移" || c.widget.text == "删除" {
			if !strings.Contains(c.widget.classes, "icon-only") {
				t.Fatal("custom rule action has text")
			}
		}
	}
	find("删除").click()
	pumpUI()
	if u.popup == nil || u.popup.kind != "confirm" {
		t.Fatal("custom rule deletion must be confirmed in a popup")
	}
	find("确认").click()
	pumpUI()
	if len(u.state.Config.Rules) != 0 {
		t.Fatal("custom rule delete failed")
	}
}
