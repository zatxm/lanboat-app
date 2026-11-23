package native

import (
	"net"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/engine"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func (v *nativeView) nodeField(spec profile.Field) *widget {
	u := v.ui
	id, title := "opt."+spec.Key, spec.Title
	if spec.Required {
		title += " *"
	}
	if spec.Secret && u.draft.ID != "" && !(u.draft.Kind() == "hysteria2" && spec.Key == "obfs-password") {
		title += "（留空保留）"
	}
	if spec.Kind == "switch" {
		w := v.button(spec.Title, "switch", false, false, func() {
			value := strconv.FormatBool(u.field(id).Text() != "true")
			u.field(id).SetText(value)
			if conditionalNodeField(spec.Key) {
				u.changeSSChoice(spec.Key, value)
			}
		})
		if spec.Hint != "" {
			w.setTooltip(spec.Hint)
		}
		w.update = func() {
			classes := "button switch"
			if u.field(id).Text() == "true" {
				classes += " switch-on"
			}
			w.setClasses(classes)
		}
		v.inputs[id] = w
		return v.nodeRow(title, w)
	}
	if spec.Kind == "choice" {
		options := make([]option, 0, len(spec.Choices))
		for _, value := range spec.Choices {
			label := value
			switch value {
			case "":
				label = "默认"
			case "true":
				label = "开启"
			case "false":
				label = "关闭"
			}
			if spec.Key == "uot-version" {
				label = map[string]string{"0": "关闭", "1": "1 (sp.udp-over-tcp.arpa)", "2": "2 (sp.v2.udp-over-tcp.arpa)"}[value]
			}
			if spec.Key == "lua-file" && value == "custom" {
				label = "本地文件…"
			}
			options = append(options, option{value, label})
		}
		w := v.selectWidget(u.field(id).Text(), options, false, func(value string) {
			if spec.Key == "lua-file" && value == "custom" {
				if path := chooseLuaFile(); path != "" {
					u.importLuaFile(path)
				} else {
					u.render()
				}
				return
			}
			u.field(id).SetText(value)
			if conditionalNodeField(spec.Key) {
				u.changeSSChoice(spec.Key, value)
			}
		})
		if spec.Hint != "" {
			w.setTooltip(spec.Hint)
		}
		v.inputs[id] = w
		return v.nodeRow(title, w)
	}
	if spec.Kind == "multiline" {
		return v.nodeRow(title, v.input(id, spec.Placeholder, true, false, false))
	}
	var row *widget
	if spec.Hint == "" {
		row = v.field(id, title, spec.Placeholder, spec.Secret)
	} else {
		row = v.nodeRow(title, v.box("", true, 12, false, v.box("", false, 0, true, v.input(id, spec.Placeholder, false, spec.Secret, false)), v.label(spec.Hint, "help")))
	}
	if spec.Key == "alpn" {
		v.inputs[id].event = func(event, value string) {
			u.field(id).SetText(value)
			u.field("opt.http2").SetText(strconv.FormatBool(strings.TrimSpace(value) == "h2"))
		}
	}
	if !spec.Required && (spec.Key == "plugin-opts.server" || spec.Key == "plugin-opts.port") {
		v.inputs[id].setTooltip("不指定插件服务器地址和端口将使用节点的地址和端口")
	}
	return row
}

func (v *nativeView) shadowsocksFields(box *widget) {
	u := v.ui
	v.pack(box, v.field("server", "地址 *", "域名或IP", false), v.field("nodePort", "端口 *", "1–65535", false))
	password := "密码 *"
	if u.draft.ID != "" {
		password = "密码（留空保留原密码）"
	}
	v.pack(box, v.field("password", password, "必填", true))
	methods := make([]option, 0, len(u.state.Methods))
	for _, m := range u.state.Methods {
		methods = append(methods, option{m, m})
	}
	w := v.selectWidget(u.draft.Method, methods, false, func(value string) {
		u.draft.Method = value
		if u.draft.Kind() == "ssr" {
			u.draft.SetOption("cipher", value)
		}
	})
	v.inputs["method"] = w
	v.pack(box, v.nodeRow("算法", w))
	fields := profile.ShadowsocksFields(u.draft)
	var obfsFields []profile.Field
	for _, field := range fields[4:] {
		if !strings.HasPrefix(field.Key, "plugin-opts.") {
			if field.Key == "obfs-host" || field.Key == "obfs-path" || field.Key == "obfs-param" || strings.HasPrefix(field.Key, "xhttp-") || strings.HasPrefix(field.Key, "xhttp.") {
				field.Group = "混淆设置"
			}
			obfsFields = append(obfsFields, field)
		}
	}
	v.packNodeFields(box, obfsFields)
	plugins := make([]option, 0, len(profile.SSPlugins))
	for _, p := range profile.SSPlugins {
		plugins = append(plugins, option{p, p})
	}
	plugin := u.draft.Plugin
	if plugin == "" {
		plugin = "none"
	}
	w = v.selectWidget(plugin, plugins, false, func(value string) { u.changeSSChoice("plugin", value) })
	v.inputs["plugin"] = w
	v.pack(box, v.nodeRow("插件", w))
	var pluginFields []profile.Field
	for _, field := range fields {
		if strings.HasPrefix(field.Key, "plugin-opts.") {
			if field.Group == "" {
				field.Group = "插件设置"
			} else {
				field.Group = "插件设置/" + field.Group
			}
			if u.draft.Plugin != "shadow-tls" && (field.Key == "plugin-opts.sni" || field.Key == "plugin-opts.skip-cert-verify") {
				field.Group += "/传输设置"
			}
			pluginFields = append(pluginFields, field)
		}
	}
	v.packNodeFields(box, pluginFields)
	row := v.box("", false, 4, false)
	for _, field := range fields[:3] {
		v.pack(row, v.nodeField(field))
	}
	v.pack(box, row, v.nodeField(fields[3]), v.field("name", "备注", "可选", false))
	issue := v.label("", "help")
	issue.update = func() {
		n := u.draft.Clone()
		n.SetOption("ota", u.field("opt.ota").Text() == "true")
		text := engine.ShadowsocksIssue(n)
		issue.setText(text)
		issue.setVisible(text != "")
	}
	v.pack(box, issue)
}

func (u *UI) changeSSChoice(key, value string) {
	for _, field := range profile.NodeFields(u.draft) {
		text := u.field("opt." + field.Key).Text()
		if field.Secret && text == "" {
			continue
		}
		var value any = text
		if field.Kind == "switch" {
			value = text == "true"
		}
		if field.Kind == "number" {
			if n, err := strconv.Atoi(text); err == nil {
				value = n
			}
		}
		u.draft.SetOption(field.Key, value)
	}
	if key == "plugin" {
		if value == "none" {
			value = ""
		}
		u.draft.Plugin, u.draft.PluginOpts = value, ""
		u.field("pluginOpts").SetText("")
		delete(u.draft.Options, "plugin-opts")
	} else {
		u.draft.SetOption(key, value)
	}
	for _, field := range profile.NodeFields(u.draft) {
		text := u.draft.TextOption(field.Key)
		if field.Key == "plugin" {
			text = u.draft.Plugin
		}
		if text == "" {
			text = field.Default
		}
		u.field("opt." + field.Key).SetText(text)
	}
	u.render()
}

func ssRemark(server string, port int) string { return net.JoinHostPort(server, strconv.Itoa(port)) }
