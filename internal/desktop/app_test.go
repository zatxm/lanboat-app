package desktop

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestSubscriptionRefreshIsAtomic(t *testing.T) {
	a := testApp(t)
	content := profile.URI(profile.Node{Name: "sub", Server: "example.com", Port: 8388, Method: "aes-256-gcm", Password: "secret"})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(content)) }))
	defer server.Close()
	a.client = server.Client()
	if count, err := a.SaveSubscription("test", server.URL); err != nil || count != 1 {
		t.Fatalf("add: %d %v", count, err)
	}
	state := a.GetState()
	id := state.Config.Subscriptions[0].ID
	nodeID := state.Config.Nodes[0].ID
	if _, err := a.RefreshSubscription(id); err != nil {
		t.Fatal(err)
	}
	if a.GetState().Config.Nodes[0].ID != nodeID {
		t.Fatal("unchanged node selection lost")
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
