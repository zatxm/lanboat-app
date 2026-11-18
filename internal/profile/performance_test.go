package profile

import "testing"

func performanceNode() Node {
	return Node{Type: "vless", Name: "fixture", Server: "example.invalid", Port: 443, Options: map[string]any{
		"uuid": "12345678-1234-4234-8234-123456789012", "tls": true, "network": "ws", "alpn": []any{"h2", "http/1.1"},
		"reality-opts": map[string]any{"public-key": "public", "short-id": "abcd"}, "ws-opts": map[string]any{"headers": map[string]any{"Authorization": "private"}},
	}}
}
func BenchmarkNodeClone(b *testing.B) {
	n := performanceNode()
	b.ReportAllocs()
	for b.Loop() {
		_ = n.Clone()
	}
}
func BenchmarkNodeMasked(b *testing.B) {
	n := performanceNode()
	b.ReportAllocs()
	for b.Loop() {
		_ = n.Masked()
	}
}
