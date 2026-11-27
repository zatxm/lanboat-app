package desktop

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
)

func testApp(t *testing.T) *App {
	t.Helper()
	app, err := New(t.TempDir(), "", zap.NewNop(), &Logs{})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestNodeSecretsAndPersistence(t *testing.T) {
	a := testApp(t)
	n := profile.Node{Name: "test", Server: "example.com", Port: 8388, Method: "aes-256-gcm", Password: "private-secret"}
	if err := a.SaveNode(n); err != nil {
		t.Fatal(err)
	}
	state := a.GetState()
	if state.Config.Nodes[0].Password != "" {
		t.Fatal("password exposed in state")
	}
	n = state.Config.Nodes[0]
	n.Name = "renamed"
	if err := a.SaveNode(n); err != nil {
		t.Fatal(err)
	}
	uri, err := a.ExportNode(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := profile.ParseURI(uri)
	if err != nil || parsed.Password != "private-secret" {
		t.Fatal("edit lost original password")
	}
	stored, err := profile.Load(a.path)
	if err != nil || stored.Nodes[0].Name != "renamed" {
		t.Fatal("not persisted")
	}
	if err := a.DeleteNode(n.ID); err != nil {
		t.Fatal(err)
	}
	if a.GetState().Config.Selected != "" {
		t.Fatal("selection not cleared")
	}
}

func TestCopyNodePreservesPrivateConfiguration(t *testing.T) {
	a := testApp(t)
	original := profile.Node{
		Name: strings.Repeat("节", 53), Server: "example.com", Port: 8388,
		Method: "aes-256-gcm", Password: "fixture",
		Options: map[string]any{"opaque": map[string]any{"value": "fixture"}},
	}
	if err := a.SaveNode(original); err != nil {
		t.Fatal(err)
	}
	id := a.GetState().Config.Selected
	if err := a.CopyNode(id); err != nil {
		t.Fatal(err)
	}
	cfg, err := profile.Load(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Nodes) != 2 || cfg.Selected != id {
		t.Fatal("copy was not saved or changed the selection")
	}
	want, got := cfg.Nodes[0], cfg.Nodes[1]
	if got.ID == "" || got.ID == id || got.Source != "" || got.Name != strings.Repeat("节", 52)+"-1" {
		t.Fatal("copy must have a new ID and a valid suffixed name")
	}
	got.ID, got.Name = want.ID, want.Name
	if !reflect.DeepEqual(got, want) {
		t.Fatal("copy lost stored node configuration")
	}
}

func TestSubscriptionRefreshIsAtomic(t *testing.T) {
	a := testApp(t)
	content := profile.URI(profile.Node{Type: "socks5", Name: "sub", Server: "example.com", Port: 1080})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(content)) }))
	defer server.Close()
	a.client = server.Client()
	if count, err := a.SaveSubscription("test", server.URL); err != nil || count != 1 {
		t.Fatalf("add: %d %v", count, err)
	}
	state := a.GetState()
	id := state.Config.Subscriptions[0].ID
	nodeID := state.Config.Nodes[0].ID
	shareLink, err := a.ExportNode(nodeID)
	if err != nil {
		t.Fatal(err)
	}
	node := state.Config.Nodes[0]
	node.Flag = "🇯🇵"
	if err := a.SaveNode(node); err != nil {
		t.Fatal(err)
	}
	saved, err := profile.Load(a.path)
	if err != nil || saved.Nodes[0].Flag != "🇯🇵" || saved.Nodes[0].Name != "sub" {
		t.Fatal("flag update did not persist separately from the node name", err)
	}
	if uri, err := a.ExportNode(nodeID); err != nil || uri != shareLink {
		t.Fatal("flag update changed the share link", err)
	}
	if _, err := a.RefreshSubscription(id); err != nil {
		t.Fatal(err)
	}
	if node := a.GetState().Config.Nodes[0]; node.ID != nodeID || node.Flag != "🇯🇵" {
		t.Fatal("refresh lost the existing node or its flag")
	}
	content = "invalid subscription"
	if _, err := a.RefreshSubscription(id); err == nil {
		t.Fatal("accepted invalid subscription")
	}
	if a.GetState().Config.Selected != nodeID || len(a.GetState().Config.Nodes) != 1 {
		t.Fatal("failed refresh destroyed config")
	}
	content = strings.Repeat("x", profile.MaxSubscription+1)
	if _, err := a.RefreshSubscription(id); err == nil {
		t.Fatal("accepted oversized response")
	}
	if err := a.DeleteSubscription(id); err != nil {
		t.Fatal(err)
	}
	if len(a.GetState().Config.Nodes) != 0 {
		t.Fatal("subscription nodes retained")
	}
}

func TestMixedBase64SubscriptionSecrets(t *testing.T) {
	a := testApp(t)
	content := "vless://12345678-1234-4234-8234-123456789012@example.com:443?security=tls&type=ws&path=%2Fproxy#VLESS\nhy2://secret@example.com:443#HY2"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(base64.RawURLEncoding.EncodeToString([]byte(content))))
	}))
	defer server.Close()
	a.client = server.Client()
	count, err := a.SaveSubscription("mixed", server.URL)
	if err != nil || count != 2 {
		t.Fatal(count, err)
	}
	state := a.GetState()
	n := state.Config.Nodes[0]
	if n.TextOption("uuid") != "" {
		t.Fatal("credential leaked")
	}
	n.Name = "edited"
	if err = a.SaveNode(n); err != nil {
		t.Fatal(err)
	}
	shared, _ := a.ExportNode(n.ID)
	full, err := profile.ParseURI(shared)
	if err != nil || full.TextOption("uuid") != "12345678-1234-4234-8234-123456789012" {
		t.Fatal("masked edit lost secret")
	}
	content = "invalid"
	if _, err = a.RefreshSubscription(state.Config.Subscriptions[0].ID); err == nil {
		t.Fatal("bad subscription accepted")
	}
	if len(a.GetState().Config.Nodes) != 2 {
		t.Fatal("refresh lost data")
	}
}

func TestMemoryIncludesChildren(t *testing.T) {
	m := ProcessMemory(0)
	if m.RSS != 0 {
		t.Fatal("invented process tree")
	}
	m = ProcessMemory(1)
	if m.RSS <= 0 || m.Processes < 1 {
		t.Fatal("could not read process memory")
	}
}

func TestLogRingBounded(t *testing.T) {
	logger, logs, closeLog := NewLogger(t.TempDir())
	defer closeLog()
	for i := 0; i < 240; i++ {
		logger.Info("event")
	}
	if len(logs.List()) != 200 {
		t.Fatal("unbounded log ring")
	}
}

func TestLogEntriesKeepFields(t *testing.T) {
	logger, logs, closeLog := NewLogger(t.TempDir())
	defer closeLog()
	logger.Warn("rule data update failed", zap.Error(errors.New("timeout")), zap.Int("count", 2))
	entries := logs.List()
	if len(entries) != 1 || entries[0].Level != "warn" || entries[0].Message != "rule data update failed count=2 error=timeout" || entries[0].Time == "" {
		t.Fatal(entries)
	}
}

func TestDiskLogRotationAndClear(t *testing.T) {
	dir := t.TempDir()
	logger, logs, closeLog := NewLogger(dir)
	defer closeLog()
	logger.Info("before rotation")
	payload := strings.Repeat("x", 64<<10)
	for range 34 {
		logger.Info(payload)
	}
	logger.Info("after rotation")
	if err := logger.Sync(); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(dir, "logs")
	backups, err := filepath.Glob(filepath.Join(logDir, "app-*.log"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one rotated log, got %v: %v", backups, err)
	}
	active := filepath.Join(logDir, "app.log")
	data, err := os.ReadFile(active)
	if err != nil || !strings.Contains(string(data), "after rotation") || strings.Contains(string(data), "before rotation") {
		t.Fatalf("active file should contain the entries after rotation: %v", err)
	}
	if err := logs.Clear(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "app.log" || len(logs.List()) != 0 {
		t.Fatalf("clearing logs should leave only app.log and empty the in-memory list: %v (%v)", entries, err)
	}
	file, err := os.Stat(active)
	if err != nil || file.Size() != 0 {
		t.Fatalf("clearing logs should empty app.log: %v", err)
	}
}

func TestNodeFileImportUsesSubscriptionSizeLimit(t *testing.T) {
	a := testApp(t)
	nodes := make([]profile.Node, 1000)
	for i := range nodes {
		nodes[i] = profile.Node{Type: "vless", Name: fmt.Sprintf("fixture-%d", i), Server: "example.invalid", Port: 443, Options: map[string]any{"uuid": "12345678-1234-4234-8234-123456789012", "network": "ws", "transport-path": "/" + strings.Repeat("x", 1600)}}
	}
	data, err := json.Marshal(map[string]any{"nodes": nodes})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) <= 1<<20 || len(data) > profile.MaxImport {
		t.Fatal("fixture must exercise the 1–2 MiB range")
	}
	path := filepath.Join(t.TempDir(), "nodes.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if count, err := a.Import(path); err != nil || count != 1000 {
		t.Fatal(count, err)
	}
	if len(a.GetState().Config.Nodes) != 1000 {
		t.Fatal("file import lost nodes")
	}
}

func TestShadowTLSLegacyPluginSecretSurvivesFormEdit(t *testing.T) {
	a := testApp(t)
	n := profile.Node{Type: "ss", Name: "Legacy", Server: "example.com", Port: 8388, Method: profile.DefaultSSMethod, Password: "legacy-secret", Plugin: "shadow-tls"}
	if err := a.SaveNode(n); err != nil {
		t.Fatal(err)
	}
	masked := a.GetState().Config.Nodes[0]
	profile.NormalizeSS(&masked)
	masked.Name = "Edited"
	if err := a.SaveNode(masked); err != nil {
		t.Fatal(err)
	}
	if a.config.Nodes[0].TextOption("plugin-opts.password") != "legacy-secret" {
		t.Fatal("editing legacy implicit plugin password lost the secret")
	}
	n.Name = "New optional plugin password"
	n.SetOption("plugin-opts.server", "shadow.example")
	n.SetOption("plugin-opts.port", 443)
	n.SetOption("plugin-opts.password", "")
	if err := a.SaveNode(n); err != nil {
		t.Fatal(err)
	}
	if a.config.Nodes[1].TextOption("plugin-opts.password") != "" {
		t.Fatal("new explicit empty plugin password inherited SS password")
	}
}
