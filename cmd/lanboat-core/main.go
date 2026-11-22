package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	allinone "github.com/zatxm/lanboat-allinone/v26"
	"github.com/zatxm/lanboat-app/internal/corebackend"
)

func run() error {
	if handled, err := allinone.RunTailscaleSSHWorker(os.Args[1:]); handled {
		return err
	}
	if handled, err := allinone.RunNetworkNamespaceWorker(os.Args[1:]); handled {
		return err
	}
	if len(os.Args) != 2 || os.Args[1] != "--lanboat-core" {
		return fmt.Errorf("invalid Lanboat backend invocation")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return corebackend.Run(ctx, os.Stdin, os.Stdout)
}

func main() {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(32 << 20)
	}
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(80)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Lanboat core:", err)
		os.Exit(1)
	}
}
