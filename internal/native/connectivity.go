package native

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"slices"
	"strconv"
	"strings"
)

func (u *UI) openConnectivity() { u.openDetail("test-settings") }
func (u *UI) editConnectivity(speed bool) {
	s := u.state.Config.Settings
	prefix, target, title, defaults := "test", s.TestTarget(), "延迟测试", profile.TestURLs
	if speed {
		prefix, target, title, defaults = "speed", s.SpeedTarget(), "测速", profile.SpeedURLs
	}
	choice := target
	if !slices.Contains(defaults, target) {
		choice = "custom"
	}
	u.field(prefix + ".choice").SetText(choice)
	u.field(prefix + ".custom").SetText(target)
	u.field("testTimeout").SetText(strconv.Itoa(s.TestSeconds()))
	u.field("testConcurrency").SetText(strconv.Itoa(s.TestWorkers()))
	u.popup = &popup{kind: prefix + "-settings", title: title}
	u.notify()
}
func (u *UI) connectivityTarget(prefix string) string {
	value := u.field(prefix + ".choice").Text()
	if value == "custom" {
		return strings.TrimSpace(u.field(prefix + ".custom").Text())
	}
	return value
}
func (u *UI) saveConnectivity(speed bool) {
	prefix := "test"
	if speed {
		prefix = "speed"
	}
	target := u.connectivityTarget(prefix)
	if target == "" {
		u.err = "请填写URL"
		u.notify()
		return
	}
	s := u.state.Config.Settings
	if speed {
		s.SpeedURL = target
		s.SpeedURLs = nil
	} else {
		timeout, e := strconv.Atoi(u.field("testTimeout").Text())
		workers, e2 := strconv.Atoi(u.field("testConcurrency").Text())
		if e != nil || timeout < 1 || timeout > 300 {
			u.err = "超时时长需在1–300秒之间"
			u.notify()
			return
		}
		if e2 != nil || workers < 1 || workers > 20 {
			u.err = "并发数量需在1–20之间"
			u.notify()
			return
		}
		s.TestURL = target
		s.TestTimeout = timeout
		s.TestConcurrency = workers
		s.TestMethod = ""
		s.TestURLs = nil
	}
	if err := s.Validate(); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.savingSettings = true
	u.submit(func() error { return u.backend.SaveSettings(s) }, func() { u.state.Config.Settings = s; u.popup = nil })
}
func (v *nativeView) connectivityPage() {
	u := v.ui
	section := func(speed bool) {
		title, icon := "延迟测试", "test"
		applied := func() string { return u.state.Config.Settings.TestTarget() }
		var test func()
		if speed {
			title, icon = "测速", "speed"
			applied = func() string { return u.state.Config.Settings.SpeedTarget() }
			test = func() { u.openExternal(applied()) }
		}
		value := v.dataLabel(applied(), "node-name applied-url")
		value.setExpand(true)
		value.update = func() { value.setText(applied()) }
		row := v.box("", true, 8, false, v.label("URL", "field-label"), value)
		if speed {
			v.pack(row, v.button(title, "link icon-only icon-"+icon, false, false, test))
		}
		v.pack(row, v.button("修改", "link icon-only icon-edit", false, false, func() { u.editConnectivity(speed) }))
		panel := v.box("panel proxy-panel", false, 14, false,
			v.box("", true, 10, false, v.icon(icon), v.label(title, "connectivity-title")),
			row)
		if !speed {
			v.pack(panel, v.box("", true, 32, false, v.settingValue("超时时长（秒）", func() string { return strconv.Itoa(u.state.Config.Settings.TestSeconds()) }), v.settingValue("并发数量", func() string { return strconv.Itoa(u.state.Config.Settings.TestWorkers()) })))
		}
		v.pack(v.body, panel)
	}
	section(true)
	section(false)
}
func (v *nativeView) connectivityEditor(box *widget, speed bool) {
	u := v.ui
	prefix, defaults := "test", profile.TestURLs
	if speed {
		prefix, defaults = "speed", profile.SpeedURLs
	}
	labelWidth := float32(108)
	if speed {
		labelWidth = 76
	}
	v.pack(box, v.urlFields(prefix, defaults, labelWidth))
	if !speed {
		timeout := v.input("testTimeout", "15", false, false, false)
		concurrency := v.input("testConcurrency", "20", false, false, false)
		timeout.pointer.Width(120)
		concurrency.pointer.Width(120)
		v.pack(box, v.settingControlRow("超时时长（秒）", timeout, labelWidth), v.settingControlRow("并发数量", concurrency, labelWidth))
	}
	v.dialogActions(box, func() { u.saveConnectivity(speed) })
}

func (v *nativeView) urlFields(prefix string, defaults []string, labelWidth float32) *widget {
	u := v.ui
	row := func(label string, control *widget) *widget {
		if labelWidth == 0 {
			return v.compactFormRow(label, control)
		}
		return v.settingControlRow(label, control, labelWidth)
	}
	opts := make([]option, 0, len(defaults)+1)
	for _, target := range defaults {
		opts = append(opts, option{target, target})
	}
	opts = append(opts, option{"custom", "自定义"})
	input := v.input(prefix+".custom", "https://example.com", false, false, false)
	input.setExpand(true)
	custom := row("自定义URL", input)
	custom.setVisible(u.field(prefix+".choice").Text() == "custom")
	choice := v.selectWidget(u.field(prefix+".choice").Text(), opts, false, func(value string) {
		u.field(prefix + ".choice").SetText(value)
		custom.setVisible(value == "custom")
		v.formAlignmentPending = labelWidth == 0 || v.formAlignmentPending
		fitNativeDialog()
	})
	choice.setExpand(true)
	v.inputs[prefix+".choice"] = choice
	return v.box("", false, 14, false, row("URL", choice), custom)
}

func (u *UI) editGroupTestURL() {
	target := u.field("group.url").Text()
	choice := target
	if !slices.Contains(profile.TestURLs, target) {
		choice = "custom"
	}
	u.field("group.test.choice").SetText(choice)
	u.field("group.test.custom").SetText(target)
	u.popup = &popup{kind: "group-test-settings", title: "测速URL", previous: u.popup}
	u.notify()
}
func (v *nativeView) groupTestURLEditor(box *widget) {
	u := v.ui
	v.pack(box, v.urlFields("group.test", profile.TestURLs, 0))
	v.dialogActions(box, func() {
		target := u.connectivityTarget("group.test")
		if target == "" {
			u.err = "请填写URL"
			u.notify()
			return
		}
		settings := u.state.Config.Settings
		settings.TestURL = target
		if err := settings.Validate(); err != nil {
			u.err = err.Error()
			u.notify()
			return
		}
		u.field("group.url").SetText(target)
		u.closePopup()
	})
}
