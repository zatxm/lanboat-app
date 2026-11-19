package native

import (
	_ "embed"
	"unsafe"

	"github.com/ebitengine/purego"
)

//go:embed icon.rgba
var windowRGBA []byte

// Fego 1.0.2 exposes no window handle. Find only our uniquely titled window
// before restoring its display title; a window manager may already have reparented it.
func installWindowIcon(title string) func() {
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW)
	if err != nil {
		return nil
	}
	defer purego.Dlclose(lib)
	var open func(uintptr) uintptr
	var closeDisplay, root, grab, ungrab func(uintptr) uintptr
	var tree func(uintptr, uintptr, *uintptr, *uintptr, **uintptr, *uint32) int32
	var property func(uintptr, uintptr, uintptr, int64, int64, int32, uintptr, *uintptr, *int32, *uintptr, *uintptr, **byte) int32
	var set func(uintptr, uintptr, uintptr, uintptr, int32, int32, unsafe.Pointer, int32) int32
	var atom func(uintptr, string, int32) uintptr
	var free func(unsafe.Pointer) int32
	for name, fn := range map[string]any{"XOpenDisplay": &open, "XCloseDisplay": &closeDisplay, "XDefaultRootWindow": &root, "XGrabServer": &grab, "XUngrabServer": &ungrab, "XQueryTree": &tree, "XGetWindowProperty": &property, "XChangeProperty": &set, "XInternAtom": &atom, "XFree": &free} {
		purego.RegisterLibFunc(fn, lib, name)
	}
	d := open(0)
	if d == 0 {
		return nil
	}
	defer closeDisplay(d)
	iconAtom := atom(d, "_NET_WM_ICON", 0)
	wmName := atom(d, "WM_NAME", 0)
	pixels := make([]uintptr, 2+len(windowRGBA)/4)
	pixels[0], pixels[1] = 64, 64
	for i := 0; i < len(windowRGBA); i += 4 {
		p := windowRGBA[i : i+4]
		pixels[2+i/4] = uintptr(p[3])<<24 | uintptr(p[0])<<16 | uintptr(p[1])<<8 | uintptr(p[2])
	}
	var find func(uintptr, int) uintptr
	find = func(parent uintptr, depth int) uintptr {
		var r, p uintptr
		var list *uintptr
		var n uint32
		if tree(d, parent, &r, &p, &list, &n) == 0 || list == nil {
			return 0
		}
		defer free(unsafe.Pointer(list))
		children := unsafe.Slice(list, n)
		for i := len(children) - 1; i >= 0; i-- {
			id := children[i]
			var actual, count, remaining uintptr
			var format int32
			var data *byte
			property(d, id, wmName, 0, 32, 0, 0, &actual, &format, &count, &remaining, &data)
			if data != nil {
				match := format == 8 && count == uintptr(len(title)) && string(unsafe.Slice(data, len(title))) == title
				free(unsafe.Pointer(data))
				if match {
					return id
				}
			}
		}
		if depth > 0 {
			for i := len(children) - 1; i >= 0; i-- {
				if id := find(children[i], depth-1); id != 0 {
					return id
				}
			}
		}
		return 0
	}
	// Keep the queried XIDs valid until their properties have been installed.
	grab(d)
	defer ungrab(d)
	if id := find(root(d), 2); id != 0 {
		class := []byte("lanboat\x00Lanboat\x00")
		set(d, id, 67, 31, 8, 0, unsafe.Pointer(&class[0]), int32(len(class)))         // WM_CLASS / STRING
		set(d, id, iconAtom, 6, 32, 0, unsafe.Pointer(&pixels[0]), int32(len(pixels))) // CARDINAL
		return func() { raiseXWindow(id) }
	}
	return nil
}

// The user requested activation through the tray. Let the window manager switch
// workspaces and restore focus, even if the window is currently minimized.
func raiseXWindow(id uintptr) {
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW)
	if err != nil {
		return
	}
	defer purego.Dlclose(lib)
	var open func(uintptr) uintptr
	var closeDisplay, root func(uintptr) uintptr
	var atom func(uintptr, string, int32) uintptr
	var raise func(uintptr, uintptr) int32
	var send func(uintptr, uintptr, int32, int64, *[24]uintptr) int32
	for name, fn := range map[string]any{"XOpenDisplay": &open, "XCloseDisplay": &closeDisplay, "XDefaultRootWindow": &root, "XMapRaised": &raise, "XSendEvent": &send, "XInternAtom": &atom} {
		purego.RegisterLibFunc(fn, lib, name)
	}
	d := open(0)
	if d == 0 {
		return
	}
	defer closeDisplay(d)
	raise(d, id)
	event := [24]uintptr{0: 33, 3: d, 4: id, 5: atom(d, "_NET_ACTIVE_WINDOW", 0), 6: 32, 7: 2}
	send(d, root(d), 0, (1<<19)|(1<<20), &event)
}
