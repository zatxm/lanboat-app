package native

import (
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestHomeNodeExportAndCopy(t *testing.T) {
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

	export := v.exportNodes
	if export == nil || export.enabled || export.iconColor != lanboatTheme(v.theme).Muted {
		t.Fatal("export must be disabled and grey when there are no nodes")
	}
	finish := func() {
		t.Helper()
		waitJob(t, u)
		pump()
		if u.err != "" {
			t.Fatal(u.err)
		}
	}
	u.submit(func() error {
		return u.backend.SaveNode(profile.Node{
			Name: "Local", Server: "example.com", Port: 8388, Method: "aes-256-gcm", Password: "fixture",
		})
	}, nil)
	finish()
	if !export.enabled || v.exportNodes != export {
		t.Fatal("adding the first node did not enable export in place")
	}
	v.inputs["search"].edit("not found")
	v.inputs["search"].dispatch("submit", "")
	if !export.enabled {
		t.Fatal("an empty search must leave export enabled")
	}
	v.inputs["search"].edit("")
	v.inputs["search"].dispatch("submit", "")

	selected := u.state.Config.Selected
	for _, name := range []string{"Local-1", "Local-2"} {
		var actions []string
		var copyButton *widget
		for _, c := range v.controls {
			w := c.widget
			if w.kind == "button" && w.parent != nil && w.parent.text == "Local" {
				actions = append(actions, w.text)
				if w.text == "复制节点" {
					copyButton = w
				}
			}
		}
		if copyButton == nil || strings.Join(actions, ",") != "节点详情,分享节点,复制节点,删除节点" {
			t.Fatalf("local node actions are out of order: %v", actions)
		}
		copyButton.click()
		finish()
		nodes := u.state.Config.Nodes
		duplicate := nodes[len(nodes)-1]
		if duplicate.Name != name || duplicate.ID == selected || duplicate.Source != "" || u.state.Config.Selected != selected {
			t.Fatalf("unexpected duplicate or selection for %q", name)
		}
	}

	u.state.Config.Subscriptions = []profile.Subscription{{ID: "sub", Name: "Subscription"}}
	u.state.Config.Nodes = u.state.Config.Nodes[:1]
	u.state.Config.Nodes[0].Source = "sub"
	u.expanded["sub"] = true
	u.nodesDirty = true
	pump()
	var subscriptionActions []string
	for _, c := range v.controls {
		w := c.widget
		if w.kind == "button" && w.parent != nil && w.parent.text == "Local" {
			subscriptionActions = append(subscriptionActions, w.text)
		}
	}
	if strings.Join(subscriptionActions, ",") != "节点详情,分享节点,删除节点" || !export.enabled {
		t.Fatalf("subscription nodes must support export without a copy action: %v", subscriptionActions)
	}

	u.submit(u.backend.DeleteAllNodes, nil)
	finish()
	if export.enabled || v.exportNodes != export {
		t.Fatal("deleting the last node did not disable export in place")
	}
}

func TestHomeDeleteAllLocalNodes(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	const bundle = `{
		"format":"lanboat-nodes/v1", "selected":"local",
		"nodes":[
			{"id":"local","name":"Shown","server":"127.0.0.1","port":8388,"method":"aes-256-gcm","password":"fixture"},
			{"id":"orphan","name":"Hidden","server":"127.0.0.1","port":8388,"method":"aes-256-gcm","password":"fixture","source":"old-feed"},
			{"id":"remote","name":"Remote","server":"127.0.0.1","port":8388,"method":"aes-256-gcm","password":"fixture","source":"feed"}
		],
		"subscriptions":[{"id":"feed","name":"Feed","url":"https://example.invalid/feed"}],
		"ruleOverlays":[
			{"id":"local-rules","name":"Local","content":"[Rule]\nFINAL,DIRECT","nodeIds":["local"]},
			{"id":"feed","subscriptionId":"feed","name":"Feed","content":"[Rule]\nFINAL,DIRECT","nodeIds":["remote"]}
		]
	}`
	if _, err := u.backend.Import(bundle); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	pump()
	find := func(text string) *widget {
		for _, c := range v.controls {
			if c.widget.text == text {
				return c.widget
			}
		}
		return nil
	}
	v.inputs["search"].edit("Shown")
	v.inputs["search"].dispatch("submit", "")
	pump()
	deleteAll := find("删除所有本地节点")
	detect := find("延迟检测")
	if deleteAll == nil || detect == nil || deleteAll.parent != detect.parent || deleteAll.id <= detect.id ||
		!hasClass(deleteAll.classes, "icon-delete") || deleteAll.iconColor != lanboatTheme(v.theme).Danger {
		t.Fatal("local group must end with the red trash button")
	}
	deleteAll.click()
	pump()
	if u.popup == nil || u.popup.title != "节点" || confirmText(u.confirm) != "确定删除所有本地节点？" || len(u.backend.GetState().Config.Nodes) != 3 {
		t.Fatal("bulk deletion must wait for confirmation")
	}
	find("取消").click()
	pump()
	if u.popup != nil || len(u.backend.GetState().Config.Nodes) != 3 {
		t.Fatal("cancel changed nodes")
	}
	deleteAll.click()
	pump()
	confirm := find("确认")
	if confirm == nil || !hasClass(confirm.classes, "danger") {
		t.Fatal("missing destructive confirmation button")
	}
	confirm.click()
	waitJob(t, u)
	pump()
	config := u.backend.GetState().Config
	if u.err != "" || u.popup != nil || len(config.Nodes) != 1 || config.Nodes[0].ID != "remote" || config.Selected != "remote" || len(config.Subscriptions) != 1 {
		t.Fatal("bulk deletion must preserve and select the subscription node", u.err)
	}
	if len(config.RuleOverlays) != 1 || config.RuleOverlays[0].SubscriptionID != "feed" {
		t.Fatal("bulk deletion did not preserve the subscription rule overlay")
	}
	v.inputs["search"].edit("")
	v.inputs["search"].dispatch("submit", "")
	pump()
	if find("删除所有本地节点") != nil || find("删除订阅") == nil {
		t.Fatal("only subscription actions should remain")
	}
}

func TestNativeWindow(t *testing.T) {
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
	click := func(label string) {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == label || strings.HasPrefix(c.widget.text, label+"\t") {
				c.widget.click()
				pump()
				return
			}
		}
		t.Fatalf("missing control %q", label)
	}
	clickNodeDetail := func() {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == "节点详情" && c.widget.parent != nil && strings.Contains(c.widget.parent.classes, "node-row") {
				c.widget.click()
				pump()
				return
			}
		}
		t.Fatal("missing node detail icon")
	}
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for u.busy && time.Now().Before(deadline) {
			pump()
			time.Sleep(time.Millisecond)
		}
		pump()
		if u.busy {
			t.Fatal("UI action timed out")
		}
		if u.err != "" {
			t.Fatal(u.err)
		}
	}
	has := func(text string) bool {
		for _, w := range v.widgets {
			if strings.Contains(w.text, text) {
				return true
			}
		}
		return false
	}
	if len(v.nav) != 3 || v.back.visible || v.title.visible || v.header.visible || has("本地节点") || has("全屏") || has("还没有节点") || has("服务器") {
		t.Fatal("initial home navigation or empty local group")
	}
	if v.theme != "light" || u.state.Config.Settings.Mode != "rule" {
		t.Fatal("white theme or configuration routing is not default")
	}
	for _, w := range v.nav {
		x, _, width, _ := w.bounds()
		if x >= 64 || width != 44 || w.buttonCaption != nil {
			t.Fatal("navigation not in left sidebar")
		}
	}
	u.state.Config.Nodes = []profile.Node{{ID: "sub-node", Name: "only subscribed", Source: "demo"}}
	u.state.Config.Subscriptions = []profile.Subscription{{ID: "demo", Name: "唯一订阅"}, {ID: "second", Name: "第二个订阅"}}
	v.render()
	pump()
	if has("本地节点") || !has("唯一订阅") {
		t.Fatal("subscription-only data showed local section")
	}
	updates := 0
	for _, c := range v.controls {
		if c.widget.text == "更新订阅" {
			if !strings.Contains(c.widget.classes, "icon-only") {
				t.Fatal("subscription update has visible text")
			}
			updates++
		}
		if c.widget.text == "测试" || strings.Contains(c.widget.classes, "node-radio") {
			t.Fatal("individual test or radio remained")
		}
		if c.widget.text == "延迟检测" && !strings.Contains(c.widget.classes, "icon-only") {
			t.Fatal("group latency action has visible text")
		}
	}
	if updates != 2 {
		t.Fatal("home must have one update per subscription", updates)
	}
	for _, c := range v.controls {
		if c.widget.text != "更新订阅" {
			continue
		}
		parent := c.widget.parent
		var order []string
		for _, action := range v.controls {
			if action.widget.parent == parent && strings.Contains(action.widget.classes, "icon-only") {
				order = append(order, action.widget.text)
			}
		}
		if strings.Join(order, ",") != "更新订阅,自动选择,延迟检测,订阅详情,分享订阅,删除订阅" {
			t.Fatal("subscription action order", order)
		}
	}
	u.state.Config.Nodes[0].Source = "orphan"
	v.render()
	pump()
	if !has("本地节点") {
		t.Fatal("orphan source hid a local node")
	}
	u.state = u.backend.GetState()
	v.render()
	pump()
	click("导入")
	v.inputs["content"].edit(profile.URI(profile.Node{Name: "Imported SS", Server: "localhost", Port: 8388, Method: "aes-256-gcm", Password: "imported-private"}))
	click("保存")
	wait()
	if u.editor != "" || len(u.state.Config.Nodes) != 1 || u.state.Config.Nodes[0].Source != "" {
		t.Fatal("home import did not create local node")
	}
	click("添加节点")
	if u.editor != "node" || u.draft.Type != "subscribe" || v.inputs["subURL"] == nil || !v.back.visible {
		t.Fatal("unified subscription form missing")
	}
	v.inputs["name"].edit("HTTP3 local")
	v.inputs["protocol"].edit("http3")
	pump()
	if u.editor != "node" || u.draft.Kind() != "http3" || u.field("name").Text() != "HTTP3 local" {
		t.Fatal("protocol changed page or lost form")
	}
	for k, value := range map[string]string{"server": "example.com", "nodePort": "443", "password": "native-secret", "opt.username": "user"} {
		v.inputs[k].edit(value)
	}
	for _, c := range v.controls {
		if strings.Contains(c.widget.text, "导入") {
			t.Fatal("node form contains import action")
		}
	}
	bx, _, bw, _ := v.body.bounds()
	rx, _, _, _ := v.back.bounds()
	sx, _, sw, _ := v.save.bounds()
	if rx != bx || sx+sw != bx+bw || !strings.Contains(v.save.classes, "icon-check") {
		t.Fatal("header alignment or check icon", bx, bw, rx, sx, sw)
	}
	click("保存")
	wait()
	if u.editor != "" || len(u.state.Config.Nodes) != 2 || v.back.visible || !has("本地节点") {
		t.Fatal("save did not return home")
	}
	v.inputs["search"].edit("HTTP3")
	v.inputs["search"].dispatch("submit", "")
	pump()
	preferred := u.state.Config.Selected
	clickNodeDetail()
	if u.backend.GetState().Config.Selected != preferred {
		t.Fatal("details selected a node")
	}
	v.inputs["protocol"].edit("vless")
	pump()
	v.inputs["opt.uuid"].edit("12345678-1234-4234-8234-123456789012")
	click("保存")
	wait()
	clickNodeDetail()
	v.inputs["name"].edit("Renamed VLESS")
	click("保存")
	wait()
	var saved profile.Node
	for _, n := range u.state.Config.Nodes {
		if n.Kind() == "vless" {
			saved = n
		}
	}
	uri, err := u.backend.ExportNode(saved.ID)
	full, parseErr := profile.ParseURI(uri)
	if err != nil || parseErr != nil || full.TextOption("uuid") == "" {
		t.Fatal("masked edit lost UUID")
	}
	u.field("search").SetText("")
	u.navigate("home")
	pump()
	click("Renamed VLESS")
	wait()
	if u.state.Config.Selected != saved.ID {
		t.Fatal("row click did not select node")
	}
	if u.detail != "" || v.inputs["pickerSearch"] != nil || v.back.visible || !has("代理模式") {
		t.Fatal("routing controls must stay on home without a routing page")
	}
	if modes[0].value != "rule" || modes[0].label != "规则" || v.inputs["search"] == nil {
		t.Fatal("home routing name or icon search is missing")
	}
	for _, c := range v.controls {
		if c.widget.kind == "select" && c.config {
			if u.state.Config.Settings.Mode != "rule" {
				t.Fatal("default route is not selected")
			}
		}
	}
	click("自动选择")
	wait()
	if !u.state.Config.Settings.AutoSelectEnabled("") || has("回退") {
		t.Fatal("automatic selection not saved or fallback remained")
	}
	u.delays[saved.ID] = 188
	u.render()
	pump()
	list, delay, search, nextID := v.nodeContainer, v.delayLabels[saved.ID], v.inputs["search"], v.next
	x, y, width, height := delay.parent.bounds()
	u.settings("global")
	wait()
	if list != v.nodeContainer || delay != v.delayLabels[saved.ID] || search != v.inputs["search"] || nextID != v.next {
		t.Fatal("routing change rebuilt home widgets")
	}
	nx, ny, nw, nh := delay.parent.bounds()
	if x != nx || y != ny || width != nw || height != nh || delay.text != "188 ms" {
		t.Fatal("routing change moved nodes or discarded latency results")
	}
	if u.state.Config.Settings.Mode != "global" {
		t.Fatal("global route not saved")
	}
	if v.back.visible || u.detail != "" || !has("代理模式") {
		t.Fatal("changing route left home")
	}
	v.nav["settings"].click()
	pump()
	click("路由规则")
	click("测试规则")
	if !v.back.visible {
		t.Fatal("rule test page back missing")
	}
	click("返回")
	if u.page != "config" || u.detail != "" {
		t.Fatal("nested back skipped routing rules")
	}
	click("查看")
	if u.editor != "config-view" || v.save.visible {
		t.Fatal("default configuration is editable")
	}
	if strings.Contains(strings.ToLower(v.inputs["fileContent"].nativeText()), "shadowrocket") {
		t.Fatal("legacy label appeared in configuration editor")
	}
	if !v.back.visible {
		t.Fatal("configuration editor back button missing")
	}
	v.nav["settings"].click()
	pump()
	var theme *widget
	for _, c := range v.controls {
		if c.widget.kind == "select" && len(c.widget.options) == 2 && c.widget.options[0].value == "light" {
			theme = c.widget
			break
		}
	}
	if theme == nil {
		t.Fatal("theme popup selector missing")
	}
	theme.edit("dark")
	wait()
	if u.detail != "" || v.theme != "dark" {
		t.Fatal("theme opened a separate page or was not applied")
	}
	for _, page := range []string{"home", "data", "settings"} {
		v.nav[page].click()
		pump()
		if v.back.visible || v.title.visible || v.header.visible {
			t.Fatal("root page has a title or back", page)
		}
		for _, c := range v.controls {
			if c.widget.kind == "select" || hasClass(c.widget.classes, "search-field") {
				continue
			}
			k := c.widget.classes
			if !strings.Contains(k, "icon-") && !strings.Contains(k, "switch") && !strings.Contains(k, "node-row") && !strings.Contains(k, "disclosure") {
				t.Fatal("action icon missing", c.widget.text)
			}
		}
	}
	v.nav["home"].click()
	pump()
	count := len(v.widgets)
	for range 5 {
		v.nav["settings"].click()
		pump()
		v.nav["home"].click()
		pump()
	}
	if len(v.widgets) != count {
		t.Fatal("navigation leaked widgets")
	}
}

func TestLatePollDoesNotUndoCompletedAction(t *testing.T) {
	u := testUI(t)
	u.submit(func() error { return nil }, func() { u.status.Connected = true })
	staleVersion := u.version.Load()
	waitJob(t, u)
	u.post(update{version: staleVersion})
	u.drain()
	if !u.status.Connected {
		t.Fatal("late disconnected poll undid completed connection")
	}
}
