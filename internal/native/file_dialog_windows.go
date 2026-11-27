package native

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	createFileDialog = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	fileOpenClass    = windows.GUID{Data1: 0xdc1c5a9c, Data2: 0xe88a, Data3: 0x4dde, Data4: [8]byte{0xa5, 0xa1, 0x60, 0xf8, 0x2a, 0x20, 0xae, 0xf7}}
	fileOpenIID      = windows.GUID{Data1: 0xd57c7288, Data2: 0xd4ad, Data3: 0x4768, Data4: [8]byte{0xbe, 0x02, 0x9d, 0x96, 0x95, 0x32, 0xd9, 0x60}}
)

type fileOpenDialog struct {
	methods *struct {
		queryInterface, addRef, release, show                              uintptr
		setFileTypes, setFileTypeIndex, getFileTypeIndex, advise, unadvise uintptr
		setOptions, getOptions, setDefaultFolder, setFolder, getFolder     uintptr
		getCurrentSelection, setFileName, getFileName, setTitle            uintptr
		setOkButtonLabel, setFileNameLabel, getResult                      uintptr
	}
}

type fileDialogItem struct {
	methods *struct {
		queryInterface, addRef, release, bindToHandler, getParent, getDisplayName uintptr
	}
}

func dialogResult(result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("Windows file dialog: HRESULT 0x%08x", uint32(result))
	}
	return nil
}

func choosePath(ctx context.Context, title, pattern string, directory bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && err != syscall.Errno(1) {
		return "", err
	}
	defer windows.CoUninitialize()
	var dialog *fileOpenDialog
	result, _, _ := createFileDialog.Call(
		uintptr(unsafe.Pointer(&fileOpenClass)), 0, windows.CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(&fileOpenIID)), uintptr(unsafe.Pointer(&dialog)))
	if err := dialogResult(result); err != nil {
		return "", err
	}
	self := uintptr(unsafe.Pointer(dialog))
	defer syscall.SyscallN(dialog.methods.release, self)
	var options uint32
	result, _, _ = syscall.SyscallN(dialog.methods.getOptions, self, uintptr(unsafe.Pointer(&options)))
	if err := dialogResult(result); err != nil {
		return "", err
	}
	const (
		pickFolders     = 0x20
		forceFilesystem = 0x40
		fileMustExist   = 0x1000
	)
	if directory {
		// Forcing filesystem entries while browsing WSL folders can stall
		// the shell dialog. Validate the selected path with SIGDN_FILESYSPATH.
		options = (options &^ (fileMustExist | forceFilesystem)) | pickFolders
	} else {
		options |= forceFilesystem
	}
	result, _, _ = syscall.SyscallN(dialog.methods.setOptions, self, uintptr(options))
	if err := dialogResult(result); err != nil {
		return "", err
	}
	if title != "" {
		label, err := windows.UTF16PtrFromString(title)
		if err != nil {
			return "", err
		}
		result, _, _ = syscall.SyscallN(dialog.methods.setTitle, self, uintptr(unsafe.Pointer(label)))
		if err = dialogResult(result); err != nil {
			return "", err
		}
	}
	if !directory && pattern != "" && pattern != "*" {
		matches, err := windows.UTF16PtrFromString(pattern)
		if err != nil {
			return "", err
		}
		filter := struct{ name, pattern *uint16 }{matches, matches}
		result, _, _ = syscall.SyscallN(dialog.methods.setFileTypes, self, 1, uintptr(unsafe.Pointer(&filter)))
		if err = dialogResult(result); err != nil {
			return "", err
		}
	}
	// The modal dialog stays above Lanboat when Lanboat owns the active window.
	owner := windows.GetForegroundWindow()
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(owner, &pid); err != nil || pid != windows.GetCurrentProcessId() {
		owner = 0
	}
	result, _, _ = syscall.SyscallN(dialog.methods.show, self, uintptr(owner))
	if uint32(result) == 0x800704c7 { // HRESULT_FROM_WIN32(ERROR_CANCELLED)
		return "", nil
	}
	if err := dialogResult(result); err != nil {
		return "", err
	}
	var item *fileDialogItem
	result, _, _ = syscall.SyscallN(dialog.methods.getResult, self, uintptr(unsafe.Pointer(&item)))
	if err := dialogResult(result); err != nil {
		return "", err
	}
	itemPtr := uintptr(unsafe.Pointer(item))
	defer syscall.SyscallN(item.methods.release, itemPtr)
	var filename *uint16
	const filesystemPath = 0x80058000 // SIGDN_FILESYSPATH
	result, _, _ = syscall.SyscallN(item.methods.getDisplayName, itemPtr, filesystemPath, uintptr(unsafe.Pointer(&filename)))
	if err := dialogResult(result); err != nil {
		return "", err
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(filename))
	return windows.UTF16PtrToString(filename), nil
}
