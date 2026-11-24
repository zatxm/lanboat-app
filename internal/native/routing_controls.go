package native

import (
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
	for _, g := range u.state.Config.PolicyGroups {
		opts = append(opts, option{g.Ref(), g.Name})
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
	target := strings.TrimPrefix(value, "policy-group-")
	if strings.HasPrefix(target, "group:") {
		for _, g := range u.state.Config.PolicyGroups {
			if target == g.Ref() {
				return g.Name
			}
		}
	}
	if target != value {
		return target
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
func (u *UI) saveRoutingContent(content string) {
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
		if u.popup != nil && u.popup.kind == "file-rule-editor" {
			u.closePopup()
		}
		u.render()
	})
}
func (u *UI) modifyRoutingLine(line int, text string) {
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
	u.saveRoutingContent(content)
}
func (u *UI) editRoutingRule(entry *routing.Entry) {
	editor := &popup{kind: "file-rule-editor", title: "添加规则"}
	u.field("route.type").SetText("DOMAIN-SUFFIX")
	u.field("route.value").SetText("")
	u.field("route.action").SetText("proxy")
	u.field("route.noResolve").SetText("false")
	u.field("route.disabled").SetText("false")
	if entry != nil {
		editor.title = "编辑规则"
		r := entry.Rule
		editor.routingRule = r
		u.field("route.type").SetText(r.Type)
		u.field("route.value").SetText(r.Value)
		u.field("route.action").SetText(r.Action)
		u.field("route.noResolve").SetText(strconv.FormatBool(r.NoResolve))
		u.field("route.disabled").SetText(strconv.FormatBool(entry.Disabled))
	}
	u.popup = editor
	u.notify()
}
func (u *UI) saveRoutingRule() {
	r := u.popup.routingRule
	r.Type = u.field("route.type").Text()
	value := strings.TrimSpace(u.field("route.value").Text())
	if r.Type != "RULE-SET" || value != r.Value {
		r.SetHeaders, r.SetInterval = nil, 0
	}
	r.Value = value
	r.Action = u.field("route.action").Text()
	r.NoResolve = u.field("route.noResolve").Text() == "true"
	text := r.Text()
	if _, err := routing.Parse("[Rule]\n" + text); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	if u.field("route.disabled").Text() == "true" {
		text = routing.DisabledPrefix + text
	}
	if r.Line > 0 {
		u.modifyRoutingLine(r.Line, text)
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
	u.saveRoutingContent(content)
}
func (v *nativeView) routingRuleFields(box *widget) {
	u := v.ui
	value := v.input("route.value", ruleHint(u.field("route.type").Text()), false, false, false)
	kind := v.selectWidget(u.field("route.type").Text(), routingTypes, false, func(s string) {
		u.field("route.type").SetText(s)
		value.setPlaceholder(ruleHint(s))
	})
	policy := v.policyChoice(u.field("route.action").Text(), func(s string) { u.field("route.action").SetText(s) })
	for _, control := range []*widget{kind, value, policy} {
		control.setExpand(true)
	}
	v.pack(box, v.compactFormRow("匹配类型", kind), v.compactFormRow("匹配内容", value), v.compactFormRow("策略", policy),
		v.compactFormRow("不解析域名", v.switchInput("route.noResolve", "不解析域名")),
		v.compactFormRow("停用规则", v.switchInput("route.disabled", "停用规则")))
	v.dialogActions(box, u.saveRoutingRule)
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
	v.pack(v.body, toolbar)
	if readOnly {
		v.pack(v.body, v.label("默认配置为只读，请先复制后修改", "help"))
	}
	entries, _ := routing.Entries(f.Content)
	search := v.searchBox("routingSearch", "搜索规则", func() {
		u.field("page.routing").SetText("0")
		u.resetScroll = true
		u.focus = "routingSearch"
		u.render()
	})
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
	final.setTooltip("当流量没有匹配任何分流规则时，使用此策略。")
	v.controls[len(v.controls)-1].disabled = readOnly
	if !readOnly {
		v.inputs["routingFinal"] = final
	}
	fallback := v.box("", true, 6, false, v.label("未匹配时", "field-label tight-label"), final)
	if !readOnly {
		v.pack(fallback, v.button("添加", "link icon-only icon-add", true, false, func() { u.editRoutingRule(nil) }))
	}
	v.pack(v.body, v.box("", true, 12, false, search, fallback))
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
			u.modifyRoutingLine(r.Line, text)
		})
		move := func(delta int) {
			other := entries[i+delta].Rule
			lines := strings.Split(f.Content, "\n")
			lines[r.Line-1], lines[other.Line-1] = lines[other.Line-1], lines[r.Line-1]
			u.saveRoutingContent(strings.Join(lines, "\n"))
		}
		row := v.box(class, true, 8, false, v.label(strconv.Itoa(i+1), "help"), v.box("", false, 5, true, v.dataLabel(r.Value, "node-name node-title"), v.dataLabel(func() string {
			if entry.Disabled {
				return r.Type + " · " + v.translate("已停用")
			}
			return r.Type
		}(), "help")), v.dataLabel(u.policyLabel(r.Action), "strategy"), toggle,
			v.button("编辑", "link icon-only icon-edit", true, readOnly, func() { u.editRoutingRule(&entry) }), v.button("上移", "link icon-only icon-up", true, readOnly || i == 0, func() { move(-1) }), v.button("下移", "link icon-only icon-down", true, readOnly || i == len(entries)-1, func() { move(1) }), v.button("删除", "link icon-only icon-delete", true, readOnly, func() {
				u.confirm = &deletion{kind: "file-rule", name: r.Value, run: func() { u.modifyRoutingLine(r.Line, "") }}
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
	field := func(key, label, placeholder string) *widget {
		input := v.input(key, placeholder, false, false, false)
		input.setExpand(true)
		return v.compactFormRow(label, input)
	}
	network := v.selectWidget(u.field("matchNetwork").Text(), []option{{"tcp", "TCP"}, {"udp", "UDP"}}, false, func(s string) { u.field("matchNetwork").SetText(s) })
	network.setExpand(true)
	v.inputs["matchNetwork"] = network
	v.pack(v.body, v.box("panel", false, 12, false,
		field("matchHost", "域名", "example.com"),
		field("matchIP", "IP地址（可选）", "1.1.1.1"),
		v.compactFormRow("", v.label("需要测试IP规则时，请填写目标IP，域名可不填", "help")),
		field("matchPort", "目标端口", "443"),
		v.compactFormRow("网络协议", network),
		field("matchAgent", "User-Agent（可选）", ""),
		v.box("", true, 8, false, v.spacer(), v.button("测试规则", "primary icon-test", false, false, test))))
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

func (v *nativeView) policyChoice(current string, apply func(string)) *widget {
	choice := v.selectWidget(current, v.ui.policyOptions(), false, apply)
	v.inputs["policy"] = choice
	return choice
}

func (v *nativeView) policySelect(current string, apply func(string)) *widget {
	return v.box("field", false, 7, false, v.label("策略", "field-label"), v.policyChoice(current, apply))
}
