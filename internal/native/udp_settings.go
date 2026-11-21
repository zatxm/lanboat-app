package native

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"strconv"
)

func (u *UI) openUDP() { u.openDetail("udp-settings") }
func (u *UI) editUDP() {
	s := u.state.Config.Settings.UDP
	for k, v := range map[string]string{"relay": strconv.FormatBool(s.RelayEnabled()), "stun": strconv.FormatBool(s.BlockSTUN), "ports": s.Ports, "timeout": strconv.Itoa(s.Seconds())} {
		u.field("udp." + k).SetText(v)
	}
	u.popup = &popup{kind: "udp-settings", title: "UDP"}
	u.notify()
}
func (u *UI) saveUDP() {
	seconds, err := strconv.Atoi(u.field("udp.timeout").Text())
	if err != nil || seconds < 1 || seconds > 3600 {
		u.err = "UDP超时需在1–3600秒之间"
		u.notify()
		return
	}
	relay := u.field("udp.relay").Text() == "true"
	s := u.state.Config.Settings
	s.UDP = profile.UDPSettings{Relay: &relay, BlockSTUN: u.field("udp.stun").Text() == "true", Ports: u.field("udp.ports").Text(), Timeout: seconds}
	if err := s.Validate(); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.savingSettings = true
	u.submit(func() error { return u.backend.SaveSettings(s) }, func() { u.state.Config.Settings = s; u.popup = nil })
}
func (v *nativeView) udpPage() {
	u := v.ui
	s := func() profile.UDPSettings { return u.state.Config.Settings.UDP }
	v.settingsSummary("udp", "UDP", u.editUDP, v.alignedSettingValue("启用转发", func() string { return v.translate(onOff(s().RelayEnabled())) }), v.alignedSettingValue("禁用STUN", func() string { return v.translate(onOff(s().BlockSTUN)) }), v.alignedSettingValue("运行端口", func() string {
		if s().Ports == "" {
			return v.translate("全部")
		}
		return s().Ports
	}), v.alignedSettingValue("超时（秒）", func() string { return strconv.Itoa(s().Seconds()) }))
	v.pack(v.body, v.label("*现在仅Shadowsocks、ShadowsocksR、Socks5、Socks5overTLS、VMess、Hysteria、Hysteria2、Relay、Trojan和VLESS协议支持UDP中继", "help"))
}
func (v *nativeView) udpEditor(box *widget) {
	u := v.ui
	toggle := func(key, title, hint string) *widget {
		b := v.button(title, "switch", false, false, func() { u.field("udp." + key).SetText(strconv.FormatBool(u.field("udp."+key).Text() != "true")) })
		b.update = func() {
			class := "button switch"
			if u.field("udp."+key).Text() == "true" {
				class += " switch-on"
			}
			b.setClasses(class)
		}
		b.setTooltip(hint)
		return v.box("", false, 8, false, v.alignedSetting(title, b), v.label(hint, "help"))
	}
	v.pack(box,
		toggle("relay", "启用转发", "如果开启中继则使用规则配置转发UDP流量，否则全部直连转发UDP流量"),
		toggle("stun", "禁用STUN", "开启此选项可以防止WebRTC泄漏真实IP"),
		v.alignedSetting("运行端口", v.input("udp.ports", "", false, false, false)),
		v.label("留空则允许所有端口，0则禁止所有端口", "help"),
		v.alignedSetting("超时（秒）", v.input("udp.timeout", "30", false, false, false)))
	v.dialogActions(box, u.saveUDP)
}
