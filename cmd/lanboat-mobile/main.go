//go:build (android || ios) && cgo

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync/atomic"

	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/corebackend"
	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/i18n"
	"github.com/zatxm/lanboat-app/internal/native"
	"go.uber.org/zap"
)

var running atomic.Bool
var home atomic.Pointer[string]

//export LanboatHome
func LanboatHome(path, zone, language *C.char) {
	i18n.SetSystemLocale(C.GoString(zone), C.GoString(language))
	s := C.GoString(path)
	home.Store(&s)
}

//export FegoStart
func FegoStart() {
	if running.CompareAndSwap(false, true) {
		go func() {
			defer running.Store(false)
			main()
		}()
	}
}

func localBackend(ctx context.Context, in io.Reader, out io.Writer) error {
	return corebackend.RunAttached(ctx, in, out, func(context.Context) (corebackend.Tunnel, error) {
		return corebackend.Tunnel{}, errors.New("需要通过系统启动 VPN")
	})
}

func main() {
	path := home.Load()
	if path == nil || !filepath.IsAbs(*path) {
		return
	}
	dir := filepath.Join(*path, "lanboat")
	if err := appmeta.Prepare(dir); err != nil {
		return
	}
	logger, logs, closeLogs := desktop.NewLogger(dir)
	defer closeLogs()
	coreipc.SetEmbeddedBackend(localBackend)
	native.SetMobileHandlers(openExternal, mobileFile)
	app, err := desktop.New(dir, "", logger, logs)
	if err != nil {
		logger.Error("open mobile profile", zap.Error(err))
		return
	}
	defer app.Shutdown(nil)
	app.UseMobileLauncher(launchVPN)
	if err := native.Run(app, logger); err != nil {
		logger.Error("start mobile UI", zap.Error(err))
	}
}
