package native

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

var routingTypes = []option{{"DOMAIN-SUFFIX", "域名后缀"}, {"DOMAIN", "完整域名"}, {"DOMAIN-KEYWORD", "域名关键词"}, {"DOMAIN-REGEX", "域名正则"}, {"IP-CIDR", "IPv4网段"}, {"IP-CIDR6", "IPv6网段"}, {"GEOIP", "国家地区"}, {"IP-ASN", "ASN"}, {"DST-PORT", "目标端口"}, {"NETWORK", "网络协议"}, {"RULE-SET", "规则集"}, {"DOMAIN-SET", "域名集"}, {"USER-AGENT", "User-Agent"}}

func (u *UI) routingFile() (profile.ConfigFile, bool) {
	id := u.fileID
	if u.page == "rules" {
		id = u.state.Config.ActiveFile
	}
	for _, f := range u.state.Config.Files {
		if f.ID == id {
			return f, true
		}
	}
	return profile.ConfigFile{}, false
}
func (u *UI) openRoutingFile(f profile.ConfigFile) {
	u.field("page.routing").SetText("0")
	u.fileID = f.ID
	u.openDetail("routing-control")
}
func (u *UI) policyOptions() []option {
	opts := slices.Clone(actions)
	if f, ok := u.routingFile(); ok {
		p, _ := routing.Parse(f.Content)
		for _, g := range p.Groups {
			opts = append(opts, option{g.Name, g.Name})
		}
	}
	for _, n := range u.state.Config.Nodes {
		opts = append(opts, option{"node:" + n.ID, n.Name})
	}
	return opts
}

// policyText is policyLabel for raw labels: built-in actions are translated,
// node and group names are user content and stay as written.
func (u *UI) policyText(value string) string {
	label := u.policyLabel(value)
	for _, a := range actions {
		if label == a.label && u.view != nil {
			return u.view.translate(label)
		}
	}
	return label
}
func (u *UI) policyLabel(value string) string {
	for _, a := range actions {
		if value == a.value {
			return a.label
		}
	}
	// Core outbound tags from the runtime's connection records.
	if value == "DIRECT" {
		return u.policyLabel("direct")
	}
	if name, ok := strings.CutPrefix(value, "policy-group-"); ok {
		return name
	}
	id, ok := strings.CutPrefix(value, "node:")
	if !ok {
		id, ok = strings.CutPrefix(value, "policy-node-")
	}
	if ok {
		for _, n := range u.state.Config.Nodes {
			if n.ID == id {
				return n.Name
			}
		}
	}
	return value
}
func (u *UI) saveRoutingContent(content string, back bool) {
	f, ok := u.routingFile()
	if !ok {
		return
	}
	if f.ID == "default" {
		u.err = "默认配置为只读，请先复制后修改"
		u.notify()
		return
	}
	if _, err := routing.Parse(content); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.submit(func() error { return u.backend.SaveConfigFile(f.ID, f.Name, content) }, func() {
		if back {
			u.back()
		} else {
			u.render()
		}
	})
}
func (u *UI) modifyRoutingLine(line int, text string, back bool) {
	f, ok := u.routingFile()
	if !ok {
		return
	}
	content, err := routing.ReplaceLine(f.Content, line, text)
	if err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.saveRoutingContent(content, back)
}
func (u *UI) editRoutingRule(entry *routing.Entry) {
	u.field("route.line").SetText("0")
	u.field("route.type").SetText("DOMAIN-SUFFIX")
	u.field("route.value").SetText("")
	u.field("route.action").SetText("proxy")
	u.field("route.noResolve").SetText("false")
	u.field("route.disabled").SetText("false")
	if entry != nil {
		r := entry.Rule
		u.field("route.line").SetText(strconv.Itoa(r.Line))
		u.field("route.type").SetText(r.Type)
		u.field("route.value").SetText(r.Value)
		u.field("route.action").SetText(r.Action)
		u.field("route.noResolve").SetText(strconv.FormatBool(r.NoResolve))
		u.field("route.disabled").SetText(strconv.FormatBool(entry.Disabled))
	}
	u.openEditor("file-rule")
}
func (u *UI) saveRoutingRule() {
	r := routing.Rule{Type: u.field("route.type").Text(), Value: strings.TrimSpace(u.field("route.value").Text()), Action: u.field("route.action").Text(), NoResolve: u.field("route.noResolve").Text() == "true"}
	text := r.Text()
	if _, err := routing.Parse("[Rule]\n" + text); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	if u.field("route.disabled").Text() == "true" {
		text = routing.DisabledPrefix + text
	}
	line, _ := strconv.Atoi(u.field("route.line").Text())
	if line > 0 {
		u.modifyRoutingLine(line, text, true)
		return
	}
	f, ok := u.routingFile()
	if !ok {
		return
	}
	content, err := routing.InsertSectionLine(f.Content, "Rule", text)
	if err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.saveRoutingContent(content, true)
}
func (v *nativeView) routingRuleFields(box *widget) {
	u := v.ui
	value := v.input("route.value", ruleHint(u.field("route.type").Text()), false, false, false)
	v.pack(box, v.selectField("匹配类型", u.field("route.type").Text(), routingTypes, func(s string) { u.field("route.type").SetText(s); value.setPlaceholder(ruleHint(s)) }),
		v.box("field", false, 7, false, v.label("匹配内容", "field-label"), value), v.policySelect(u.field("route.action").Text(), func(s string) { u.field("route.action").SetText(s) }), v.formSwitch("route.noResolve", "不解析域名"), v.formSwitch("route.disabled", "停用规则"))
}
func (v *nativeView) switchInput(key, title string) *widget {
	u := v.ui
	b := v.button(title, "switch", false, false, func() { u.field(key).SetText(strconv.FormatBool(u.field(key).Text() != "true")) })
	b.update = func() {
		class := "button switch"
		if u.field(key).Text() == "true" {
			class += " switch-on"
		}
		b.setClasses(class)
	}
	v.inputs[key] = b
	return b
}

func (v *nativeView) formSwitch(key, title string) *widget {
	return v.box("", true, 12, false, v.label(title, "node-name"), v.spacer(), v.switchInput(key, title))
}
func (v *nativeView) routingControlPage() {
	u := v.ui
	f, ok := u.routingFile()
	if !ok {
		return
	}
	p, err := routing.Parse(f.Content)
	if err != nil {
		v.pack(v.body, v.label(err.Error(), "help error"))
		return
	}
	readOnly := f.ID == "default"
	toolbar := v.box("", true, 10, false, v.dataLabel(f.Name, "section-title"), v.spacer())
	if readOnly {
		v.pack(toolbar, v.button("复制后编辑", "secondary icon-copy", true, false, func() {
			ids := map[string]bool{}
			for _, old := range u.state.Config.Files {
				ids[old.ID] = true
			}
			u.submit(func() error { return u.backend.CopyConfigFile(f.ID) }, func() {
				for _, next := range u.state.Config.Files {
					if !ids[next.ID] {
						u.fileID = next.ID
						break
					}
				}
				u.render()
			})
		}))
	}
	v.pack(toolbar, v.button("代理分组", "link icon-layers", false, false, func() { u.openDetail("routing-groups") }))
	if !readOnly {
		v.pack(toolbar, v.button("添加", "primary tight-icon", true, false, func() { u.editRoutingRule(nil) }))
	}
	v.pack(v.body, toolbar)
	if readOnly {
		v.pack(v.body, v.label("默认配置为只读，请先复制后修改", "help"))
	}
	entries, _ := routing.Entries(f.Content)
	search := v.input("routingSearch", "搜索规则", false, false, false)
	search.event = func(event, value string) {
		u.field("routingSearch").SetText(value)
		u.field("page.routing").SetText("0")
		u.focus = "routingSearch"
		u.render()
	}
	search.setExpand(true)
	final := v.selectWidget(p.Final, u.policyOptions(), false, func(action string) {
		current, ok := u.routingFile()
		if !ok {
			return
		}
		content, err := routing.SetFinal(current.Content, action)
		if err != nil {
			u.err = err.Error()
			u.notify()
			return
		}
		u.savingSettings = true
		u.submit(func() error { return u.backend.SaveConfigFile(current.ID, current.Name, content) }, func() {
			for i := range u.state.Config.Files {
				if u.state.Config.Files[i].ID == current.ID {
					u.state.Config.Files[i].Content = content
					break
				}
			}
		})
	})
	final.setClasses("compact final-policy")
	v.controls[len(v.controls)-1].disabled = readOnly
	if !readOnly {
		v.inputs["routingFinal"] = final
	}
	v.pack(v.body, v.box("", true, 16, false, search, v.label("最终策略", "field-label tight-label"), final))
	v.editorScroll = v.create("scroll", props{classes: "editor-scroll", expand: true})
	v.pack(v.body, v.box("panel group list-frame", false, 0, true, v.editorScroll))
	list := v.box("", false, 0, false)
	query := strings.ToLower(strings.TrimSpace(u.field("routingSearch").Text()))
	matches := make([]int, 0, len(entries))
	for i, entry := range entries {
		r := entry.Rule
		if query == "" || strings.Contains(strings.ToLower(r.Type+" "+r.Value+" "+u.policyLabel(r.Action)), query) {
			matches = append(matches, i)
		}
	}
	start, end, pager := v.listRange("routing", len(matches))
	shown := 0
	for _, i := range matches[start:end] {
		entry := entries[i]
		i, entry := i, entry
		r := entry.Rule
		if query != "" && !strings.Contains(strings.ToLower(r.Type+" "+r.Value+" "+u.policyLabel(r.Action)), query) {
			continue
		}
		shown++
		class := "node-row"
		if entry.Disabled {
			class += " rule-disabled"
		}
		if shown > 1 {
			v.pack(list, v.create("separator", props{}))
		}
		toggleText, toggleStyle := "停用", "link icon-only icon-check rule-state"
		if entry.Disabled {
			toggleText, toggleStyle = "启用", "link icon-only icon-close danger rule-state"
		}
		toggle := v.button(toggleText, toggleStyle, true, readOnly, func() {
			text := r.Text()
			if !entry.Disabled {
				text = routing.DisabledPrefix + text
			}
			u.modifyRoutingLine(r.Line, text, false)
		})
		move := func(delta int) {
			other := entries[i+delta].Rule
			lines := strings.Split(f.Content, "\n")
			lines[r.Line-1], lines[other.Line-1] = lines[other.Line-1], lines[r.Line-1]
			u.saveRoutingContent(strings.Join(lines, "\n"), false)
		}
		row := v.box(class, true, 8, false, v.label(strconv.Itoa(i+1), "help"), v.box("", false, 5, true, v.dataLabel(r.Value, "node-name node-title"), v.dataLabel(func() string {
			if entry.Disabled {
				return r.Type + " · " + v.translate("已停用")
			}
			return r.Type
		}(), "help")), v.dataLabel(u.policyLabel(r.Action), "strategy"), toggle,
			v.button("编辑", "link icon-only icon-edit", true, readOnly, func() { u.editRoutingRule(&entry) }), v.button("上移", "link icon-only icon-up", true, readOnly || i == 0, func() { move(-1) }), v.button("下移", "link icon-only icon-down", true, readOnly || i == len(entries)-1, func() { move(1) }), v.button("删除", "link icon-only icon-delete", true, readOnly, func() {
				u.confirm = &deletion{kind: "file-rule", name: r.Value, run: func() { u.modifyRoutingLine(r.Line, "", false) }}
				u.notify()
			}))
		v.pack(list, row)
	}
	if len(entries) == 0 {
		v.pack(list, v.label("暂无分流规则", "empty help"))
	}
	v.pack(v.editorScroll, list)
	if pager != nil {
		v.pack(v.body, pager)
	}
}
func (u *UI) editRoutingGroup(g *routing.Group) {
	for k, v := range map[string]string{"line": "0", "name": "", "type": "select", "members": "proxy\ndirect", "selected": "proxy", "url": profile.TestURLs[0], "interval": "600", "tolerance": "0"} {
		u.field("group." + k).SetText(v)
	}
	if g != nil {
		for k, v := range map[string]string{"line": strconv.Itoa(g.Line), "name": g.Name, "type": g.Type, "members": strings.Join(g.Members, "\n"), "selected": g.Selected, "url": g.URL, "interval": strconv.Itoa(g.Interval), "tolerance": strconv.Itoa(g.Tolerance)} {
			u.field("group." + k).SetText(v)
		}
	}
	u.openEditor("routing-group")
}
func (u *UI) saveRoutingGroup() {
	f, ok := u.routingFile()
	if !ok {
		return
	}
	interval, err := strconv.Atoi(u.field("group.interval").Text())
	tolerance, err2 := strconv.Atoi(u.field("group.tolerance").Text())
	if err != nil || err2 != nil {
		u.err = "代理分组参数无效"
		u.notify()
		return
	}
	g := routing.Group{Name: strings.TrimSpace(u.field("group.name").Text()), Type: u.field("group.type").Text(), Members: strings.Split(u.field("group.members").Text(), "\n"), Selected: u.field("group.selected").Text(), URL: strings.TrimSpace(u.field("group.url").Text()), Interval: interval, Tolerance: tolerance}
	line, _ := strconv.Atoi(u.field("group.line").Text())
	var content string
	if line > 0 {
		content, err = routing.ReplaceLine(f.Content, line, g.Text())
	} else {
		content, err = routing.InsertSectionLine(f.Content, "Proxy Group", g.Text())
	}
	if err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.saveRoutingContent(content, true)
}
func (v *nativeView) routingGroupFields(box *widget) {
	u := v.ui
	v.pack(box, v.field("group.name", "名称", "", false), v.selectField("类型", u.field("group.type").Text(), []option{{"select", "手动选择"}, {"url-test", "自动测速"}}, func(value string) { u.field("group.type").SetText(value); u.render() }))
	members := strings.Split(u.field("group.members").Text(), "\n")
	opts := []option{}
	for _, m := range members {
		if m != "" {
			opts = append(opts, option{m, u.policyLabel(m)})
		}
	}
	if u.field("group.type").Text() == "select" && len(opts) > 0 {
		v.pack(box, v.selectField("默认策略", u.field("group.selected").Text(), opts, func(value string) { u.field("group.selected").SetText(value) }))
	}
	options := u.policyOptions()
	filtered := options[:0]
	for _, o := range options {
		if o.value != "reject" && o.value != u.field("group.name").Text() {
			filtered = append(filtered, o)
		}
	}
	filtered = append([]option{{"", "选择策略"}}, filtered...)
	choice := v.selectWidget("", filtered, false, func(value string) {
		if value == "" {
			return
		}
		members := strings.Split(u.field("group.members").Text(), "\n")
		if len(members) == 1 && members[0] == "" {
			members = nil
		}
		if !slices.Contains(members, value) {
			members = append(members, value)
			u.field("group.members").SetText(strings.Join(members, "\n"))
			if u.field("group.selected").Text() == "" {
				u.field("group.selected").SetText(value)
			}
		}
		u.render()
	})
	choice.setClasses("compact final-policy")
	v.inputs["group.addPolicy"] = choice
	v.pack(box, v.box("", true, 10, false, v.label("分组成员", "field-label"), v.spacer(), v.label("添加策略", "field-label"), choice))

	list := v.box("panel group", false, 0, false)
	start, end, pager := v.listRange("group-members", len(members))
	for i := start; i < end; i++ {
		m := members[i]
		if m == "" {
			continue
		}
		i, m := i, m
		if i > 0 {
			v.pack(list, v.create("separator", props{}))
		}
		v.pack(list, v.box("group-row", true, 8, false, v.dataLabel(u.policyLabel(m), "node-name"), v.spacer(), v.button("删除", "link icon-only icon-delete", false, false, func() {
			rest := append(slices.Clone(members[:i]), members[i+1:]...)
			u.field("group.members").SetText(strings.Join(rest, "\n"))
			if u.field("group.selected").Text() == m {
				value := ""
				if len(rest) > 0 {
					value = rest[0]
				}
				u.field("group.selected").SetText(value)
			}
			u.render()
		})))
	}
	v.pack(box, list)
	if pager != nil {
		v.pack(box, pager)
	}
	if u.field("group.type").Text() == "url-test" {
		value := v.dataLabel(u.field("group.url").Text(), "node-name applied-url")
		value.setExpand(true)
		value.update = func() { value.setText(u.field("group.url").Text()) }
		v.pack(box, v.box("option-group", false, 12, false,
			v.box("", true, 8, false, v.label("测试URL", "field-label"), value, v.button("修改", "link icon-only icon-edit", false, false, u.editGroupTestURL)),
			v.box("", true, 20, false, v.field("group.interval", "测试间隔（秒）", "600", false), v.field("group.tolerance", "切换公差（毫秒）", "0", false))))
	}
}
func (v *nativeView) routingGroupsPage() {
	u := v.ui
	f, ok := u.routingFile()
	if !ok {
		return
	}
	p, _ := routing.Parse(f.Content)
	readOnly := f.ID == "default"
	toolbar := v.box("", true, 10, false, v.dataLabel(f.Name, "section-title"), v.spacer())
	if !readOnly {
		v.pack(toolbar, v.button("添加", "primary tight-icon", true, false, func() { u.editRoutingGroup(nil) }))
	}
	v.pack(v.body, toolbar)
	list := v.box("", false, 0, false)
	start, end, pager := v.listRange("routing-groups", len(p.Groups))
	for i := start; i < end; i++ {
		g := p.Groups[i]
		if i > 0 {
			v.pack(list, v.create("separator", props{}))
		}
		row := v.box("node-row", true, 8, false, v.box("", false, 5, true, v.dataLabel(g.Name, "node-name node-title"), v.dataLabel(fmt.Sprintf("%s · %d", g.Type, len(g.Members)), "help")), v.button("编辑", "link icon-only icon-edit", true, readOnly, func() { u.editRoutingGroup(&g) }))
		if !readOnly {
			v.pack(row, v.button("删除", "link icon-only icon-delete", true, readOnly, func() {
				u.confirm = &deletion{kind: "routing-group", name: g.Name, run: func() { u.modifyRoutingLine(g.Line, "", false) }}
				u.notify()
			}))
		}
		v.pack(list, row)
	}
	if len(p.Groups) == 0 {
		v.pack(list, v.label("暂无代理分组", "empty help"))
	}
	v.listViewport(list)
	if pager != nil {
		v.pack(v.body, pager)
	}
}

func (v *nativeView) routingTestPage() {
	u := v.ui
	if u.field("matchPort").Text() == "" {
		u.field("matchPort").SetText("443")
	}
	if u.field("matchNetwork").Text() == "" {
		u.field("matchNetwork").SetText("tcp")
	}
	result := v.dataLabel(u.result, "node-name")
	result.update = func() { result.setText(u.result); result.parent.setVisible(u.result != "") }
	test := func() {
		port, err := strconv.Atoi(u.field("matchPort").Text())
		if err != nil || port < 1 || port > 65535 {
			u.err = "测试端口需在1–65535之间"
			u.notify()
			return
		}
		ipText := strings.TrimSpace(u.field("matchIP").Text())
		ip, err := netip.ParseAddr(ipText)
		if ipText != "" && err != nil {
			u.err = "IP地址无效"
			u.notify()
			return
		}
		req := routing.Request{Host: strings.TrimSpace(u.field("matchHost").Text()), IP: ip, Port: port, Network: u.field("matchNetwork").Text(), Agent: u.field("matchAgent").Text()}
		var output string
		u.submit(func() error { var err error; output, err = u.backend.TestRuleRequest(req); return err }, func() { u.result = output })
	}
	v.pack(v.body, v.box("panel", false, 18, false,
		v.box("", true, 18, false, v.box("", false, 0, true, v.field("matchHost", "域名", "example.com", false)), v.box("", false, 0, true, v.field("matchIP", "IP地址（可选）", "1.1.1.1", false))),
		v.box("", true, 18, false, v.box("", false, 0, true, v.field("matchPort", "目标端口", "443", false)), v.box("", false, 0, true, v.selectField("网络协议", u.field("matchNetwork").Text(), []option{{"tcp", "TCP"}, {"udp", "UDP"}}, func(s string) { u.field("matchNetwork").SetText(s) }))),
		v.field("matchAgent", "User-Agent（可选）", "", false), v.label("需要测试IP规则时，请填写目标IP", "help"), v.box("", true, 8, false, v.spacer(), v.button("测试规则", "primary icon-test", false, false, test))))
	card := v.box("panel", false, 12, false, v.box("", true, 8, false, v.icon("route"), v.label("匹配结果", "field-label")), result)
	card.setVisible(u.result != "")
	v.pack(v.body, card)
}

func ruleHint(kind string) string {
	if raw := profile.RuleTypeNames[kind]; raw != "" {
		kind = raw
	}
	hints := map[string]string{"DOMAIN-REGEX": `^example\.com$`, "IP-CIDR": "192.168.0.0/16", "IP-CIDR6": "2001:db8::/32", "GEOIP": "CN", "IP-ASN": "132203", "DST-PORT": "443", "NETWORK": "tcp / udp", "RULE-SET": "lanboat://geosite/cn 或 HTTPS URL", "DOMAIN-SET": "https://example.com/domains.list", "USER-AGENT": "ExampleApp*"}
	if hint := hints[kind]; hint != "" {
		return hint
	}
	return "example.com"
}

func (v *nativeView) policySelect(current string, apply func(string)) *widget {
	choice := v.selectWidget(current, v.ui.policyOptions(), false, apply)
	v.inputs["policy"] = choice
	return v.box("field", false, 7, false, v.label("策略", "field-label"), choice)
}
