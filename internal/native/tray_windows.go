package native

import (
	"fyne.io/systray"
	"github.com/zatxm/lanboat-app/packaging"
)

func registerTray() func() {
	// Fego dispatches thread messages for both HWNDs on this UI thread.
	systray.Register(nil, nil)
	systray.SetIcon(icons.ICO)
	systray.SetTooltip("Lanboat")
	return systray.Quit
}
