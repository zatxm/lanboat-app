package native

// Operation feedback is acknowledged explicitly. Progress stays nonblocking.
func (v *nativeView) dismissBanner(index int) {
	switch index {
	case 0:
		v.ui.err = ""
	case 1:
		v.ui.notice = ""
		v.ui.testNotice = ""
	case 2:
		v.dismissedCoreError = v.ui.status.Error
	}
	v.ui.notify()
}
func (v *nativeView) refreshBanners() {
	u := v.ui
	for _, row := range v.bannerRows {
		row.setVisible(false)
	}
	for _, w := range []*widget{v.alert, v.notice, v.coreError} {
		w.setVisible(false)
	}
	if u.status.Error == "" {
		v.dismissedCoreError = ""
	}
	text, title := "", "提示"
	if u.err != "" {
		text, title = u.err, "操作失败"
		u.err = ""
	} else if u.notice != "" {
		text = u.notice
		u.notice = ""
	} else if u.status.Error != "" && u.status.Error != v.dismissedCoreError {
		text, title = u.status.Error, "连接失败"
		v.dismissedCoreError = u.status.Error
	}
	if text != "" {
		if u.popup != nil && u.popup.kind == "message" {
			u.popup.link += "\n" + text
			v.popupShown = nil
		} else {
			u.popup = &popup{kind: "message", title: title, link: text, previous: u.popup}
		}
	}
}
