package desktop

import (
	"github.com/goccy/go-json"
	"github.com/zatxm/lanboat-app/internal/profile"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCurrentDefaultReplacesLegacyOnUpgradeAndRestore(t *testing.T) {
	a := testApp(t)
	if err := a.SaveNode(profile.Node{Name: "keep", Server: "example.com", Port: 443, Method: "aes-256-gcm", Password: "keep-private"}); err != nil {
		t.Fatal(err)
	}
	if err := a.CopyConfigFile("default"); err != nil {
		t.Fatal(err)
	}
	c, err := profile.Load(a.path)
	if err != nil {
		t.Fatal(err)
	}
	custom := c.Files[1]
	c.ActiveFile = custom.ID
	c.Files[0].Content = "[Rule]\nDOMAIN,legacy-only.invalid,DIRECT\nFINAL,PROXY"
	legacy, _ := json.Marshal(c)
	if err := profile.Save(a.path, c); err != nil {
		t.Fatal(err)
	}
	b, err := New(filepath.Dir(a.path), "", a.log, a.logs)
	if err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		stored, err := profile.Load(a.path)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Files[0].Content != profile.DefaultFile().Content || strings.Contains(stored.Files[0].Content, "legacy-only.invalid") {
			t.Fatal("legacy default survived")
		}
		if stored.Files[1] != custom || stored.ActiveFile != custom.ID || stored.Nodes[0].Password != "keep-private" || !reflect.DeepEqual(stored.Settings, c.Settings) {
			t.Fatal("default upgrade changed user data")
		}
	}
	check()
	if err := b.ImportBackup(string(legacy)); err != nil {
		t.Fatal(err)
	}
	check()
}

func TestDefaultReadOnlyAndIndependentCopies(t *testing.T) {
	a := testApp(t)
	original := a.GetState().Config.Files[0]
	before, _ := os.ReadFile(a.path)
	if err := a.SaveConfigFile("default", "default.conf", "[Rule]\nFINAL,DIRECT"); err == nil {
		t.Fatal("default edit accepted")
	}
	after, _ := os.ReadFile(a.path)
	if string(before) != string(after) {
		t.Fatal("rejected edit changed storage")
	}
	for range 2 {
		if err := a.CopyConfigFile("default"); err != nil {
			t.Fatal(err)
		}
	}
	c := a.GetState().Config
	if c.Files[1].Name == c.Files[2].Name || c.Files[1].Content != original.Content || c.ActiveFile != "default" {
		t.Fatal("copy changed original or active file")
	}
	copy := c.Files[1]
	if err := a.SaveConfigFile(copy.ID, copy.Name, "[Rule]\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	c = a.GetState().Config
	if c.Files[0] != original || c.Files[2].Content != original.Content || c.Files[1].Content == original.Content {
		t.Fatal("copies are not independent")
	}
}

func TestConfigFilesAndBackup(t *testing.T) {
	a := testApp(t)
	if err := a.SaveConfigFile("", "custom", "[Rule]\nDOMAIN,example.com,REJECT\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	state := a.GetState()
	f := state.Config.Files[1]
	if err := a.ActivateConfigFile(f.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := a.TestRule("example.com", ""); err != nil || !strings.HasPrefix(got, "REJECT") {
		t.Fatal(got, err)
	}
	if err := a.SaveConfigFile("", "custom.conf", "[Rule]\nFINAL,PROXY"); err == nil {
		t.Fatal("duplicate file name accepted")
	}
	if err := a.SaveConfigFile("", "../escape", "[Rule]\nFINAL,PROXY"); err == nil {
		t.Fatal("unsafe file accepted")
	}
	record, err := a.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	backup, err := a.ExportBackupTo(record.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(backup)
	if info.Mode().Perm() != 0600 {
		t.Fatal("backup permissions")
	}
	if err = a.ActivateConfigFile("default"); err != nil {
		t.Fatal(err)
	}
	if err = a.ImportBackup(backup); err != nil {
		t.Fatal(err)
	}
	if a.GetState().Config.ActiveFile != f.ID {
		t.Fatal("backup lost active file")
	}
	if err = a.DeleteConfigFile(f.ID); err == nil {
		t.Fatal("deleted active file")
	}
	if err = a.ActivateConfigFile("default"); err != nil {
		t.Fatal(err)
	}
	if err = a.DeleteConfigFile(f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = profile.Load(a.path); err != nil {
		t.Fatal(err)
	}
}
func TestBackupOverOneMiB(t *testing.T) {
	a := testApp(t)
	content := "[Rule]\n" + strings.Repeat("# padding for real backup\n", 25000) + "FINAL,PROXY"
	for _, name := range []string{"one", "two"} {
		if err := a.SaveConfigFile("", name, content); err != nil {
			t.Fatal(err)
		}
	}
	record, err := a.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	path, err := a.ExportBackupTo(record.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Size() < 1<<20 {
		t.Fatal("fixture too small")
	}
	if err = a.ImportBackup(path); err != nil {
		t.Fatal(err)
	}
	if err = a.ImportBackup(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing backup accepted")
	}
}
