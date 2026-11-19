package native

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func installWindowIcon(title string) func() {
	user := windows.NewLazySystemDLL("user32.dll")
	module, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	name, _ := windows.UTF16PtrFromString(title)
	hwnd, _, _ := user.NewProc("FindWindowW").Call(0, uintptr(unsafe.Pointer(name)))
	if hwnd == 0 {
		return nil
	}
	for kind, size := range []uintptr{16, 32} {
		icon, _, _ := user.NewProc("LoadImageW").Call(module, 1, 1, size, size, 0x8000)
		if icon != 0 {
			user.NewProc("SendMessageW").Call(hwnd, 0x80, uintptr(kind), icon)
		} // WM_SETICON, lifetime of the main window
	}
	return func() {
		if iconic, _, _ := user.NewProc("IsIconic").Call(hwnd); iconic != 0 {
			user.NewProc("ShowWindow").Call(hwnd, 9)
		}
		user.NewProc("SetForegroundWindow").Call(hwnd)
	}
}
