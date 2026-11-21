package native

import "github.com/zatxm/fego/ui"

func (v *nativeView) settingText(value func() string) *widget {
	w := v.dataLabel(value(), "node-name")
	w.update = func() { w.setText(value()) }
	return w
}
func (v *nativeView) settingValue(title string, value func() string) *widget {
	return v.box("", true, 12, false, v.label(title, "field-label tight-label"), v.settingText(value))
}
func (v *nativeView) alignedSetting(title string, control *widget) *widget {
	label := v.box("", true, 0, false, v.label(title, "field-label"))
	label.pointer.Width(132).Justify(ui.End)
	return v.box("", true, 12, false, label, control)
}
func (v *nativeView) alignedSettingValue(title string, value func() string) *widget {
	return v.alignedSetting(title, v.settingText(value))
}
func (v *nativeView) settingsSummary(icon, title string, edit func(), children ...*widget) {
	panel := v.box("panel proxy-panel", false, 16, false, v.box("", true, 10, false, v.icon(icon), v.label(title, "connectivity-title"), v.spacer(), v.button("修改", "link icon-only icon-edit", false, false, edit)))
	v.pack(panel, children...)
	v.pack(v.body, panel)
}
func (v *nativeView) dialogActions(box *widget, save func()) {
	v.pack(box, v.spacer(), v.box("", true, 10, false, v.spacer(), v.button("取消", "secondary icon-close", false, false, v.ui.closePopup), v.button("确定", "primary icon-check", false, false, save)))
}
