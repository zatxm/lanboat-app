package native

import (
	"strings"

	"github.com/zatxm/lanboat-app/internal/engine"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func (v *nativeView) nodeRow(title string, control *widget, below ...*widget) *widget {
	row := v.formRow(title, control)
	if len(below) == 0 {
		return row
	}
	return v.box("", false, 8, false, append([]*widget{row}, below...)...)
}

func conditionalNodeField(key string) bool {
	switch key {
	case "obfs", "obfs-opts.mode", "lua-file", "plugin", "plugin-opts.Transport", "plugin-opts.mode", "network", "tls", "fragment", "mutual-auth", "trojan-protocol", "private-key-enabled", "version", "http-mask", "http-mask-mode":
		return true
	}
	return false
}

// A nested group belongs to its controller and retains the existing form draft.
func (v *nativeView) packNodeFields(box *widget, fields []profile.Field) {
	groups := map[string]*widget{}
	for _, field := range fields {
		parent := box
		if field.Group != "" {
			key := ""
			for _, title := range strings.Split(field.Group, "/") {
				key += "/" + title
				group := groups[key]
				if group == nil {
					group = v.box("option-group", false, 12, false, v.label(title, "field-label"))
					groups[key] = group
					v.pack(parent, group)
				}
				parent = group
			}
		}
		v.pack(parent, v.nodeField(field))
		if field.Key == "amnezia-wg-option.jc" {
			v.pack(parent, v.label("AmneziaWG 3.1", "help"), v.button("一键随机", "link", false, false, func() {
				for key, value := range profile.RandomAmneziaOptions() {
					v.ui.field("opt.amnezia-wg-option." + key).SetText(value)
				}
				v.ui.render()
			}), v.label("随机生成会替换当前混淆参数；S1–S4、H1–H4 和头部保护密钥需同步到服务端。I1–I5 保留原值。", "help"))
		}
	}
}
func (v *nativeView) detailedNodeFields(box *widget) {
	u := v.ui
	if u.draft.Kind() != "tailscale" {
		v.pack(box, v.field("server", "地址 *", "域名或IP", false), v.field("nodePort", "端口 *", "1–65535", false))
	}
	fields := profile.NodeFields(u.draft)
	for len(fields) > 0 && (fields[0].Key == "username" || (u.draft.Kind() == "tuic" || u.draft.Kind() == "juicity") && (fields[0].Key == "version" || fields[0].Key == "uuid")) {
		v.pack(box, v.nodeField(fields[0]))
		fields = fields[1:]
	}
	if profile.NodePasswordField(u.draft.Kind()) {
		title, hint := "密码 *", "必填"
		if u.draft.Kind() == "shadowtls" {
			title, hint = "密码", "可选"
		}
		if u.draft.ID != "" {
			title = "密码（留空保留原密码）"
		}
		v.pack(box, v.field("password", title, hint, true))
	}
	v.packNodeFields(box, fields)
	v.pack(box, v.remarkField("可选"))
	issue := v.label("", "help")
	issue.update = func() {
		n := u.draft.Clone()
		for _, field := range profile.NodeFields(n) {
			if !field.Secret {
				n.SetOption(field.Key, u.field("opt."+field.Key).Text())
			}
		}
		text := engine.NodeOptionsIssue(n)
		issue.setText(text)
		issue.setVisible(text != "")
	}
	v.pack(box, issue)
}
