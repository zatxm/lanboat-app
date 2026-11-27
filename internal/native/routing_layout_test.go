package native

import (
	"fmt"
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
		if w.text == "国家/地区" && w.kind == "label" {
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
	for code, want := range map[string]string{"hk": "中国香港", "mo": "中国澳门", "tw": "中国台湾"} {
		found := false
		for _, item := range regionSelect.options {
			found = found || item.value == code && item.label == strings.ToUpper(code)+" · "+want
		}
		if !found {
			t.Fatalf("region %s should display %s", code, want)
		}
	}
	if labelBox.X <= find("自定义规则").pointer.Bounds().X || !hasClass(find("新增规则配置").classes, "icon-only") || !hasClass(find("设置").classes, "icon-only") {
		t.Fatal("routing toolbar action order or icon styling")
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
	nameBox := v.inputs["fileName"].pointer.Bounds()
	for _, key := range []string{"importMethod", "importURL"} {
		r := v.inputs[key].pointer.Bounds()
		if r.X != nameBox.X || r.W <= 0 || (r.W == nameBox.W) != (key == "importMethod") {
			t.Fatalf("configuration field %s does not align within its row: %v vs %v", key, r, nameBox)
		}
	}
	inline := func(input, button *widget) {
		t.Helper()
		x, y, w, h := input.bounds()
		bx, by, _, bh := button.bounds()
		if bx <= x+w || math.Abs(float64((2*y+h)-(2*by+bh))) > 2 {
			t.Fatalf("button %q does not follow its field on the same row", button.text)
		}
	}
	inline(v.inputs["importURL"], find("解析"))
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
	inline(v.inputs["importMethod"], find("选择文件"))
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
	find("测试规则").click()
	pumpUI()
	ip := v.inputs["matchIP"]
	var ipHint *widget
	for _, w := range v.widgets {
		if strings.HasPrefix(w.text, "需要测试IP规则时") {
			ipHint = w
		}
	}
	ipBox := ip.pointer.Bounds()
	if ipHint == nil || ipHint.pointer.Bounds().Y < ipBox.Y+ipBox.H || ipHint.pointer.Bounds().X != ipBox.X || !strings.HasSuffix(ipHint.text, "，域名可不填") {
		t.Fatal("IP-only help should be under the IP field")
	}
	ip.edit("203.0.113.17")
	find("测试规则").click()
	pumpUI()
	if u.result == "" || u.popup != nil {
		t.Fatal("IP-only rule test should produce a result")
	}
	u.back()
	pumpUI()
	find("自定义规则").click()
	pumpUI()
	if v.routing != nil {
		t.Fatal("custom rules still contain route mode")
	}
	find("添加").click()
	pumpUI()
	if u.page != "rules" || u.editor != "" || u.popup == nil || u.popup.title != "添加规则" {
		t.Fatal("add rule should be a popup on the list")
	}
	for _, c := range v.controls {
		if c.widget.text == "取消" && c.widget.visible {
			t.Fatal("add rule still has cancel")
		}
	}
	v.inputs["ruleValue"].edit("custom.invalid")
	find("确定").click()
	pumpUI()
	if u.popup != nil || u.editor != "" || u.page != "rules" || len(u.state.Config.Rules) != 1 || u.notice != "" {
		t.Fatal("add rule did not return quietly to the list")
	}
	find("停用").click()
	pumpUI()
	if enable := find("启用"); enable.icon != "close" || enable.iconColor != lanboatTheme(v.theme).Danger {
		t.Fatal("disabled rule should show a red cross")
	}
	find("编辑").click()
	pumpUI()
	if u.popup == nil || u.popup.title != "编辑规则" || v.inputs["ruleValue"].nativeText() != "custom.invalid" {
		t.Fatal("edit rule should show the existing value in a popup")
	}
	v.inputs["ruleValue"].edit("changed.invalid")
	find("确定").click()
	pumpUI()
	if u.popup != nil || len(u.state.Config.Rules) != 1 || u.state.Config.Rules[0].Value != "changed.invalid" || !u.state.Config.Rules[0].Disabled {
		t.Fatal("edit did not preserve rule identity and disabled state")
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

func TestCustomRulesPaginationBoundary(t *testing.T) {
	u := testUI(t)
	u.page = "rules"
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()
	clickPage := pageClicker(t, v)
	for i := range pageSize {
		u.state.Config.Rules = append(u.state.Config.Rules, profile.Rule{Type: "domain", Value: fmt.Sprintf("rule-%d.invalid", i), Action: "direct"})
	}
	v.render()
	for _, c := range v.controls {
		if c.widget.classes == "page-bar" {
			t.Fatal("40 rules fit on one page")
		}
	}
	u.state.Config.Rules = append(u.state.Config.Rules, profile.Rule{Type: "domain", Value: "last.invalid", Action: "direct"})
	v.render()
	clickPage("Next page")
	v.flush()
	if u.field("page.custom-rules").Text() != "1" {
		t.Fatal("41st rule should be on the second page")
	}
	for _, w := range v.widgets {
		if w.text == "last.invalid" {
			return
		}
	}
	t.Fatal("second page is missing the last rule")
}
