package native

import (
	"context"
	"fmt"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func (v *nativeView) row(title, value string, locked bool, action func()) *widget {
	if action != nil {
		return v.button(title+"\t"+value, "row-button action-row chevron"+actionIcon(title, ""), locked, false, action)
	}
	label := v.label(title, "node-name")
	if value != "" {
		label = v.box("row-label-value", true, 10, false, label, v.label(value, "help inline-value"))
	}
	return v.box("group-row", true, 12, false, v.icon(iconName(title)), label, v.spacer())
}
func (v *nativeView) boundRow(title string, value func() string, locked bool, action func()) *widget {
	w := v.row(title, value(), locked, action)
	w.update = func() { w.setText(title + "\t" + value()) }
	return w
}
func (v *nativeView) boundSetting(title string, value func() string, locked bool, action func()) *widget {
	w := v.boundRow(title, value, locked, action)
	w.setClasses(w.classes + " settings-row")
	return w
}
func onOff(b bool) string {
	if b {
		return "开启"
	}
	return "关闭"
}

func (v *nativeView) settingsRow(title, value string, locked bool, action func()) *widget {
	w := v.row(title, value, locked, action)
	w.setClasses(w.classes + " settings-row")
	return w
}
func (v *nativeView) group(title string, rows ...*widget) {
	if title != "" {
		v.pack(v.body, v.label(title, "section-title"))
	}
	group := v.box("panel group", false, 0, false)
	for i, row := range rows {
		if i > 0 {
			v.pack(group, v.create("separator", props{}))
		}
		v.pack(group, row)
	}
	v.pack(v.body, group)
}
func (u *UI) editConfigFile(f profile.ConfigFile) {
	u.fileID = f.ID
	u.field("fileName").SetText(f.Name)
	u.field("fileContent").SetText(displayConfigContent(f.Content))
	if f.ID == "default" {
		u.openEditor("config-view")
	} else {
		u.openEditor("config-file")
	}
}
func (u *UI) exportData(kind, id string) {
	directory := chooseDirectory("选择保存目录")
	if directory == "" {
		return
	}
	var saved string
	u.submit(func() error { var err error; saved, err = u.backend.ExportDataTo(kind, id, directory); return err }, func() { u.notice = fmt.Sprintf("已导出到：\n%s", saved) })
}

func (v *nativeView) configPage() {
	u := v.ui
	if u.detail == "routing-control" {
		v.routingControlPage()
		return
	}
	if u.detail == "routing-groups" {
		v.routingGroupsPage()
		return
	}
	if u.detail == "rule-test" {
		v.routingTestPage()
		return
	}
	v.pack(v.body, v.box("routing-toolbar", true, 12, false,
		v.button("测试规则", "link toolbar-button icon-test", false, false, func() { u.openDetail("rule-test") }),
		v.button("自定义规则", "link toolbar-button icon-route", false, false, func() { u.openPage("rules") }), v.spacer(),
		v.button("新增规则配置", "primary toolbar-button icon-add", true, false, u.importConfig)))
	autoRegion := v.translate("自动匹配")
	if region := ruledata.Region(""); region != "" {
		autoRegion += " · " + strings.ToUpper(region)
	}
	countryOptions := []option{{"", autoRegion}}
	regions := ruledata.Regions(u.state.Config.Settings.Language)
	codes := make([]string, 0, len(regions))
	for code := range regions {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		countryOptions = append(countryOptions, option{code, strings.ToUpper(code) + " · " + regions[code]})
	}
	country := v.selectWidget(u.state.Config.Settings.RuleRegion, countryOptions, false, func(value string) { u.changeSettings(func(s *profile.Settings) { s.RuleRegion = value }) })
	country.update = func() { country.setOptions(countryOptions, u.state.Config.Settings.RuleRegion) }
	country.setClasses("region-select")
	country.setTooltip("自动匹配根据系统语言和时区推测国家和地区。")
	updateNow := v.button("立即更新", "link icon-only icon-refresh", false, false, func() {
		u.submit(func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			return u.backend.UpdateRuleData(ctx)
		}, func() { u.notice = "规则库已更新" })
	})
	summary := v.dataLabel("", "help node-title rule-update-summary")
	summary.update = func() {
		settings := u.state.Config.Settings
		if settings.RulesAutoUpdate() {
			if settings.RulesUpdateDays() == 1 {
				summary.setText(v.translate("每天"))
			} else {
				summary.setText(fmt.Sprintf(v.translate("每%d天更新"), settings.RulesUpdateDays()))
			}
		} else {
			summary.setText(v.translate("不自动更新"))
		}
	}
	v.group("", v.box("group-row", true, 6, false,
		v.label("国家和地区", "field-label tight-label"), country, v.spacer(), summary,
		v.button("设置", "secondary icon-settings", false, false, u.editRuleUpdate), updateNow))
	list := v.box("routing-files", false, 0, false)
	start, end, pager := v.listRange("config-files", len(u.state.Config.Files))
	for i := start; i < end; i++ {
		f := u.state.Config.Files[i]
		p, err := routing.Parse(f.Content)
		info := fmt.Sprintf("%d 条规则 · %s", len(p.Rules), formatBytes(int64(len(f.Content))))
		if err != nil {
			info = err.Error()
		}
		edit, icon := "编辑", "edit"
		if f.ID == "default" {
			edit, icon = "查看", "info"
		}
		rowClass := "node-row routing-file-row"
		if i == start {
			rowClass += " routing-file-first"
		}
		if i == end-1 {
			rowClass += " routing-file-last"
		}
		row := v.box(rowClass, true, 8, false, v.box("", false, 5, true, v.dataLabel(f.Name, "node-name node-title"), v.label(info, "help")), v.button("分流控制", "link icon-only icon-route", false, false, func() { u.openRoutingFile(f) }), v.button(edit, "link icon-only icon-"+icon, f.ID != "default", false, func() { u.editConfigFile(f) }), v.button("复制", "link icon-only icon-copy", true, false, func() {
			u.submit(func() error { return u.backend.CopyConfigFile(f.ID) }, func() { u.notice = ""; u.render() })
		}), v.button("导出", "link icon-only icon-export", false, false, func() { u.exportData("config", f.ID) }))
		row.text = f.Name
		row.update = func() {
			class := rowClass
			if f.ID == u.state.Config.ActiveFile {
				class += " node-selected"
			}
			row.setClasses(class)
		}
		row.event = func(event, value string) {
			if !u.locked() && f.ID != u.state.Config.ActiveFile {
				u.submit(func() error { return u.backend.ActivateConfigFile(f.ID) }, nil)
			}
		}
		v.controls = append(v.controls, control{row, false, false})
		if f.ID != "default" {
			button := v.button("删除配置", "link icon-only icon-delete", true, false, func() { u.confirm = &deletion{kind: "file", id: f.ID, name: f.Name}; u.notify() })
			button.update = func() { button.setVisible(f.ID != u.state.Config.ActiveFile) }
			v.pack(row, button)
		}
		if i > start {
			v.pack(list, v.create("separator", props{}))
		}
		v.pack(list, row)
	}
	v.pack(v.body, v.box("panel group list-frame routing-file-frame", false, 0, false, list))
	if pager != nil {
		v.pack(v.body, pager)
	}
}
func (v *nativeView) dataPage() {
	u := v.ui
	if u.detail == "network-logs" {
		v.networkLogPage()
		return
	}
	v.uptime = v.label("", "stat-value")
	v.connections = v.label("", "stat-value")
	v.download = v.label("", "stat-traffic")
	v.upload = v.label("", "stat-traffic")
	v.totalDown = v.label("", "stat-traffic")
	v.totalUp = v.label("", "stat-traffic")
	card := func(icon, title string, values ...*widget) *widget {
		caption := v.dataLabel("", "node-name")
		caption.update = func() {
			key := title
			if key == "连接" && u.state.Config.Settings.Language == "en" {
				key = "连接数"
			}
			caption.setText(v.translate(key))
		}
		caption.update()
		header := v.box("", true, 8, false, v.create("icon", props{text: icon, classes: "stat-icon"}), caption)
		return v.box("panel stat", false, 10, true, append([]*widget{header}, values...)...)
	}
	cards := []*widget{
		card("clock", "连接时间", v.uptime),
		card("monitor", "连接", v.connections),
		card("pie", "总流量", v.totalUp, v.totalDown),
		card("speed", "速度", v.upload, v.download),
	}
	if v.compactWindow() {
		v.pack(v.body, v.box("stats-grid", true, 14, false, cards[:2]...), v.box("stats-grid", true, 14, false, cards[2:]...))
	} else {
		v.pack(v.body, v.box("stats-grid", true, 14, false, cards...))
	}

	v.pack(v.body, v.box("", true, 8, false, v.label("日志", "section-title"), v.spacer(), v.button("查看日志", "link icon-only icon-logs", false, false, u.openNetworkLogs), v.button("清空日志", "link icon-only icon-delete", false, false, u.confirmClearLogs)))
	retention := v.selectWidget(strconv.Itoa(u.state.Config.Settings.LogRetention), []option{{"0", "关闭"}, {"1", "1 天"}, {"3", "3 天"}, {"7", "7 天"}, {"30", "30 天"}}, false, func(value string) {
		days, _ := strconv.Atoi(value)
		u.changeSettings(func(s *profile.Settings) { s.LogRetention = days })
	})
	retention.setClasses("compact")
	retention.update = func() { retention.setOptions(retention.options, strconv.Itoa(u.state.Config.Settings.LogRetention)) }
	toggle := func(title, hint string, value func() bool, change func(*profile.Settings)) *widget {
		b := v.button(title, "switch", false, false, func() { u.changeSettings(change) })
		b.update = func() {
			class := "button switch"
			if value() {
				class += " switch-on"
			}
			b.setClasses(class)
		}
		text := v.box("", false, 4, true, v.label(title, "node-name"), v.label(hint, "help"))
		return v.box("group-row", true, 12, false, text, b)
	}
	proxy := toggle("记录代理日志", "每个连接结束时记录目标、匹配规则、出站、耗时和流量；被拒绝或失败的连接立即记录", func() bool { return u.state.Config.Settings.ProxyLog }, func(s *profile.Settings) { s.ProxyLog = !s.ProxyLog })
	dns := toggle("记录DNS日志", "记录每次DNS查询的域名、类型、结果和耗时", func() bool { return u.state.Config.Settings.DNSLog }, func(s *profile.Settings) { s.DNSLog = !s.DNSLog })
	v.group("", v.box("group-row", true, 12, false, v.box("", false, 4, true, v.label("自动删除", "node-name"), v.label("超过保留天数的代理、DNS和应用日志会被删除", "help")), retention), proxy, dns)
}
func (u *UI) changeSettings(change func(*profile.Settings)) {
	if u.busy {
		return
	}
	s := u.state.Config.Settings
	change(&s)
	notice := u.notice
	u.savingSettings = true
	u.submit(func() error { return u.backend.SaveSettings(s) }, func() { u.state.Config.Settings = s })
	u.notice = notice
}
func (v *nativeView) settingsPage() {
	if v.ui.detail == "backups" {
		v.backupsPage()
		return
	}
	u := v.ui
	if u.detail == "proxy-settings" {
		v.proxyPage()
		return
	}
	if u.detail == "dns-query" {
		v.dnsQueryPage()
		return
	}
	if u.detail == "test-settings" {
		v.connectivityPage()
		return
	}
	if u.detail == "udp-settings" {
		v.udpPage()
		return
	}
	if u.detail == "tcp-settings" {
		v.tcpPage()
		return
	}
	language := v.selectWidget(u.state.Config.Settings.Language, []option{{"zh", "中文"}, {"en", "English"}}, false, func(value string) { u.changeSettings(func(s *profile.Settings) { s.Language = value }) })
	language.update = func() {
		language.setOptions([]option{{"zh", "中文"}, {"en", "English"}}, u.state.Config.Settings.Language)
	}
	theme := v.selectWidget(u.state.Config.Settings.Appearance, []option{{"light", "白色"}, {"dark", "雅黑"}}, false, func(value string) { u.changeSettings(func(s *profile.Settings) { s.Appearance = value }) })
	theme.update = func() {
		current := u.state.Config.Settings.Appearance
		if current == "" {
			current = "light"
		}
		theme.setOptions([]option{{"light", "白色"}, {"dark", "雅黑"}}, current)
	}
	language.setClasses("compact")
	theme.setClasses("compact")
	v.group("", v.box("group-row", true, 28, false,
		v.box("", true, 10, true, v.icon("language"), v.label("语言", "node-name"), v.spacer(), language),
		v.box("", true, 10, true, v.icon("moon"), v.label("主题", "node-name"), v.spacer(), theme)))

	dnsRow := v.settingsRow("DNS设置", "", false, u.openDNS)
	dnsRow.setClasses(dnsRow.classes + " settings-align")
	proxyRow := v.boundSetting("代理", func() string {
		return fmt.Sprintf("%s:%d", u.state.Config.Settings.Proxy.ListenHost(), u.state.Config.Settings.Port)
	}, false, u.openProxy)
	proxyRow.setClasses(proxyRow.classes + " settings-align")
	tcpRow := v.settingsRow("TCP", "", false, u.openTCP)
	tcpRow.setClasses(tcpRow.classes + " settings-align")
	udpRow := v.settingsRow("UDP", "", false, u.openUDP)
	udpRow.setClasses(udpRow.classes + " settings-align")
	routingRow := v.settingsRow("路由规则", "", false, func() { u.openPage("config") })
	routingRow.setClasses(routingRow.classes + " settings-align")
	v.group("连接", proxyRow, tcpRow, udpRow, routingRow, dnsRow)
	v.group("高级", v.settingsRow("连通性测试", "", false, u.openConnectivity), v.settingsRow("代理泄漏检测", "", false, func() { u.openExternal("https://browserleaks.com/ip") }), v.settingsRow("备份", "", false, u.openBackups), v.settingsRow("关于", appmeta.Version, false, func() { u.popup = &popup{kind: "about", title: "关于"}; u.notify() }))

}
func (u *UI) testVisibleNodes() {
	nodes := filterNodes(u.state.Config.Nodes, u.field("search").Text())
	start, end, _ := pageBounds(len(nodes), u.nodePage)
	nodes = append([]profile.Node{}, nodes[start:end]...)
	u.testNodes(nodes, "当前页")
}

func (u *UI) testGroupNodes(id string) {
	sources := map[string]bool{}
	for _, s := range u.state.Config.Subscriptions {
		sources[s.ID] = true
	}
	var nodes []profile.Node
	for _, n := range u.state.Config.Nodes {
		if n.Source == id || id == "" && !sources[n.Source] {
			nodes = append(nodes, n)
		}
	}
	u.testNodes(nodes, "本组")
}

func (u *UI) testNodes(nodes []profile.Node, scope string) {
	if u.testing {
		return
	}
	u.testing = true
	u.testControl = u.actionID
	u.notify()
	u.testNotice = ""
	workers := u.state.Config.Settings.TestWorkers()
	go func() {
		var wg sync.WaitGroup
		jobs := make(chan profile.Node)
		for range workers {
			wg.Go(func() {
				for n := range jobs {
					ms, err := u.backend.TestNode(n.ID)
					if err != nil {
						ms = -1
					}
					id := n.ID
					value := ms
					u.post(update{background: true, apply: func() { u.delays[id] = value }})
				}
			})
		}
		for _, n := range nodes {
			jobs <- n
		}
		close(jobs)
		wg.Wait()
		u.post(update{background: true, apply: func() { u.testing = false; u.testControl = 0; u.testNotice = "" }})
	}()
}

func confirmText(d *deletion) string {
	if d.kind == "subscription" {
		return "删除订阅「" + d.name + "」及其所有节点？"
	}
	if d.kind == "allnodes" {
		return "删除全部节点？此操作保留订阅地址，可稍后更新订阅重新导入。"
	}
	if d.kind == "logs" {
		return "清空代理日志、DNS日志和应用日志？"
	}
	return "删除「" + d.name + "」？"
}
