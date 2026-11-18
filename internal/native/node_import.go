package native

func (u *UI) importNodes() {
	u.field("content").SetText("")
	u.field("nodeImportMethod").SetText("manual")
	u.field("nodeImportPath").SetText("")
	u.openEditor("import")
}

func (v *nativeView) nodeImportFields(box *widget) {
	u := v.ui
	method := v.selectWidget(u.field("nodeImportMethod").Text(), []option{{"manual", "手动输入"}, {"file", "上传文件"}}, false, func(value string) {
		u.field("nodeImportMethod").SetText(value)
		u.render()
	})
	method.setClasses("protocol-choice")
	v.inputs["nodeImportMethod"] = method
	row := v.box("", true, 12, false, v.label("方式", "field-label"), method)
	if u.field("nodeImportMethod").Text() == "file" {
		v.pack(row, v.button("选择文件", "link icon-upload", false, false, func() {
			if path := chooseFile("导入节点", "*"); path != "" {
				var content string
				u.submit(func() error { var err error; content, err = u.backend.ReadImport(path); return err }, func() { u.field("nodeImportPath").SetText(path); u.field("content").SetText(content); u.render() })
			}
		}))
	}
	v.pack(box, row)
	if path := u.field("nodeImportPath").Text(); u.field("nodeImportMethod").Text() == "file" && path != "" {
		v.pack(box, v.dataLabel(path, "help"))
	}
	v.pack(box, v.box("field", false, 8, true, v.label("导入内容", "field-label"), v.input("content", "", true, false, false)), v.label("兼容Clash、V2ray/V2fly、Sing-box、Shadowsocks、Sub、Github、Karing订阅。", "help"))
}
