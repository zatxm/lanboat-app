package native

import "github.com/zatxm/fego/v26/ui"

func (v *nativeView) settingText(value func() string) *widget {
	w := v.dataLabel(value(), "node-name")
	w.update = func() { w.setText(value()) }
	return w
}
func (v *nativeView) settingValue(title string, value func() string) *widget {
	return v.box("", true, 12, false, v.label(title, "field-label tight-label"), v.settingText(value))
}
func (v *nativeView) alignedSetting(title string, control *widget) *widget {
	return v.settingControlRow(title, control, 132)
}
func (v *nativeView) settingControlRow(title string, control *widget, labelWidth float32) *widget {
	return v.box("", true, 12, false, v.settingLabel(title, labelWidth), control)
}
func (v *nativeView) settingLabel(title string, width float32) *widget {
	label := v.box("", true, 0, false, v.label(title, "field-label"))
	label.pointer.Width(width).Justify(ui.End)
	return label
}

func (v *nativeView) formRow(title string, control *widget, trailing ...*widget) *widget {
	if hasClass(control.classes, "switch") {
		return v.box("", true, 12, false, append([]*widget{v.label(title, "node-name"), control}, trailing...)...)
	}
	if control.kind == "select" || (control.kind == "entry" || control.kind == "text") && !control.readonly {
		control.fieldTitle = title
		control.localizeFieldTitle()
		if len(trailing) == 0 {
			if control.kind != "text" {
				control.setExpand(false)
			}
			return control
		}
		control.setExpand(true)
		return v.box("", true, 10, false, append([]*widget{control}, trailing...)...)
	}
	if title == "" {
		return control
	}
	return v.box("", false, 8, false, v.label(title, "field-label"), control)
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
