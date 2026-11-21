package native

import (
	"net"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func (u *UI) openProxy() { u.openDetail("proxy-settings") }
func (u *UI) editProxy(section string) {
	s := u.state.Config.Settings
	p := s.Proxy
	for key, value := range map[string]string{"port": strconv.Itoa(s.Port), "username": p.Username, "password": p.Password, "type": p.Upstream.Type, "server": p.Upstream.Server, "upstreamPort": strconv.Itoa(p.Upstream.Port), "upstreamUser": p.Upstream.Username, "upstreamPassword": p.Upstream.Password} {
		if key == "type" && value == "" {
			value = "socks5"
		}
		if key == "upstreamPort" && p.Upstream.Port == 0 {
			value = ""
		}
		u.field("proxy." + key).SetText(value)
	}
	for key, value := range map[string]bool{"lan": p.AllowLAN, "auth": p.Authentication, "upstream": p.Upstream.Enabled} {
		u.field("proxy." + key).SetText(strconv.FormatBool(value))
	}
	u.popup = &popup{kind: "proxy-" + section, title: map[string]string{"port": "本地监听", "sharing": "网络共享", "upstream": "前置代理"}[section]}
	u.notify()
}

func (u *UI) saveProxySection(section string) {
	text := func(key string) string { return u.field("proxy." + key).Text() }
	number := func(key string) int { n, _ := strconv.Atoi(strings.TrimSpace(text(key))); return n }
	s := u.state.Config.Settings
	if section == "port" {
		s.Port = number("port")
	}
	if section == "sharing" {
		s.Proxy.AllowLAN = text("lan") == "true"
		s.Proxy.Authentication = s.Proxy.AllowLAN && text("auth") == "true"
		s.Proxy.Username = strings.TrimSpace(text("username"))
		s.Proxy.Password = text("password")
	}
	if section == "upstream" {
		s.Proxy.Upstream = profile.UpstreamProxy{Enabled: text("upstream") == "true", Type: text("type"), Server: strings.TrimSpace(text("server")), Port: number("upstreamPort"), Username: strings.TrimSpace(text("upstreamUser")), Password: text("upstreamPassword")}
	}

	if err := s.Validate(); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.savingSettings = true
	u.submit(func() error { return u.backend.SaveSettings(s) }, func() { u.state.Config.Settings = s; u.popup = nil })
}

func (v *nativeView) copySetting(title string, value func() string, visible func() bool) *widget {
	entry := v.create("entry", props{value: value(), readOnly: true, expand: true})
	row := v.box("", true, 12, false, v.label(title, "field-label tight-label"), entry, v.button("复制", "link icon-only icon-copy", false, false, func() { copyClipboard(value()) }))
	row.update = func() {
		row.setVisible(visible())
		if entry.nativeText() != value() {
			entry.setValue(value())
		}
	}
	row.update()
	return row
}
func (v *nativeView) proxyPage() {
	u := v.ui
	s := func() profile.Settings { return u.state.Config.Settings }
	v.settingsSummary("layers", "本地监听", func() { u.editProxy("port") }, v.settingValue("端口", func() string { return strconv.Itoa(s().Port) }), v.label("HTTP与SOCKS5共用此端口。", "help"))
	auth := v.settingValue("访问认证", func() string { return v.translate(onOff(s().Proxy.Authentication)) })
	auth.update = func() { auth.setVisible(s().Proxy.AllowLAN) }
	credentials := func() bool { return s().Proxy.AllowLAN && s().Proxy.Authentication }
	v.settingsSummary("network-sharing", "网络共享", func() { u.editProxy("sharing") }, v.settingValue("允许局域网设备连接", func() string { return v.translate(onOff(s().Proxy.AllowLAN)) }), auth, v.copySetting("用户", func() string { return s().Proxy.Username }, credentials), v.copySetting("密码", func() string { return s().Proxy.Password }, credentials))
	enabled := func() bool { return s().Proxy.Upstream.Enabled }
	v.settingsSummary("route", "前置代理", func() { u.editProxy("upstream") }, v.settingValue("启用前置代理", func() string { return v.translate(onOff(enabled())) }), v.copySetting("地址", func() string {
		p := s().Proxy.Upstream
		return p.Type + "://" + net.JoinHostPort(p.Server, strconv.Itoa(p.Port))
	}, enabled), v.copySetting("用户", func() string { return s().Proxy.Upstream.Username }, func() bool { return enabled() && s().Proxy.Upstream.Username != "" }), v.copySetting("密码", func() string { return s().Proxy.Upstream.Password }, func() bool { return enabled() && s().Proxy.Upstream.Password != "" }))
}
func (v *nativeView) proxyFields(box *widget, section string) {
	u := v.ui
	row := func(title string, control *widget) *widget {
		label := v.label(title, "field-label")
		label.pointer.Width(64)
		return v.box("", true, 12, false, label, control)
	}
	field := func(key, title, placeholder string) *widget {
		input := v.input("proxy."+key, placeholder, false, false, false)
		input.setExpand(true)
		return row(title, input)
	}
	switchRow := func(title, key string, conditional *widget) *widget {
		row := v.formSwitch("proxy."+key, title)
		if conditional != nil {
			conditional.setVisible(u.field("proxy."+key).Text() == "true")
			b := v.controls[len(v.controls)-1].widget
			original := b.event
			b.event = func(event, value string) {
				original(event, value)
				conditional.setVisible(u.field("proxy."+key).Text() == "true")
				fitNativeDialog()
			}
		}
		return row
	}
	switch section {
	case "port":
		v.pack(box, field("port", "端口", "1024–65535"), v.label("HTTP与SOCKS5共用此端口。", "help"))
	case "sharing":
		auth := v.box("option-group", false, 12, false,
			field("username", "用户", "必填"),
			field("password", "密码", "必填"))
		authGroup := v.box("", false, 14, false, switchRow("访问认证", "auth", auth), auth)
		v.pack(box, switchRow("允许局域网设备连接", "lan", authGroup), v.label("关闭时仅本机可连接；开启后使用本机局域网IP和代理端口连接。", "help"), authGroup)
	case "upstream":
		hint := v.label("HTTP前置代理支持TCP；UDP协议需使用支持UDP的SOCKS5前置代理。", "help")
		hint.setVisible(u.field("proxy.type").Text() == "http")
		choice := v.selectWidget(u.field("proxy.type").Text(), []option{{"socks5", "SOCKS5"}, {"http", "HTTP"}}, false, func(value string) {
			u.field("proxy.type").SetText(value)
			hint.setVisible(value == "http")
			fitNativeDialog()
		})
		choice.setExpand(true)
		v.inputs["proxy.type"] = choice
		fields := v.box("option-group", false, 12, false,
			row("类型", choice),
			field("server", "地址", "IP"),
			field("upstreamPort", "端口", "1–65535"),
			field("upstreamUser", "用户", "可选"),
			field("upstreamPassword", "密码", "可选"),
			hint)
		v.pack(box, switchRow("启用前置代理", "upstream", fields), v.label("通过前置代理连接节点；直连流量不经过它。", "help"), fields)
	}
	v.dialogActions(box, func() { u.saveProxySection(section) })
}
