//go:build linux || windows

package native

import (
	"fyne.io/systray"
	"github.com/zatxm/fego"
)

func startSystemTray(showLabel, quitLabel string, show func()) *systemTray {
	stop := registerTray()
	openItem := systray.AddMenuItem(showLabel, "")
	quitItem := systray.AddMenuItem(quitLabel, "")
	if show == nil {
		openItem.Disable()
	}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-openItem.ClickedCh:
				if show != nil {
					fego.App.Post(show)
				}
			case <-quitItem.ClickedCh:
				fego.App.Quit()
			case <-done:
				return
			}
		}
	}()
	return &systemTray{
		relabel: func(open, quit string) { openItem.SetTitle(open); quitItem.SetTitle(quit) },
		close:   func() { close(done); stop() },
	}
}
