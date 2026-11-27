package native

import "strconv"

func (u *UI) editRuleUpdate() {
	s := u.state.Config.Settings
	u.field("ruleUpdate.auto").SetText(strconv.FormatBool(s.RulesAutoUpdate()))
	u.field("ruleUpdate.days").SetText(strconv.Itoa(s.RulesUpdateDays()))
	u.popup = &popup{kind: "rule-update-settings", title: "自动更新"}
	u.notify()
}
func (u *UI) saveRuleUpdate() {
	s := u.state.Config.Settings
	enabled := u.field("ruleUpdate.auto").Text() == "true"
	s.RuleAutoUpdate = &enabled
	s.RuleUpdateDays, _ = strconv.Atoi(u.field("ruleUpdate.days").Text())
	if err := s.Validate(); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	u.savingSettings = true
	u.submit(func() error { return u.backend.SaveSettings(s) }, func() { u.state.Config.Settings = s; u.popup = nil })
}
func (v *nativeView) ruleUpdateEditor(box *widget) {
	u := v.ui
	opts := make([]option, 0, 7)
	for day := 1; day <= 7; day++ {
		opts = append(opts, option{strconv.Itoa(day), strconv.Itoa(day) + " 天"})
	}
	choice := v.selectWidget(u.field("ruleUpdate.days").Text(), opts, false, func(value string) { u.field("ruleUpdate.days").SetText(value) })
	choice.pointer.Width(112)
	v.inputs["ruleUpdate.days"] = choice
	interval := v.formRow("更新间隔", choice)
	interval.setVisible(u.field("ruleUpdate.auto").Text() == "true")
	toggle := v.button("自动更新", "switch", false, false, func() {
		enabled := u.field("ruleUpdate.auto").Text() != "true"
		u.field("ruleUpdate.auto").SetText(strconv.FormatBool(enabled))
		interval.setVisible(enabled)
		fitNativeDialog()
	})
	toggle.update = func() {
		class := "button switch"
		if u.field("ruleUpdate.auto").Text() == "true" {
			class += " switch-on"
		}
		toggle.setClasses(class)
	}
	v.pack(box, v.formRow("自动更新", toggle), interval)
	v.dialogActions(box, u.saveRuleUpdate)
}
