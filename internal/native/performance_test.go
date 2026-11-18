package native

import (
	"fmt"
	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func benchmarkView(b *testing.B) *nativeView {
	b.Helper()
	if os.Getenv("LANBOAT_FLTK_TEST") != "1" {
		b.Skip("isolated display required")
	}
	runtime.LockOSThread()
	b.Cleanup(runtime.UnlockOSThread)
	dir := b.TempDir()
	c := profile.Default()
	c.Settings.Language = "en"
	profile.EnsureFiles(&c)
	for i := range 1000 {
		c.Nodes = append(c.Nodes, profile.Node{ID: fmt.Sprint(i), Type: "vless", Name: fmt.Sprintf("Tokyo-%04d", i), Server: "example.invalid", Port: 443, Options: map[string]any{"uuid": "12345678-1234-4234-8234-123456789012", "tls": true, "sni": "example.invalid", "network": "ws", "alpn": []any{"h2", "http/1.1"}}})
	}
	c.Selected = "0"
	if err := profile.Save(filepath.Join(dir, "config.json"), c); err != nil {
		b.Fatal(err)
	}
	backend, err := desktop.New(dir, "", zap.NewNop(), &desktop.Logs{})
	if err != nil {
		b.Fatal(err)
	}
	u := newUI(backend, requestIdle)
	v, err := newView(u)
	if err != nil {
		b.Fatal(err)
	}
	active = v
	v.render()
	pump()
	b.Cleanup(func() { active = nil; close(u.done); v.dispose(); backend.Shutdown(nil) })
	return v
}
func BenchmarkNativeRefresh1000(b *testing.B) {
	v := benchmarkView(b)
	b.ReportAllocs()
	for b.Loop() {
		v.refresh()
	}
}
func BenchmarkNativeRender1000(b *testing.B) {
	v := benchmarkView(b)
	b.ReportAllocs()
	for b.Loop() {
		v.render()
	}
}
func BenchmarkNativeSettings1000(b *testing.B) {
	v := benchmarkView(b)
	u := v.ui
	b.ReportAllocs()
	for b.Loop() {
		mode := "global"
		if u.state.Config.Settings.Mode == mode {
			mode = "rule"
		}
		u.settings(mode)
		for u.busy {
			pump()
		}
		v.flush()
	}
}
