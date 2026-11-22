package main

import (
	"context"
	"fmt"
	"github.com/goccy/go-json"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/dnscheck"
	"github.com/zatxm/lanboat-app/internal/engine"
	"github.com/zatxm/lanboat-app/internal/native"
)

func main() {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(64 << 20)
	}
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(80)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Lanboat:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(appmeta.Version)
		return nil
	}
	if len(os.Args) == 2 && os.Args[1] == "--dns-probe" {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		return json.NewEncoder(os.Stdout).Encode(dnscheck.TUN(ctx))
	}
	dir, err := appmeta.DataDir()
	if err != nil {
		return err
	}
	if err = appmeta.Prepare(dir); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "app.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := lockInstance(lock); err != nil {
		return fmt.Errorf("锁定应用数据目录失败: %w", err)
	}
	if err := appmeta.MigrateLegacy(dir); err != nil {
		return err
	}
	logger, logs, closeLog := desktop.NewLogger(dir)
	defer closeLog()
	app, err := desktop.New(dir, engine.FindCore(), logger, logs)
	if err != nil {
		return err
	}
	defer app.Shutdown(nil)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	go func() { <-signals; app.Shutdown(nil); _ = closeLog(); os.Exit(0) }()
	logger.Info("application started")
	return native.Run(app, logger)
}
