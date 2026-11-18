package desktop

import (
	"context"
	"fmt"
	"github.com/zatxm/lanboat-app/internal/engine"
	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	dir, err := os.MkdirTemp(filepath.Join(filepath.Dir(a.path), "runtime"), ".test-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)
	temporary := engine.New(engine.FindCore(), dir, zap.NewNop())
	transport := &http.Transport{DisableKeepAlives: true}
	if node.Kind() == "tailscale" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		c := profile.Default()
		c.Settings.Port, c.Settings.Mode = port, "global"
		c.Nodes, c.Selected = []profile.Node{node}, node.ID
		if err = temporary.StartContext(ctx, c); err != nil {
			return 0, err
		}
		defer temporary.Stop()
		proxyURL, _ := url.Parse("socks5://127.0.0.1:" + strconv.Itoa(port))
		transport.Proxy = http.ProxyURL(proxyURL)
	} else {
		probe, err := temporary.ProbeClient(ctx, node)
		if err != nil {
			return 0, err
		}
		defer probe.Close()
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return probe.DialContext(ctx, address)
		}
	}

	target := s.TestTarget()

	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Duration(timeout) * time.Second}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("通过节点进行HTTP连通性测试失败：%w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return 0, fmt.Errorf("连通性测试返回HTTP状态%d", resp.StatusCode)
	}
	return max(1, time.Since(start).Milliseconds()), nil
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
