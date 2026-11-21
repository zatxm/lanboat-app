package native

import "strings"

func (v *nativeView) dnsQueryPage() {
	u := v.ui
	input := v.input("dnsHost", "example.com", false, false, false)
	input.setExpand(true)
	output := v.dataLabel(u.result, "")
	results := v.create("scroll", props{expand: true})
	v.pack(results, v.box("panel", false, 12, false, v.label("查询结果", "field-label"), output))
	results.setVisible(u.result != "")
	results.update = func() {
		output.setText(u.result)
		results.setVisible(u.result != "")
	}
	query := func() {
		host := strings.TrimSpace(u.field("dnsHost").Text())
		u.result = ""
		if results.visible {
			results.scrollPosition(0)
			results.setVisible(false)
		}
		var result string
		u.savingSettings = true
		u.submit(func() error {
			var err error
			result, err = u.backend.DNSLookup(host)
			return err
		}, func() { u.result = result })
	}
	v.pack(v.body,
		v.box("panel", false, 18, false,
			v.box("", true, 12, false, v.label("域名", "field-label tight-label"), input, v.button("查询", "primary icon-test", false, false, query)),
			v.label("使用本机解析器诊断，只记录主动查询结果。", "help")),
		results)
}
