package native

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"strconv"
)

func (u *UI) openTCP() { u.openDetail("tcp-settings") }
func (u *UI) editTCP() {
	s := u.state.Config.Settings.TCP
	u.field("tcp.fingerprint").SetText(s.DefaultFingerprint())
	u.field("tcp.maxRead").SetText(strconv.Itoa(s.ReadLimit()))
	u.popup = &popup{kind: "tcp-settings", title: "TCP"}
	u.notify()
}
func (u *UI) saveTCP() {
	s := u.state.Config.Settings
	n, _ := strconv.Atoi(u.field("tcp.maxRead").Text())
	s.TCP = profile.TCPSettings{Fingerprint: u.field("tcp.fingerprint").Text(), MaxRead: &n}
	if err := s.Validate(); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.savingSettings = true
	u.submit(func() error { return u.backend.SaveSettings(s) }, func() { u.state.Config.Settings = s; u.popup = nil })
}
func (v *nativeView) tcpPage() {
	u := v.ui
	v.settingsSummary("tcp", "TCP", u.editTCP, v.alignedSettingValue("TLS", func() string { return "Go/uTLS" }), v.alignedSettingValue("指纹", func() string { return u.state.Config.Settings.TCP.DefaultFingerprint() }), v.alignedSettingValue("最大读取长度", func() string {
		n := u.state.Config.Settings.TCP.ReadLimit()
		if n == 0 {
			return v.translate("0（不限制长度）")
		}
		return strconv.Itoa(n)
	}))
}
func (v *nativeView) tcpEditor(box *widget) {
	u := v.ui
	opts := make([]option, 0, len(profile.TCPFingerprints))
	for _, fp := range profile.TCPFingerprints {
		opts = append(opts, option{fp, fp})
	}
	choice := func(title, key string, options []option) *widget {
		w := v.selectWidget(u.field(key).Text(), options, false, func(value string) { u.field(key).SetText(value) })
		v.inputs[key] = w
		w.setClasses("protocol-choice")
		return v.alignedSetting(title, w)
	}
	v.pack(box, v.alignedSettingValue("TLS", func() string { return "Go/uTLS" }), choice("指纹", "tcp.fingerprint", opts), v.label("节点未指定指纹时使用此设置；节点自己的指纹设置优先。", "help"), choice("最大读取长度", "tcp.maxRead", []option{{"0", "0（不限制长度）"}, {"4096", "4096"}, {"8192", "8192"}, {"16384", "16384"}}))
	v.dialogActions(box, u.saveTCP)
}
