package native

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

func (u *UI) importConfig() {
	u.fileID = ""
	for _, key := range []string{"fileName", "fileContent", "importPath", "importURL"} {
		u.field(key).SetText("")
	}
	u.field("importMethod").SetText("manual")
	u.openEditor("config-import")
}

func (u *UI) previewConfig(input, name string) {
	var content string
	u.submit(func() error {
		var err error
		content, err = u.backend.ParseConfigImport(input)
		return err
	}, func() {
		u.field("fileContent").SetText(displayConfigContent(content))
		if name == "default.conf" {
			name = "import.conf"
		}
		if u.field("fileName").Text() == "" {
			u.field("fileName").SetText(name)
		}
		u.render()
	})
}

func (v *nativeView) configImportFields(box *widget) {
	u := v.ui
	v.pack(box, v.field("fileName", "名称", "例如：custom.conf", false))
	method := v.selectWidget(u.field("importMethod").Text(), []option{{"manual", "手动输入"}, {"file", "上传文件"}, {"url", "URL"}}, false, func(value string) {
		u.field("importMethod").SetText(value)
		u.err = ""
		u.render()
	})
	method.setClasses("protocol-choice")
	v.inputs["importMethod"] = method
	row := v.box("", true, 12, false, v.label("方式", "field-label"), method)
	if u.field("importMethod").Text() == "file" {
		v.pack(row, v.button("选择文件", "link icon-upload", false, false, func() {
			if filename := chooseFile("新增规则配置", "*"); filename != "" {
				u.field("importPath").SetText(filename)
				u.previewConfig(filename, filepath.Base(filename))
			}
		}))
	}
	v.pack(box, row)
	if u.field("importMethod").Text() == "url" {
		v.pack(box, v.box("field", false, 7, false, v.label("URL", "field-label"),
			v.box("", true, 12, false, v.box("", false, 0, true, v.input("importURL", "https://…", false, false, false)), v.button("解析", "primary icon-refresh", false, false, func() {
				address := strings.TrimSpace(u.field("importURL").Text())
				parsed, err := url.Parse(address)
				if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
					u.err = "配置地址无效"
					return
				}
				name := path.Base(parsed.Path)
				if name == "." || name == "/" {
					name = "import.conf"
				}
				u.previewConfig(address, name)
			}))))
	}
	if u.field("importMethod").Text() == "file" && u.field("importPath").Text() != "" {
		v.pack(box, v.dataLabel(u.field("importPath").Text(), "help"))
	}
	v.pack(box, v.box("field", false, 8, true, v.label("配置内容", "field-label"), v.input("fileContent", "", true, false, false)))
}
