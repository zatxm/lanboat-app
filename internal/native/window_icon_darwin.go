//go:build darwin && !fltk

package native

import (
	"structs"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/zatxm/fego"
	"github.com/zatxm/lanboat-app/packaging"
)

func macIcon(png []byte) objc.ID {
	data := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("dataWithBytes:length:"), unsafe.Pointer(&png[0]), uintptr(len(png)))
	return objc.ID(objc.GetClass("NSImage")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithData:"), data)
}

func installWindowIcon(string) func() {
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	img := macIcon(icons.PNG)
	if img != 0 {
		app.Send(objc.RegisterName("setApplicationIconImage:"), img)
		img.Send(objc.RegisterName("release"))
	}
	win := app.Send(objc.RegisterName("keyWindow"))
	if win == 0 {
		win = app.Send(objc.RegisterName("orderedWindows")).Send(objc.RegisterName("firstObject"))
	}
	if win == 0 {
		return nil
	}
	// A native title-bar accessory preserves the standard traffic-light controls.
	image := macIcon(icons.MenuPNG)
	imageView := objc.ID(objc.GetClass("NSImageView")).Send(objc.RegisterName("imageViewWithImage:"), image)
	image.Send(objc.RegisterName("release"))
	type size struct {
		_             structs.HostLayout
		Width, Height float64
	}
	var resize func(objc.ID, objc.SEL, size)
	purego.RegisterLibFunc(&resize, purego.RTLD_DEFAULT, "objc_msgSend")
	resize(imageView, objc.RegisterName("setFrameSize:"), size{Width: 24, Height: 20})
	accessory := objc.ID(objc.GetClass("NSTitlebarAccessoryViewController")).Send(objc.RegisterName("new"))
	accessory.Send(objc.RegisterName("setView:"), imageView)
	accessory.Send(objc.RegisterName("setLayoutAttribute:"), 2) // NSLayoutAttributeRight
	win.Send(objc.RegisterName("addTitlebarAccessoryViewController:"), accessory)
	accessory.Send(objc.RegisterName("release"))
	return func() {
		win.Send(objc.RegisterName("deminiaturize:"), uintptr(0))
		app.Send(objc.RegisterName("activateIgnoringOtherApps:"), true)
		win.Send(objc.RegisterName("makeKeyAndOrderFront:"), uintptr(0))
	}
}

func macTitle(text string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), text)
}

func startSystemTray(showLabel, quitLabel string, show func()) *systemTray {
	open, quit := objc.RegisterName("showLanboat:"), objc.RegisterName("quitLanboat:")
	handlers, err := objc.RegisterClass("LanboatTrayTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: open, Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
			if show != nil {
				show()
			}
		}},
		{Cmd: quit, Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { fego.App.Quit() }},
	})
	if err != nil {
		return nil
	}
	target := objc.ID(handlers).Send(objc.RegisterName("new"))
	bar := objc.ID(objc.GetClass("NSStatusBar")).Send(objc.RegisterName("systemStatusBar"))
	var addStatus func(objc.ID, objc.SEL, float64) objc.ID
	purego.RegisterLibFunc(&addStatus, purego.RTLD_DEFAULT, "objc_msgSend")
	item := addStatus(bar, objc.RegisterName("statusItemWithLength:"), -2).Send(objc.RegisterName("retain"))
	img := macIcon(icons.MenuPNG)
	item.Send(objc.RegisterName("button")).Send(objc.RegisterName("setImage:"), img)
	img.Send(objc.RegisterName("release"))
	menu := objc.ID(objc.GetClass("NSMenu")).Send(objc.RegisterName("new"))
	add := func(text string, cmd objc.SEL) objc.ID {
		entry := menu.Send(objc.RegisterName("addItemWithTitle:action:keyEquivalent:"), macTitle(text), cmd, macTitle(""))
		entry.Send(objc.RegisterName("setTarget:"), target)
		return entry
	}
	openItem, quitItem := add(showLabel, open), add(quitLabel, quit)
	item.Send(objc.RegisterName("setMenu:"), menu)
	menu.Send(objc.RegisterName("release"))
	return &systemTray{
		relabel: func(open, quit string) {
			openItem.Send(objc.RegisterName("setTitle:"), macTitle(open))
			quitItem.Send(objc.RegisterName("setTitle:"), macTitle(quit))
		},
		close: func() {
			bar.Send(objc.RegisterName("removeStatusItem:"), item)
			item.Send(objc.RegisterName("release"))
			target.Send(objc.RegisterName("release"))
		},
	}
}
