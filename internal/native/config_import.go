package native

import (
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/zatxm/fego/v26/ui"
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

func (v *nativeView) configNameField(placeholder string, readOnly bool) *widget {
	name := v.input("fileName", placeholder, false, false, readOnly)
	name.setExpand(true)
	return v.compactFormRow("名称", name)
}

func (v *nativeView) configContentField(readOnly bool) *widget {
	content := v.input("fileContent", "", true, false, readOnly)
	content.pointer.FillHeight()
	row := v.compactFormRow("配置内容", content)
	row.pointer.AlignItems(ui.Start)
	row.setExpand(true)
	return row
}

func (v *nativeView) configImportFields(box *widget) {
	u := v.ui
	const fieldWidth = 228
	name := v.input("fileName", "例如：custom.conf", false, false, false)
	name.pointer.Width(fieldWidth)
	v.pack(box, v.compactFormRow("名称", name))
	method := v.selectWidget(u.field("importMethod").Text(), []option{{"manual", "手动输入"}, {"file", "导入文件"}, {"url", "URL"}}, false, func(value string) {
		u.field("importMethod").SetText(value)
		u.err = ""
		u.render()
	})
	method.pointer.Width(fieldWidth)
	v.inputs["importMethod"] = method
	methodRow := v.compactFormRow("方式", method)
	v.pack(box, methodRow)
	switch u.field("importMethod").Text() {
	case "file":
		v.pack(methodRow, v.button("选择文件", "link icon-upload", false, false, func() {
			u.pickFile("新增规则配置", "*", func(filename string) {
				if filename != "" {
					u.field("importPath").SetText(filename)
					u.previewConfig(filename, filepath.Base(filename))
				}
			})
		}))
		if filename := u.field("importPath").Text(); filename != "" {
			name := v.dataLabel(filepath.Base(filename), "help node-title")
			name.setTooltip(filename)
			v.pack(methodRow, name)
		}
	case "url":
		address := v.input("importURL", "https://…", false, false, false)
		address.pointer.Width(fieldWidth)
		parse := v.button("解析", "primary icon-refresh", false, false, func() {
			input := strings.TrimSpace(u.field("importURL").Text())
			parsed, err := url.Parse(input)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
				u.err = "配置地址无效"
				u.notify()
				return
			}
			name := path.Base(parsed.Path)
			if name == "." || name == "/" {
				name = "import.conf"
			}
			u.previewConfig(input, name)
		})
		v.pack(box, v.compactFormRow("URL", address, parse))
	}
	v.pack(box, v.configContentField(false))
}
