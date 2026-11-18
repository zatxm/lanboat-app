package native

func (v *nativeView) dnsQueryPage() {
	u := v.ui
	input := v.input("dnsHost", "example.com", false, false, false)
	input.setExpand(true)
	output := v.create("text", props{value: u.result, readOnly: true, expand: true})
	output.update = func() {
		if output.nativeText() != u.result {
			output.setValue(u.result)
		}
	}
	query := func() {
		var result string
		u.savingSettings = true
		u.submit(func() error { var err error; result, err = u.backend.DNSLookup(u.field("dnsHost").Text()); return err }, func() { u.result = result })
	}
	v.pack(v.body, v.box("panel", false, 18, false, v.box("", true, 12, false, v.label("域名", "field-label tight-label"), input, v.button("查询", "primary icon-test", false, false, query)), v.label("使用本机解析器诊断，只记录主动查询结果。", "help")), v.box("panel", false, 12, true, v.label("查询结果", "field-label"), output))
}
