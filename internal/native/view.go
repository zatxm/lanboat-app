package native

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

const linuxWindowTitle = "⛵︎ Lanboat"

var pages = []option{{"home", "首页"}, {"data", "数据"}, {"settings", "设置"}}
var modes = []option{{"rule", "规则"}, {"global", "全局"}, {"tun", "全局+TUN"}, {"direct", "直连"}}

func init() {
	if runtime.GOOS != "linux" {
		modes = slices.DeleteFunc(modes, func(o option) bool { return o.value == "tun" })
	}
}

var ruleTypes = func() []option {
	opts := make([]option, 0, len(routingTypes))
	for _, r := range routingTypes {
		for id, name := range profile.RuleTypeNames {
			if name == r.value {
				opts = append(opts, option{id, r.label})
				break
			}
		}
	}
	return opts
}()
var actions = []option{{"direct", "直连"}, {"proxy", "代理"}, {"reject", "拒绝"}}

func (v *nativeView) box(class string, horizontal bool, gap int, expand bool, children ...*widget) *widget {
	w := v.create("box", props{classes: class, horizontal: horizontal, gap: gap, expand: expand})
	for _, c := range children {
		v.pack(w, c)
	}
	return w
}
func (v *nativeView) label(text, class string) *widget {
	return v.create("label", props{text: text, classes: class})
}

// User content never passes through the UI translation catalog.
func (v *nativeView) dataLabel(text, class string) *widget {
	w := v.create("label", props{classes: class})
	w.raw = true
	w.setText(text)
	return w
}
func (v *nativeView) spacer() *widget { return v.box("", false, 0, true) }
func (v *nativeView) button(text, kind string, config, disabled bool, action func()) *widget {
	w := v.create("button", props{text: text, disabled: disabled, classes: "button " + kind + actionIcon(text, kind), expand: strings.Contains(kind, "disclosure")})
	w.event = func(event, value string) {
		if !v.ui.busy {
			action()
		}
	}
	v.controls = append(v.controls, control{w, config, disabled})
	return w
}
func (v *nativeView) input(id, placeholder string, multiline, secret, readOnly bool) *widget {
	kind := "entry"
	if multiline {
		kind = "text"
	}
	w := v.create(kind, props{value: v.ui.field(id).Text(), placeholder: placeholder, secret: secret, readOnly: readOnly, expand: id == "fileContent" || id == "content" && v.ui.editor == "import"})
	w.event = func(event, value string) {
		if !readOnly {
			v.ui.field(id).SetText(value)
		}
	}
	v.inputs[id] = w
	return w
}
func (v *nativeView) field(id, title, placeholder string, secret bool) *widget {
	return v.box("field", false, 7, false, v.label(title, "field-label"), v.input(id, placeholder, false, secret, false))
}
func (v *nativeView) selectWidget(current string, options []option, config bool, changed func(string)) *widget {
	w := v.create("select", props{})
	w.setOptions(options, current)
	w.event = func(event, value string) {
		if !v.ui.busy {
			w.value = value
			changed(value)
		}
	}
	v.controls = append(v.controls, control{w, config, false})
	return w
}
func (v *nativeView) selectField(title, current string, options []option, changed func(string)) *widget {
	return v.box("field", false, 7, false, v.label(title, "field-label"), v.selectWidget(current, options, false, changed))
}
func (v *nativeView) icon(name string) *widget { return v.create("icon", props{text: name}) }
func (v *nativeView) buildRoot() {
	u := v.ui
	v.title = v.label("首页", "toolbar-title")
	v.back = v.button("返回", "header-back link", false, false, u.back)
	v.save = v.button("保存", "header-save primary icon-check", true, false, u.saveEditor)
	v.header = v.box("header", true, 16, false, v.back, v.title, v.spacer(), v.save)
	v.back.setVisible(false)
	v.save.setVisible(false)
	sidebar := v.box("sidebar", false, 10, false)
	for _, page := range pages {
		p := page
		w := v.create("button", props{text: p.label, classes: "button nav-item icon-" + map[string]string{"home": "home", "config": "folder", "data": "data", "settings": "settings"}[p.value]})
		w.event = func(event, value string) {
			if !u.busy {
				u.navigate(p.value)
			}
		}
		v.nav[p.value] = w
		v.pack(sidebar, w)
	}
	v.alert = v.label("", "alert-text error")
	v.notice = v.label("", "alert-text success")
	v.coreError = v.label("", "alert-text error")
	for i, w := range []*widget{v.alert, v.notice, v.coreError} {
		index := i
		level := "error"
		if i == 1 {
			level = "success"
		}
		w.setExpand(true)
		row := v.box("alert-row "+level, true, 8, false, w, v.button("关闭提示", "link icon-only icon-close banner-close", false, false, func() { v.dismissBanner(index) }))
		row.setVisible(false)
		w.setVisible(false)
		v.bannerRows[i] = row
	}
	v.body = v.box("", false, 22, false)
	v.main = v.box("main", false, 14, true, v.bannerRows[0], v.bannerRows[1], v.bannerRows[2], v.body)
	v.scroll = v.create("scroll", props{expand: true})
	v.pack(v.scroll, v.main)
	content := v.box("", false, 0, true, v.header, v.scroll)
	v.shell = v.box("shell", true, 0, false, sidebar, content)
	attachWindow(v)
}
func serverTypes() []option {
	out := []option{{"subscribe", "Subscribe"}}
	order := []string{"ss", "ssr", "shadowtls", "vmess", "vless", "relay", "socks5", "socks5s", "http", "https", "http2", "http3", "trojan", "hysteria", "hysteria2", "anytls", "tuic", "juicity", "ssh", "wireguard", "openconnect", "trusttunnel", "masque", "snell", "mieru", "brook", "lua"}
	for _, id := range order {
		p, ok := profile.ProtocolByID(id)
		if ok {
			out = append(out, option{id, p.Label})
		}
	}
	for _, id := range []string{"sudoku", "naive", "tailscale"} {
		p, ok := profile.ProtocolByID(id)
		if ok {
			out = append(out, option{id, p.Label})
		}
	}
	return out
}
func (u *UI) chooseProtocol(kind string) {
	n := profile.Node{Type: kind}
	if u.editor == "node" {
		n.ID = u.draft.ID
		n.Name = u.field("name").Text()
		n.Server = u.field("server").Text()
		if u.draft.Type != "subscribe" {
			n.Port, _ = strconv.Atoi(u.field("nodePort").Text())
		}
	}
	u.editNode(n)
}
func (v *nativeView) render() {
	u := v.ui
	u.traffic.Store(u.page == "data" && u.detail == "" && u.editor == "")
	u.networkPolling.Store(u.detail == "network-logs" && u.networkQuery.Load().(logQuery).Page == 0)
	v.applyTheme()
	u.dirty = false
	u.nodesDirty = false
	formOffset := 0
	if v.editorScroll != nil && !u.resetScroll {
		formOffset = v.editorScroll.scrollPosition(-1)
	}
	v.clear(v.body)
	v.editorScroll = nil
	v.body.setExpand(u.editor == "node" || u.editor == "config-file" || u.editor == "config-view" || u.editor == "config-import" || u.editor == "import" || u.detail == "routing-control" || u.detail == "network-logs" || u.detail == "dns-query" || u.detail == "routing-groups" || u.page == "rules")
	v.controls = []control{{v.back, false, false}, {v.save, true, false}}
	for _, row := range v.bannerRows {
		for _, w := range v.widgets {
			if w.parent == row && w.kind == "button" {
				v.controls = append(v.controls, control{w, false, false})
			}
		}
	}
	v.inputs = map[string]*widget{}
	v.delayLabels = map[string]*widget{}
	v.download = nil
	v.upload = nil
	v.totalDown = nil
	v.totalUp = nil
	v.connections = nil
	v.uptime = nil
	v.connection = nil
	v.power = nil
	v.routing = nil
	v.nodeContainer = nil
	v.pagination = nil
	v.logContainer = nil
	v.logSignature = ""
	titles := map[string]string{"config": "路由规则", "rules": "自定义规则", "logs": "代理日志"}
	title := titles[u.page]
	if u.editor != "" {
		title = map[string]string{"node": "节点详情", "import": "导入节点", "subscription": "订阅详情", "rule": "添加规则", "file-rule": "编辑规则", "routing-group": "代理分组", "config-file": "编辑配置", "config-view": "查看配置", "config-import": "新增规则配置", "restore-backup": "恢复备份", "dns-settings": "DNS设置"}[u.editor]
	}
	if u.editor == "rule" && u.rule.ID != "" {
		title = "编辑规则"
	}
	if u.editor == "file-rule" && u.field("route.line").Text() == "0" {
		title = "添加规则"
	}
	if u.editor == "node" && u.draft.ID == "" {
		title = "添加节点"
	}
	if u.detail != "" && u.editor == "" {
		if t, ok := map[string]string{"rule-test": "测试规则", "routing-control": "分流控制", "routing-groups": "代理分组", "dns": "DNS 诊断", "proxy": "代理日志", "backups": "备份", "test-settings": "连通性测试", "tcp-settings": "TCP", "udp-settings": "UDP", "proxy-settings": "代理", "dns-query": "查询诊断", "network-logs": "查看日志", "about": "关于"}[u.detail]; ok {
			title = t
		}
	}
	if strings.HasPrefix(u.detail, "subscription:") && u.editor == "" {
		title = "订阅详情"
	}
	v.title.setText(title)
	v.save.setVisible(u.editor != "" && u.editor != "config-view")
	child := u.editor != "" || u.detail != "" || u.page == "config" || u.page == "rules" || u.page == "logs"
	v.back.setVisible(child)
	v.title.setVisible(child)
	v.header.setVisible(child)
	for id, w := range v.nav {
		class := "button nav-item icon-" + map[string]string{"home": "home", "config": "folder", "data": "data", "settings": "settings"}[id]
		navPage := u.page
		if navPage == "rules" || navPage == "config" {
			navPage = "settings"
		}
		if navPage == "logs" {
			navPage = "data"
		}
		if id == navPage {
			class += " active"
		}
		w.setClasses(class)
	}
	if u.editor != "" {
		v.pack(v.body, v.editor())
	} else {
		switch u.page {
		case "home":
			v.home()
		case "config":
			v.configPage()
		case "data":
			v.dataPage()
		case "rules":
			v.rules()
		case "logs":
			v.networkLogPage()
		case "settings":
			v.settingsPage()
		}
	}

	if v.editorScroll != nil && formOffset > 0 {
		v.editorScroll.scrollPosition(formOffset)
	}
	v.show()
	v.refresh()
	if u.resetScroll {
		v.top()
		u.resetScroll = false
	}
	if w := v.inputs[u.focus]; w != nil {
		w.focus()
	}
	u.focus = ""
}
func (v *nativeView) flush() {
	u := v.ui
	if u.dirty {
		v.render()
		return
	}
	if u.nodesDirty && v.nodeContainer != nil {
		v.renderNodes()
		v.show()
		u.nodesDirty = false
	}
	v.refresh()
}
func (v *nativeView) refresh() {
	u := v.ui
	v.refreshBanners()
	v.syncPopup()
	v.applyTheme()
	v.refreshLanguage()
	for _, w := range v.widgets {
		w.setLoading(u.busy && w.id == u.busyControl || u.testing && w.id == u.testControl)
		if w.update != nil && !u.busy {
			w.update()
		}
	}
	if v.routing != nil && !u.busy {
		v.routing.setOptions(modes, u.state.Config.Settings.Mode)
	}
	for _, c := range v.controls {
		c.widget.setEnabled(!u.busy && !c.disabled && !(u.editor == "node" && u.nodeLocked(u.draft.ID) && c.widget != v.back && !strings.Contains(c.widget.classes, "banner-close")))
	}
	for _, w := range v.nav {
		w.setEnabled(!u.busy)
	}
	for _, w := range v.inputs {
		w.setEnabled(!u.busy || u.savingSettings)
		if u.editor == "node" && u.nodeLocked(u.draft.ID) {
			w.setEnabled(false)
		}
	}
	if v.nodeContainer != nil && !u.flagsBusy {
		var pending []string
		for _, host := range u.flagHosts {
			if !u.flags[host].until.After(time.Now()) {
				pending = append(pending, host)
			}
		}
		u.loadFlags(pending)
	}
	if v.power != nil {
		v.power.setEnabled(!u.busy && (u.status.Connected || u.state.Config.Selected != "" || u.state.Config.Settings.Mode == "direct"))
		power := "开启连接"
		if u.status.Connected {
			power = "断开连接"
		}
		class := "connection-title toolbar-label"
		if u.status.Connected {
			class += " green"
		}
		v.connection.setText("连接")
		v.connection.setClasses(class)

		v.power.setText(power)
		if u.status.Connected {
			v.power.setClasses("button switch switch-on")
		} else {
			v.power.setClasses("button switch")
		}
	}
	if v.download != nil {
		v.download.setText("↓ " + formatBytes(u.status.DownloadRate) + "/s")
		v.upload.setText("↑ " + formatBytes(u.status.UploadRate) + "/s")
		v.totalDown.setText("↓ " + formatBytes(u.status.Download))
		v.totalUp.setText("↑ " + formatBytes(u.status.Upload))
		v.connections.setText(strconv.Itoa(u.status.Connections))
		seconds := 0
		if u.status.Connected && u.status.StartedAt > 0 {
			seconds = max(0, int(time.Since(time.UnixMilli(u.status.StartedAt))/time.Second))
		}
		v.uptime.setText(fmt.Sprintf("%d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60))

	}

	for id, w := range v.delayLabels {
		w.setText(nodeDelayText(u.delays, id))
	}

	if v.logContainer != nil {
		v.refreshNetworkLogs()
	}
}
func (v *nativeView) home() {
	u := v.ui
	if strings.HasPrefix(u.detail, "subscription:") {
		v.subscriptionPage(strings.TrimPrefix(u.detail, "subscription:"))
		return
	}
	v.connection = v.label("连接", "connection-title toolbar-label")
	v.power = v.button("开启连接", "switch", false, u.state.Config.Selected == "" && u.state.Config.Settings.Mode != "direct", u.connect)
	connection := v.box("", true, 6, false, v.connection, v.power)
	route := v.selectWidget(u.state.Config.Settings.Mode, modes, true, func(mode string) { blurFocus(); u.settings(mode) })
	v.routing = route
	route.setClasses("compact")
	route.setTooltip("全局+TUN 接管系统 TCP/UDP 流量；连接时可能需要系统授权，具体取决于操作系统。")
	routing := v.box("", true, 6, false, v.label("代理模式", "field-label toolbar-label"), route)
	autoStyle := "switch"
	if u.state.Config.Settings.AutoSelect {
		autoStyle += " switch-on"
	}
	autoLabel := v.label("自动选择", "field-label toolbar-label")
	autoButton := v.button("自动选择", autoStyle, false, false, u.toggleAutoSelect)
	autoButton.update = func() {
		class := "button switch"
		if u.state.Config.Settings.AutoSelect {
			class += " switch-on"
		}
		autoButton.setClasses(class)
	}
	autoHint := "连接时检测并选择最低延迟节点；已连接时开启会立即检测并切换。直连模式不切换节点。"
	autoLabel.setTooltip(autoHint)
	autoButton.setTooltip(autoHint)
	autoSelect := v.box("", true, 6, false, autoLabel, autoButton)
	test := v.button("延迟测试", "link icon-only icon-test", false, false, u.testVisibleNodes)
	test.update = func() {
		if u.testing {
			test.setText("正在检测延迟…")
		} else {
			test.setText("延迟测试")
		}
	}
	importButton := v.button("导入", "link icon-only icon-upload", true, false, u.importNodes)
	addButton := v.button("添加节点", "link icon-only icon-add", true, false, func() { u.editNode(profile.Node{Type: "subscribe"}) })
	settings := v.box("", true, 18, false, connection, autoSelect, routing)
	actions := v.box("", true, 8, false, v.button("测速", "link icon-only icon-speed", false, false, func() { u.openExternal(u.state.Config.Settings.SpeedTarget()) }), test, v.button("导出节点", "link icon-only icon-export", false, false, func() { u.exportData("nodes", "") }), importButton, addButton)
	var nodeToolbar *widget
	if v.compactWindow() {
		nodeToolbar = v.box("home-toolbar", false, 8, false, settings, v.box("", true, 0, false, v.spacer(), actions))
	} else {
		nodeToolbar = v.box("home-toolbar", true, 4, false, settings, v.spacer(), actions)
	}
	search := v.create("entry", props{value: u.field("search").Text(), classes: "search-input", expand: true})
	search.event = func(event, value string) {
		u.field("search").SetText(value)
		u.nodePage = 0
		u.nodesDirty = true
		u.notify()
	}
	v.inputs["search"] = search
	searchField := v.box("search-field", true, 2, false, v.icon("search"), search)
	searchField.event = func(_, _ string) { search.focus() }
	v.controls = append(v.controls, control{searchField, false, false})
	v.nodeContainer = v.box("", false, 18, false)
	v.pagination = v.box("", true, 6, false)
	v.pack(v.body, nodeToolbar, searchField, v.nodeContainer, v.pagination)
	v.renderNodes()
}
func (v *nativeView) renderNodes() {
	u := v.ui
	// Controls belonging to discarded rows must not survive a page or filter change.
	old := map[*widget]bool{}
	for _, w := range v.widgets {
		for p := w.parent; p != nil; p = p.parent {
			if p == v.nodeContainer || p == v.pagination {
				old[w] = true
				break
			}
		}
	}
	kept := v.controls[:0]
	for _, c := range v.controls {
		if !old[c.widget] {
			kept = append(kept, c)
		}
	}
	v.controls = kept
	v.clear(v.nodeContainer)
	v.clear(v.pagination)
	v.delayLabels = map[string]*widget{}
	nodes := filterNodes(u.state.Config.Nodes, u.field("search").Text())
	start, end, page := pageBounds(len(nodes), u.nodePage)
	u.nodePage = page
	u.flagHosts = nil
	hosts := map[string]bool{}
	sources := map[string]bool{}
	for _, s := range u.state.Config.Subscriptions {
		sources[s.ID] = true
	}
	local := func(n profile.Node) bool { return n.Source == "" || !sources[n.Source] }
	groups := []profile.Subscription{{ID: "", Name: "本地节点"}}
	groups = append(groups, u.state.Config.Subscriptions...)
	for _, group := range groups {
		g := group
		count := 0
		for _, n := range nodes {
			if n.Source == g.ID || g.ID == "" && local(n) {
				count++
			}
		}
		if g.ID == "" && count == 0 {
			continue
		}
		expanded := u.expanded[g.ID] || u.field("search").Text() != ""
		panel := v.box("panel group", false, 0, false)
		groupButton := v.button(g.Name, func() string {
			if expanded {
				return "row-button disclosure disclosure-open"
			}
			return "row-button disclosure"
		}(), false, false, func() { u.expanded[g.ID] = !u.expanded[g.ID]; u.nodesDirty = true; u.notify() })
		groupButton.raw = g.ID != ""
		groupButton.localize()
		header := v.box("group-row", true, 8, false, groupButton, v.label(fmt.Sprintf("%d 个", count), "help"))
		if g.ID != "" {
			v.pack(header, v.button("更新订阅", "link icon-only icon-refresh", true, false, func() {
				u.submit(func() error { _, err := u.backend.RefreshSubscription(g.ID); return err }, func() { u.notice = "订阅已更新" })
			}))
		}
		v.pack(header, v.button("延迟检测", "link icon-only icon-test", false, count == 0, func() { u.testGroupNodes(g.ID) }))
		if g.ID != "" {
			v.pack(header, v.button("订阅详情", "link icon-only icon-info", false, false, func() { u.openDetail("subscription:" + g.ID) }))
			v.pack(header, v.button("分享订阅", "link icon-only icon-share", false, false, func() { u.share("subscription", g.ID, g.Name) }))
			v.pack(header, v.button("删除订阅", "link danger icon-only icon-delete", true, false, func() { u.confirm = &deletion{kind: "subscription", id: g.ID, name: g.Name}; u.notify() }))
		}
		v.pack(panel, header)
		if expanded {
			var lastRow *widget
			for _, n := range nodes[start:end] {
				if n.Source != g.ID && !(g.ID == "" && local(n)) {
					continue
				}
				n := n
				class := "node-row"
				selected := u.state.Config.Selected
				if u.status.Connected && u.status.ActiveNode != "" {
					selected = u.status.ActiveNode
				}
				if n.ID == selected {
					class += " node-selected"
				}
				delay := v.label(nodeDelayText(u.delays, n.ID), "help node-delay")
				v.delayLabels[n.ID] = delay
				if n.Server != "" && !hosts[n.Server] {
					u.flagHosts = append(u.flagHosts, n.Server)
					hosts[n.Server] = true
				}
				flag := v.countryIcon(n.Server)
				row := v.box(class, true, 8, false, flag, v.box("", false, 4, true, v.dataLabel(n.Name, "node-name node-title"), v.dataLabel(nodeDescription(n), "help")), delay, v.button("节点详情", "link icon-only icon-info", false, false, func() { u.editNode(n) }), v.button("分享节点", "link icon-only icon-share", false, false, func() { u.share("node", n.ID, n.Name) }), v.button("删除节点", "link danger icon-only icon-delete", true, false, func() { u.confirm = &deletion{kind: "node", id: n.ID, name: n.Name}; u.notify() }))
				row.update = func() {
					selected := u.state.Config.Selected
					if u.status.Connected && u.status.ActiveNode != "" {
						selected = u.status.ActiveNode
					}
					class := "node-row"
					if row == lastRow {
						class += " node-last"
					}
					if selected == n.ID {
						class += " node-selected"
					}
					row.setClasses(class)
				}
				row.text = n.Name
				row.event = func(event, value string) {
					if !u.locked() {
						u.submit(func() error { return u.backend.SelectNode(n.ID) }, nil)
					}
				}
				v.controls = append(v.controls, control{row, false, false})
				v.pack(panel, v.create("separator", props{}), row)
				lastRow = row
			}
		}
		v.pack(v.nodeContainer, panel)
	}
	v.pagination.setVisible(len(nodes) > pageSize)
	if len(nodes) > pageSize {
		v.pack(v.pagination, v.label(fmt.Sprintf("第 %d / %d 页 · %d 个节点", page+1, max(1, (len(nodes)+pageSize-1)/pageSize), len(nodes)), "help"), v.spacer(), v.button("上一页", "link", false, page == 0, func() { u.nodePage--; u.nodesDirty = true; u.notify() }), v.button("下一页", "link", false, end == len(nodes), func() { u.nodePage++; u.nodesDirty = true; u.notify() }))
	}
}

func nodeDelayText(delays map[string]int64, id string) string {
	ms, ok := delays[id]
	if !ok {
		return ""
	}
	if ms < 0 {
		return "超时"
	}
	return fmt.Sprintf("%d ms", ms)
}

func (v *nativeView) rules() {
	u := v.ui
	v.pack(v.body, v.box("", true, 10, false, v.label(fmt.Sprintf("分流规则  %d", len(u.state.Config.Rules)), "section-title"), v.spacer(), v.button("添加", "primary tight-icon", true, false, func() { u.editCustomRule(profile.Rule{Type: "domain_suffix", Action: "direct"}) })))
	list := v.box("", false, 0, false)
	start, end, pager := v.listRange("custom-rules", len(u.state.Config.Rules))
	for i := start; i < end; i++ {
		r := u.state.Config.Rules[i]
		i, r := i, r
		typeLabel := r.Type
		for _, kind := range ruleTypes {
			if kind.value == r.Type {
				typeLabel = kind.label
			}
		}
		if r.Disabled {
			typeLabel += " · " + v.translate("已停用")
		}
		save := func(rules []profile.Rule) { u.submit(func() error { return u.backend.SaveRules(rules) }, nil) }
		move := func(delta int) {
			rules := slices.Clone(u.state.Config.Rules)
			rules[i], rules[i+delta] = rules[i+delta], rules[i]
			save(rules)
		}
		toggle := "停用"
		if r.Disabled {
			toggle = "启用"
		}
		row := v.box("node-row", true, 8, false, v.label(strconv.Itoa(i+1), "help"), v.box("", false, 5, true, v.dataLabel(r.Value, "node-name node-title"), v.label(typeLabel, "help")), v.dataLabel(u.policyLabel(r.Action), "strategy"),
			v.button(toggle, "link icon-only icon-check", true, false, func() { rules := slices.Clone(u.state.Config.Rules); rules[i].Disabled = !r.Disabled; save(rules) }),
			v.button("编辑", "link icon-only icon-edit", true, false, func() { u.editCustomRule(r) }),
			v.button("上移", "link icon-only icon-up", true, i == 0, func() { move(-1) }), v.button("下移", "link icon-only icon-down", true, i == len(u.state.Config.Rules)-1, func() { move(1) }),
			v.button("删除", "link icon-only icon-delete", true, false, func() {
				u.confirm = &deletion{kind: "custom-rule", name: r.Value, run: func() { rules := slices.Clone(u.state.Config.Rules); save(append(rules[:i], rules[i+1:]...)) }}
				u.notify()
			}))
		if i > 0 {
			v.pack(list, v.create("separator", props{}))
		}
		v.pack(list, row)
	}
	if len(u.state.Config.Rules) == 0 {
		v.pack(list, v.box("empty", false, 0, false, v.label("暂无分流规则", "help")))
	}
	v.listViewport(list)
	if pager != nil {
		v.pack(v.body, pager)
	}
}
func (u *UI) editCustomRule(r profile.Rule) {
	u.rule = r
	u.field("ruleValue").SetText(r.Value)
	u.field("custom.noResolve").SetText(strconv.FormatBool(r.NoResolve))
	u.openEditor("rule")
}
func (v *nativeView) editor() *widget {
	u := v.ui
	full := u.editor == "node" || u.editor == "config-file" || u.editor == "config-view" || u.editor == "config-import" || u.editor == "import" || u.editor == "dns-settings"
	class := "panel editor"
	if u.editor == "dns-settings" {
		class = "editor"
	}
	if u.editor == "node" {
		class += " node-editor"
	}
	frame := v.box(class, false, 12, full)
	box := frame
	if u.editor == "node" {
		v.editorScroll = v.create("scroll", props{classes: "editor-scroll", expand: true})
		box = v.box("editor-content", false, 12, false)
		v.pack(v.editorScroll, box)
		v.pack(frame, v.editorScroll)
	}
	switch u.editor {
	case "node":
		password := "密码或密钥"
		if u.draft.Kind() == "ss" {
			password = "密码或 SS2022 密钥"
		}
		if u.draft.ID != "" {
			password = "密码（留空保留原密码）"
		}
		protocols := serverTypes()
		if u.draft.ID != "" && u.draft.Type != "subscribe" {
			protocols = protocols[1:]
		}
		if u.draft.ID != "" && u.draft.Type == "subscribe" {
			protocols = protocols[:1]
		}
		protocolWidget := v.selectWidget(u.draft.Type, protocols, false, u.chooseProtocol)
		if u.draft.Type == "" {
			protocolWidget.setOptions(protocols, "ss")
		}
		protocolWidget.setClasses("protocol-choice")
		v.inputs["protocol"] = protocolWidget
		v.pack(box, v.box("protocol-row", true, 12, false, v.label("协议类型", "field-label"), protocolWidget))
		if u.draft.Type == "subscribe" {
			v.subscriptionFields(box)
			break
		}
		if u.draft.Kind() == "ss" || u.draft.Kind() == "ssr" {
			v.shadowsocksFields(box)
			break
		}
		if profile.DetailedProtocol(u.draft.Kind()) {
			v.detailedNodeFields(box)
			break
		}
		v.pack(box, v.field("name", "备注", "", false))
		if u.draft.Kind() != "tailscale" {
			v.pack(box, v.box("", true, 15, false, v.box("", false, 0, true, v.field("server", "服务器（域名或 IP）", "", false)), v.field("nodePort", "端口", "", false)))
		}
		noPassword := map[string]bool{"vmess": true, "vless": true, "wireguard": true, "masque": true, "sudoku": true, "tailscale": true}
		if !noPassword[u.draft.Kind()] {
			v.pack(box, v.field("password", password, "", true))
		}
		p, _ := profile.ProtocolByID(u.draft.Kind())
		for _, spec := range p.Fields {
			v.pack(box, v.nodeField(spec))
		}
		if p.Core == "external" {
			v.pack(box, v.label("可保存此类型的配置；此版本尚未集成其连接内核。", "alert error"))
		}
		if u.draft.Kind() == "lua" {
			v.pack(box, v.button("载入 HTTP CONNECT 示例", "link", false, false, func() { u.field("opt.script").SetText(profile.LuaHTTPExample); u.render() }))
		}

	case "import":
		v.nodeImportFields(box)
	case "subscription":
		v.pack(box, v.field("subName", "订阅名称", "", false))
		v.pack(box, v.field("subURL", "HTTPS 订阅地址", "https://…", false))
		v.pack(box, v.label("支持多协议纯文本、Clash/JSON 和 Base64 自动解码。下载最长等待 20 秒。", "help"))
	case "config-file", "config-view":
		readOnly := u.editor == "config-view"
		v.pack(box, v.configNameField("default.conf", readOnly))
		v.pack(box, v.box("field", false, 8, true, v.label("配置内容", "field-label"), v.input("fileContent", "", true, false, readOnly)))
	case "dns-settings":
		v.dnsFields(box)
	case "config-import":
		v.configImportFields(box)
	case "restore-backup":
		v.pack(box, v.box("field", false, 8, false, v.label("备份文件路径或 JSON 内容（恢复会替换现有数据）", "field-label"), v.input("content", "", true, false, false)))
	case "file-rule":
		v.routingRuleFields(box)
	case "routing-group":
		v.routingGroupFields(box)
	case "rule":
		value := v.input("ruleValue", ruleHint(u.rule.Type), false, false, false)
		v.pack(box, v.selectField("匹配类型", u.rule.Type, ruleTypes, func(s string) { u.rule.Type = s; value.setPlaceholder(ruleHint(s)) }))
		v.pack(box, v.box("field", false, 7, false, v.label("匹配内容", "field-label"), value))
		v.pack(box, v.policySelect(u.rule.Action, func(s string) { u.rule.Action = s }), v.formSwitch("custom.noResolve", "不解析域名"))
	}
	if u.editor == "config-file" || u.editor == "config-view" || u.editor == "config-import" || u.editor == "import" || u.editor == "file-rule" || u.editor == "routing-group" || u.editor == "rule" || u.editor == "dns-settings" {
		return frame
	}
	if u.editor != "node" {
		v.pack(box, v.box("", true, 8, false, v.button("取消", "", false, false, u.back)))
	} else if u.draft.ID != "" && u.draft.Type == "subscribe" {
		id := u.draft.ID
		v.pack(box, v.box("", true, 8, false, v.button("删除订阅", "danger", true, false, func() {
			u.confirm = &deletion{kind: "subscription", id: id, name: u.draft.Name}
			u.notify()
		})))
	}
	return frame
}

func (v *nativeView) subscriptionFields(box *widget) {
	u := v.ui
	v.pack(box, v.field("subURL", "URL *", "https://…", false))
	row := v.box("", true, 24, false)
	for _, spec := range []struct{ id, title string }{{"subUDP", "UDP 转发"}, {"subInsecure", "允许不安全"}, {"subFastOpen", "TCP 快速打开"}} {
		w := v.button(spec.title, "switch", false, false, func() { u.field(spec.id).SetText(strconv.FormatBool(u.field(spec.id).Text() != "true")) })
		w.update = func() {
			classes := "button switch"
			if u.field(spec.id).Text() == "true" {
				classes += " switch-on"
			}
			w.setClasses(classes)
		}
		v.inputs[spec.id] = w
		v.pack(row, v.box("field", true, 12, false, v.label(spec.title, "field-label"), w))
	}
	v.pack(box, row)
	maximum := v.field("subMax", "最大值", "可选，1–100000", false)
	v.inputs["subMax"].setTooltip("如指定最大数量，但服务器返回多于这个数值会被随机丢弃多余的")
	v.pack(box, v.box("", true, 24, false, v.box("", false, 0, true, maximum), v.box("", false, 0, true, v.field("subUA", "用户代理 (UserAgent)", "可选", false))))
	password := "密码"
	if u.draft.ID != "" {
		password = "密码（留空保留原密码）"
	}
	v.pack(box, v.field("subPassword", password, "可选", true), v.field("subFilter", "过滤", "关键字或正则表达式", false))
	v.inputs["subPassword"].setTooltip("用于解密订阅内容，不会发送给订阅服务器")
	v.inputs["subFilter"].setTooltip("按节点备注匹配，忽略大小写")
	w := v.button("保留分流规则", "switch", false, false, func() { u.field("subKeepRules").SetText(strconv.FormatBool(u.field("subKeepRules").Text() != "true")) })
	w.update = func() {
		classes := "button switch"
		if u.field("subKeepRules").Text() == "true" {
			classes += " switch-on"
		}
		w.setClasses(classes)
	}
	v.inputs["subKeepRules"] = w
	v.pack(box, v.box("", true, 12, false, v.label("保留分流规则", "field-label"), w), v.field("name", "备注", "留空使用订阅域名", false))
}

func nodeDescription(n profile.Node) string {
	if n.Kind() == "ss" {
		return n.Method
	}
	p, _ := profile.ProtocolByID(n.Kind())
	return p.Label
}
