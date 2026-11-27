package native

func (u *UI) importNodes() {
	u.field("content").SetText("")
	u.field("nodeImportMethod").SetText("manual")
	u.field("nodeImportPath").SetText("")
	u.field("nodeImportURL").SetText("")
	u.field("nodeImportUA").SetText("")
	u.field("nodeImportUA.custom").SetText("")
	u.field("nodeImportRules").SetText("false")
	u.openEditor("import")
}

func (v *nativeView) nodeImportFields(box *widget) {
	u := v.ui
	const formats = "兼容Lanboat节点JSON、普通节点链接、ClashMeta、Quantumult X、Loon、Sing-box和Karing订阅。"
	hint := v.translate(formats)
	if v.compactWindow() {
		hint = wrapDialogText(hint, 50)
	}
	v.pack(box, v.dataLabel(hint, "help"))
	method := v.selectWidget(u.field("nodeImportMethod").Text(), []option{{"manual", "手动输入"}, {"subscription", "订阅"}, {"file", "导入文件"}}, false, func(value string) {
		u.field("nodeImportMethod").SetText(value)
		u.render()
	})
	v.inputs["nodeImportMethod"] = method
	if method.value == "file" {
		v.pack(box, v.formRow("方式", method, v.button("选择文件", "link icon-upload", false, false, func() {
			u.pickFile("导入节点", "*", func(path string) {
				if path != "" {
					u.field("nodeImportPath").SetText(path)
					u.render()
				}
			})
		})))
	} else {
		v.pack(box, v.formRow("方式", method))
	}
	switch method.value {
	case "subscription":
		address := v.input("nodeImportURL", "https://…", false, false, false)
		v.pack(box, v.formRow("订阅地址", address), v.subscriptionAgent("nodeImportUA", "nodeImportURL"),
			v.formSwitch("nodeImportRules", "保留分流规则"))
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
