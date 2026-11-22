package desktop

import (
	"context"
	"github.com/zatxm/lanboat-app/internal/profile"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) httpTestNodeContext(ctx context.Context, node profile.Node, s profile.Settings) (int64, error) {
	node.DialProxy = &s.Proxy.Upstream
	node.TCP = &s.TCP
	timeout := s.TestSeconds()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return a.engine.ProbeHTTP(ctx, node, s.TestTarget())
}
func (a *App) ClearLogs() error {
	if err := a.logs.Clear(); err != nil {
		return err
	}
	return a.networkLogs.Clear()
}
func (a *App) pruneLogs() {
	days := a.config.Settings.LogRetention
	_ = a.networkLogs.Prune(days)
	if days <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	dir := filepath.Join(filepath.Dir(a.path), "logs")
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "app-") {
			continue
		}
		if info, err := entry.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}
