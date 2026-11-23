package native

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/zatxm/fego/v26/ui"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
)

func (u *UI) editRoutingGroup(group *profile.PolicyGroup) {
	title, id := "创建", ""
	g := routing.Group{Type: "select", Selected: "proxy", Members: []string{"proxy"}, URL: profile.TestURLs[0], Interval: 600}
	if group != nil {
		title, id, g = "修改", group.ID, group.Group
		if g.Selected == "" {
			g.Selected = g.Members[0]
		}
	}
	for key, value := range map[string]string{
		"id": id, "name": g.Name, "type": g.Type, "members": strings.Join(g.Members, "\n"), "selected": g.Selected,
		"url": g.URL, "interval": strconv.Itoa(g.Interval), "tolerance": strconv.Itoa(g.Tolerance),
	} {
		u.field("group." + key).SetText(value)
	}
	u.popup = &popup{kind: "routing-group", title: title}
	u.notify()
}

func (u *UI) groupPolicyOptions() []option {
	id := u.field("group.id").Text()
	opts := u.policyOptions()
	return slices.DeleteFunc(opts, func(o option) bool { return o.value == "reject" || o.value == "group:"+id })
}

func (u *UI) saveRoutingGroup() {
	g := profile.PolicyGroup{
		ID: u.field("group.id").Text(),
		Group: routing.Group{
			Name: strings.TrimSpace(u.field("group.name").Text()), Type: u.field("group.type").Text(),
			URL: u.field("group.url").Text(), Interval: 600,
		},
	}
	if g.Type == "select" {
		g.Selected = u.field("group.selected").Text()
		g.Members = []string{g.Selected}
	} else {
		var intervalErr, toleranceErr error
		g.Interval, intervalErr = strconv.Atoi(u.field("group.interval").Text())
		g.Tolerance, toleranceErr = strconv.Atoi(u.field("group.tolerance").Text())
		g.Members = groupMembers(u.field("group.members").Text())
		if intervalErr != nil || toleranceErr != nil {
			u.err = "策略组参数无效"
			u.notify()
			return
		}
	}
	u.submit(func() error { return u.backend.SavePolicyGroup(g) }, u.closePopup)
}

func groupMembers(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (v *nativeView) routingGroupFields(box *widget) {
	u := v.ui
	kind := u.field("group.type").Text()
	types := []option{{"select", "手动选择"}, {"url-test", "自动测速"}}
	if kind == "fallback" {
		types = append(types, option{"fallback", "故障转移"})
	}
	name := v.input("group.name", "", false, false, false)
	name.pointer.Width(320)
	mode := v.selectWidget(kind, types, false, func(value string) {
		u.field("group.type").SetText(value)
		p := *u.popup
		u.popup = &p
		u.notify()
	})
	mode.pointer.Width(320)
	v.inputs["group.type"] = mode
	v.pack(box, v.compactFormRow("名称", name), v.compactFormRow("类型", mode))

	if kind == "select" {
		current := u.field("group.selected").Text()
		options := u.groupPolicyOptions()
		if !slices.ContainsFunc(options, func(opt option) bool { return opt.value == current }) {
			options = append(options, option{current, u.policyLabel(current)})
		}
		choice := v.selectWidget(current, options, false, func(value string) { u.field("group.selected").SetText(value) })
		choice.pointer.Width(320)
		v.inputs["group.selected"] = choice
		v.pack(box, v.compactFormRow("默认策略", choice))
	} else {
		edit := v.button("选择测速节点", "link icon-only icon-edit", false, false, func() {
			u.popup = &popup{kind: "group-test-members", title: "测速节点", previous: u.popup}
			u.notify()
		})
		count := fmt.Sprintf(v.translate("%d项"), len(groupMembers(u.field("group.members").Text())))
		url := v.dataLabel(u.field("group.url").Text(), "node-name applied-url")
		url.pointer.Width(320).Clip()
		url.setTooltip(u.field("group.url").Text())
		v.pack(box,
			v.compactFormRow("测速节点", v.dataLabel(count, "node-name"), edit),
			v.compactFormRow("测速URL", url, v.button("修改", "link icon-only icon-edit", false, false, u.editGroupTestURL)))
		numeric := func(key, title, unit string) *widget {
			input := v.input(key, "", false, false, false)
			input.pointer.Width(128)
			return v.compactFormRow(title, input, v.label(unit, "help"))
		}
		v.pack(box, numeric("group.interval", "测速间隔", "秒"), numeric("group.tolerance", "切换公差", "毫秒"))
	}
	v.dialogActions(box, u.saveRoutingGroup)
}

func (v *nativeView) groupTestMembers(box *widget) {
	u := v.ui
	options := u.groupPolicyOptions()
	selected := groupMembers(u.field("group.members").Text())
	for _, current := range selected {
		if !slices.ContainsFunc(options, func(opt option) bool { return opt.value == current }) {
			options = append(options, option{current, u.policyLabel(current)})
		}
	}
	checked := make(map[string]bool, len(selected))
	for _, policy := range selected {
		checked[policy] = true
	}
	list := ui.VirtualList(ui.VirtualListOptions{Count: len(options), Name: func(i int) string { return options[i].label }}, func(i int) *ui.Element {
		opt := options[i]
		label := opt.label
		if opt.value == "direct" || opt.value == "proxy" {
			label = v.translate(label)
		}
		return ui.Checkbox(label, checked[opt.value]).Padding(0, 8).OnToggle(func(enabled bool) {
			checked[opt.value] = enabled
			if enabled {
				selected = append(selected, opt.value)
			} else {
				selected = slices.DeleteFunc(selected, func(policy string) bool { return policy == opt.value })
			}
		})
	}).Height(240)
	holder := v.box("", false, 0, false)
	holder.pointer.Append(list)
	v.pack(box, holder)
	v.dialogActions(box, func() {
		if len(selected) == 0 || len(selected) > 200 {
			u.err = "请选择1–200个测速节点"
			u.notify()
			return
		}
		u.field("group.members").SetText(strings.Join(selected, "\n"))
		u.closePopup()
	})
}

func (v *nativeView) routingGroupsPage() {
	u := v.ui
	groups := u.state.Config.PolicyGroups
	add := v.button("创建策略组", "link icon-only icon-add", true, false, func() { u.editRoutingGroup(nil) })
	v.pack(v.body, v.box("", true, 0, false, v.spacer(), add))
	list := v.box("", false, 0, false)
	start, end, pager := v.listRange("routing-groups", len(groups))
	for i := start; i < end; i++ {
		g := groups[i]
		if i > start {
			v.pack(list, v.create("separator", props{}))
		}
		info := u.policyText(g.Selected)
		if g.Type != "select" {
			method := "自动测速"
			if g.Type == "fallback" {
				method = "故障转移"
			}
			info = v.translate(method) + " · " + fmt.Sprintf(v.translate("%d项"), len(g.Members))
		} else if info == "" {
			info = u.policyText(g.Members[0])
		}
		row := v.box("node-row", true, 8, false,
			v.box("", false, 5, true, v.dataLabel(g.Name, "node-name node-title"), v.dataLabel(info, "help")),
			v.button("编辑", "link icon-only icon-edit", true, false, func() { u.editRoutingGroup(&g) }),
			v.button("删除", "link icon-only icon-delete", true, false, func() {
				u.confirm = &deletion{kind: "routing-group", id: g.ID, name: g.Name}
				u.notify()
			}))
		v.pack(list, row)
	}
	if len(groups) == 0 {
		v.pack(v.body, v.box("panel", false, 0, false, v.label("暂无策略组", "help")))
		return
	}
	if end-start <= 4 {
		v.pack(v.body, v.box("panel group list-frame", false, 0, false, list))
	} else {
		v.listViewport(list)
	}
	if pager != nil {
		v.pack(v.body, pager)
	}
}
