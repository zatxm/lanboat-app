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

func (v *nativeView) compactFormRow(title string, control *widget, trailing ...*widget) *widget {
	label := v.settingLabel(title, 0)
	label.classes = "compact-form-key"
	row := v.box("", true, 12, false, label, control)
	v.formAlignmentPending = true
	v.pack(row, trailing...)
	return row
}

// Let Fego measure visible, translated labels before sizing the shared column.
func (v *nativeView) alignCompactForm() {
	if !v.formAlignmentPending {
		return
	}
	v.formAlignmentPending = false
	var keys []*widget
	for _, w := range v.widgets {
		if w.classes == "compact-form-key" {
			keys = append(keys, w)
			w.pointer.Width(0)
		}
	}
	if len(keys) < 2 {
		return
	}
	if v.tester != nil {
		v.tester.Step()
	} else if painter, ok := v.events.(interface{ PaintIfDirty() }); ok {
		painter.PaintIfDirty()
	}
	var width float32
	for _, key := range keys {
		visible := true
		for w := key; w != nil; w = w.parent {
			visible = visible && w.visible
		}
		if visible {
			width = max(width, key.pointer.Bounds().W)
		}
	}
	for _, key := range keys {
		key.pointer.Width(width)
	}
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
