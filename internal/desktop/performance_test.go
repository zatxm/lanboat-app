package desktop

import (
	"fmt"
	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
	"os"
	"testing"
)

func benchmarkApp(b *testing.B) *App {
	b.Helper()
	a, err := New(b.TempDir(), "", zap.NewNop(), &Logs{})
	if err != nil {
		b.Fatal(err)
	}
	for i := range 1000 {
		a.config.Nodes = append(a.config.Nodes, profile.Node{ID: fmt.Sprint(i), Type: "vless", Name: "fixture", Server: "example.invalid", Port: 443, Options: map[string]any{"uuid": "12345678-1234-4234-8234-123456789012", "tls": true, "network": "ws", "sni": "example.invalid", "alpn": []any{"h2", "http/1.1"}, "reality-opts": map[string]any{"public-key": "public", "short-id": "abcd"}}})
	}
	return a
}
func BenchmarkProcessMemory(b *testing.B) {
	pid := os.Getpid()
	b.ReportAllocs()
	for b.Loop() {
		_ = ProcessMemory(pid)
	}
}
func BenchmarkGetState1000(b *testing.B) {
	a := benchmarkApp(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = a.GetState()
	}
}
func BenchmarkGetStatusIdle(b *testing.B) {
	a := benchmarkApp(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = a.GetStatus()
	}
}
func BenchmarkSaveSettings1000(b *testing.B) {
	a := benchmarkApp(b)
	s := a.config.Settings
	b.ReportAllocs()
	for b.Loop() {
		s.LogRetention = (s.LogRetention + 1) % 30
		if err := a.SaveSettings(s); err != nil {
			b.Fatal(err)
		}
	}
}
