package native

import (
	"fyne.io/systray"
	"github.com/zatxm/lanboat-app/packaging"
)

func registerTray() func() {
	systray.SetIcon(icons.TrayPNG)
	systray.SetTitle("Lanboat")
	systray.SetTooltip("Lanboat")
	start, stop := systray.RunWithExternalLoop(nil, nil)
	start()
	return stop
}
