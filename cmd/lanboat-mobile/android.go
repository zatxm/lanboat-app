//go:build android && cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
int lbAndroidStart(int64_t request);
int lbAndroidStop(int64_t request);
int lbAndroidProtect(int64_t request, int fd);
int lbAndroidOpen(const char *url);
*/
import "C"

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/zatxm/lanboat-allinone/v26/native"
	"github.com/zatxm/lanboat-app/internal/corebackend"
	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/traffic"
)

type vpnRequest struct {
	reply chan int
	done  chan struct{}
}

var androidVPN = struct {
	sync.Mutex
	id      atomic.Int64
	pending map[int64]*vpnRequest
}{pending: make(map[int64]*vpnRequest)}

//export LanboatAndroidAttached
func LanboatAndroidAttached(id C.int64_t, fd C.int) C.int {
	androidVPN.Lock()
	defer androidVPN.Unlock()
	if r := androidVPN.pending[int64(id)]; r != nil {
		select {
		case r.reply <- int(fd):
			return 1
		default:
		}
	}
	return 0
}

//export LanboatAndroidRevoked
func LanboatAndroidRevoked(id C.int64_t) {
	androidVPN.Lock()
	defer androidVPN.Unlock()
	if r := androidVPN.pending[int64(id)]; r != nil {
		select {
		case <-r.done:
		default:
			close(r.done)
		}
	}
}

type androidDevice struct {
	*os.File
	closeOnce sync.Once
	stop      func()
}

func (d *androidDevice) Close() error {
	d.closeOnce.Do(func() {
		_ = d.File.Close()
		d.stop()
	})
	return nil
}

func openAndroidTunnel(ctx context.Context) (corebackend.Tunnel, error) {
	id := androidVPN.id.Add(1)
	request := &vpnRequest{reply: make(chan int, 1), done: make(chan struct{})}
	androidVPN.Lock()
	androidVPN.pending[id] = request
	androidVPN.Unlock()
	stop := func() {
		androidVPN.Lock()
		delete(androidVPN.pending, id)
		select {
		case fd := <-request.reply:
			if fd >= 0 {
				_ = os.NewFile(uintptr(fd), "lanboat-tun").Close()
			}
		default:
		}
		androidVPN.Unlock()
		C.lbAndroidStop(C.int64_t(id))
	}
	if C.lbAndroidStart(C.int64_t(id)) == 0 {
		stop()
		return corebackend.Tunnel{}, errors.New("无法申请 Android VPN 权限")
	}
	select {
	case <-ctx.Done():
		stop()
		return corebackend.Tunnel{}, ctx.Err()
	case fd := <-request.reply:
		if fd < 0 {
			stop()
			return corebackend.Tunnel{}, errors.New("未授权 Android VPN，或系统无法建立隧道")
		}
		device := &androidDevice{File: os.NewFile(uintptr(fd), "lanboat-tun"), stop: stop}
		// A nonzero mark enables the core's socket-protection callback.
		return corebackend.Tunnel{
			Device: device, Info: native.DesktopTUNInfo{Name: "android-vpn", Mark: 1}, Done: request.done,
			Protect: func(fd uintptr) error {
				if C.lbAndroidProtect(C.int64_t(id), C.int(fd)) == 0 {
					return errors.New("无法将连接排除在 Android VPN 之外")
				}
				return nil
			},
		}, nil
	}
}

func launchVPN(ctx context.Context, req coreipc.Request, log func(traffic.Entry)) (coreipc.Session, error) {
	return coreipc.LaunchEmbedded(ctx, req, log, func(run context.Context, in io.Reader, out io.Writer) error {
		return corebackend.RunAttached(run, in, out, openAndroidTunnel)
	})
}

func openExternal(url string) error {
	text := C.CString(url)
	defer C.free(unsafe.Pointer(text))
	if C.lbAndroidOpen(text) == 0 {
		return errors.New("无法打开链接")
	}
	return nil
}
