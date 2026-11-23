package native

import (
	"context"
	"time"

	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/desktop"
)

func (u *UI) checkForUpdate(about *popup) {
	if about.checkingUpdate {
		return
	}
	about.checkingUpdate = true
	u.notify()
	go func() {
		release, err := u.backend.LatestAppRelease(context.Background())
		u.post(update{background: true, apply: func() {
			about.checkingUpdate = false
			if u.popup != about {
				return
			}
			if err != nil {
				u.err = "无法检查更新，请检查网络连接或稍后重试。"
				return
			}
			u.showAppRelease(about, release, appmeta.CommitTime())
		}})
	}()
}

func (u *UI) showAppRelease(about *popup, release desktop.AppRelease, installed time.Time) {
	if release.Tag == "" {
		u.notice = "GitHub 暂无正式版本。"
		return
	}
	if !installed.IsZero() && !release.CommitTime.After(installed) {
		u.notice = "当前没有可用的更新。"
		return
	}
	title := "发现新版本"
	if installed.IsZero() {
		title = "GitHub 最新正式版本"
	}
	u.popup = &popup{kind: "app-update", title: title, link: release.Page, releaseTag: release.Tag, previous: about}
}
