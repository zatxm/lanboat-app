package native

import "strings"

func (u *UI) importNodes() {
	u.field("content").SetText("")
	u.field("nodeImportMethod").SetText("manual")
	u.field("nodeImportPath").SetText("")
	u.field("nodeImportURL").SetText("")
	u.field("nodeImportRules").SetText("false")
	u.openEditor("import")
}

func (v *nativeView) nodeImportFields(box *widget) {
	u := v.ui
	const formats = "兼容Lanboat节点JSON、Clash、V2ray/V2fly、Sing-box、Shadowsocks、Sub、Github、Karing订阅。"
	hint := v.translate(formats)
	if v.compactWindow() {
		hint = strings.Replace(hint, "Shadowsocks", "\nShadowsocks", 1)
	}
	v.pack(box, v.dataLabel(hint, "help"))
	method := v.selectWidget(u.field("nodeImportMethod").Text(), []option{{"manual", "手动输入"}, {"subscription", "订阅"}, {"file", "导入文件"}}, false, func(value string) {
		u.field("nodeImportMethod").SetText(value)
		u.render()
	})
	// The dropdown only needs room for the longest localized method.
	if v.ui.state.Config.Settings.Language == "en" {
		method.pointer.Width(134)
	} else {
		method.pointer.Width(100)
	}
	v.inputs["nodeImportMethod"] = method
	row := v.box("", true, 12, false, v.label("方式", "field-label"), method)
	switch method.value {
	case "subscription":
		address := v.input("nodeImportURL", "https://…", false, false, false)
		address.setExpand(true)
		address.pointer.Label(v.translate("订阅地址"))
		v.pack(row, address, v.label("保留分流规则", "field-label"), v.switchInput("nodeImportRules", "保留分流规则"))
	case "file":
		v.pack(row, v.button("选择文件", "link icon-upload", false, false, func() {
			if path := chooseFile("导入节点", "*"); path != "" {
				u.field("nodeImportPath").SetText(path)
				u.render()
			}
		}))
	}
	v.pack(box, row)
	switch method.value {
	case "file":
		if path := u.field("nodeImportPath").Text(); path != "" {
			v.pack(box, v.dataLabel(path, "help"))
		}
	case "manual":
		content := v.input("content", "", true, false, false)
		content.pointer.Label(v.translate("导入节点"))
		v.pack(box, content)
	}
}
