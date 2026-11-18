package desktop

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShareUsesStoredCredentialsWithoutChangingState(t *testing.T) {
	a := testApp(t)
	if err := a.SaveNode(profile.Node{Type: "ss", Name: "fixture", Server: "example.invalid", Port: 443, Method: "aes-256-gcm", Password: "private-fixture"}); err != nil {
		t.Fatal(err)
	}
	id := a.config.Nodes[0].ID
	a.config.Subscriptions = []profile.Subscription{{ID: "sub", Name: "fixture", URL: "https://example.invalid/sub?token=fixture"}}
	before, _ := os.ReadFile(a.path)
	link, err := a.ShareLink("node", id)
	if err != nil {
		t.Fatal(err)
	}
	n, err := profile.ParseURI(link)
	if err != nil || n.Password != "private-fixture" {
		t.Fatal("masked credential exported")
	}
	link, err = a.ShareLink("subscription", "sub")
	if err != nil || link != a.config.Subscriptions[0].URL {
		t.Fatal("subscription must share original URL")
	}
	after, _ := os.ReadFile(a.path)
	if string(before) != string(after) {
		t.Fatal("share changed configuration")
	}
}
func TestExportDirectoryAndCollision(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "default.conf")
	os.WriteFile(path, []byte("keep existing"), 0600)
	saved, err := a.ExportDataTo("config", "default", dir)
	if err != nil {
		t.Fatal(err)
	}
	if saved == path || filepath.Base(saved) != "default-2.conf" {
		t.Fatal("export collision", saved)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep existing" {
		t.Fatal("overwrote existing file")
	}
	data, _ = os.ReadFile(saved)
	if string(data) != a.config.Files[0].Content {
		t.Fatal("export content changed")
	}
	info, _ := os.Stat(saved)
	if info.Mode().Perm() != 0600 {
		t.Fatal("export permissions")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.path), "exports")); !os.IsNotExist(err) {
		t.Fatal("left an intermediate export")
	}
	if _, err := a.ExportDataTo("config", "missing", dir); err == nil {
		t.Fatal("missing rule exported")
	}
	if _, err := a.ExportDataTo("config", "default", path); err == nil {
		t.Fatal("non-directory accepted")
	}
}
func TestRemovedSystemProxyIsDisabledOnLoad(t *testing.T) {
	a := testApp(t)
	c := a.config
	c.Settings.SystemProxy = true
	if err := profile.Save(a.path, c); err != nil {
		t.Fatal(err)
	}
	b, err := New(filepath.Dir(a.path), "", a.log, a.logs)
	if err != nil {
		t.Fatal(err)
	}
	if b.config.Settings.SystemProxy {
		t.Fatal("hidden legacy system proxy stayed enabled")
	}
	raw, _ := os.ReadFile(a.path)
	if strings.Contains(string(raw), `"systemProxy":true`) {
		t.Fatal("legacy switch not persisted")
	}
}
