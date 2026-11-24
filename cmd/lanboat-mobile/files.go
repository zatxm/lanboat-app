//go:build (android || ios) && cgo

package main

/*
#include <stdlib.h>
int lbPickFile(void);
int lbSaveFile(const char *path);
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

var fileDialog struct {
	sync.Mutex
	reply chan string
}

//export LanboatFileResult
func LanboatFileResult(path *C.char) {
	fileDialog.Lock()
	defer fileDialog.Unlock()
	if fileDialog.reply != nil {
		select {
		case fileDialog.reply <- C.GoString(path):
		default:
		}
	}
}

func mobileFile(path string) (string, error) {
	reply := make(chan string, 1)
	fileDialog.Lock()
	if fileDialog.reply != nil {
		fileDialog.Unlock()
		return "", errors.New("已有文件选择器打开")
	}
	fileDialog.reply = reply
	fileDialog.Unlock()
	defer func() {
		fileDialog.Lock()
		fileDialog.reply = nil
		fileDialog.Unlock()
	}()
	if path == "" {
		if C.lbPickFile() == 0 {
			return "", errors.New("无法打开文件选择器")
		}
	} else {
		name := C.CString(path)
		ok := C.lbSaveFile(name)
		C.free(unsafe.Pointer(name))
		if ok == 0 {
			return "", errors.New("无法打开文件选择器")
		}
	}
	result := <-reply
	if result == "-" {
		return "", errors.New("无法读取或保存所选文件")
	}
	return result, nil
}
