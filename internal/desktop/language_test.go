package desktop

import (
	"github.com/goccy/go-json"
	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLanguageInitializationAndPersistence(t *testing.T) {
	t.Setenv("TZ", "Asia/Hong_Kong")
	dir := t.TempDir()
	a, err := New(dir, "", zap.NewNop(), &Logs{})
	if err != nil {
		t.Fatal(err)
	}
	if a.GetState().Config.Settings.Language != "zh" {
		t.Fatal("wrong first-run language")
	}
	settings := a.GetState().Config.Settings
	settings.Language = "en"
	if err := a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	a.Shutdown(nil)
	b, err := New(dir, "", zap.NewNop(), &Logs{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Shutdown(nil)
	if b.GetState().Config.Settings.Language != "en" {
		t.Fatal("manual language overwritten at restart")
	}
	// Upgrading a config without a language must keep edited rules and credentials.
	c, err := profile.Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	c.Settings.Language = ""
	c.Files = append(c.Files, profile.ConfigFile{ID: "custom", Name: "custom.conf", Content: "[Rule]\nFINAL,DIRECT\n# user's unchanged comment\n"})
	if err := profile.Save(filepath.Join(dir, "config.json"), c); err != nil {
		t.Fatal(err)
	}
	before := c.Files[1]
	b.Shutdown(nil)
	d, err := New(dir, "", zap.NewNop(), &Logs{})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Shutdown(nil)
	after := d.GetState().Config
	if after.Settings.Language != "zh" || after.Files[1] != before {
		t.Fatal("language upgrade changed configuration content")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "config.json")); err != nil || len(data) == 0 {
		t.Fatal("language was not persisted")
	}
}

func TestPreferencesWhileConnectedKeepCoreAndConfigMirrors(t *testing.T) {
	a := autoSelectApp(t)
	a.config.Settings.Mode = "direct"
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	before := a.GetStatus()
	path := filepath.Join(filepath.Dir(a.path), "configs", "default.conf")
	timestamp := time.Unix(1000, 0)
	if err := os.Chtimes(path, timestamp, timestamp); err != nil {
		t.Fatal(err)
	}
	settings := a.GetState().Config.Settings
	settings.Language = "en"
	settings.Appearance = "dark"
	if err := a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	after := a.GetStatus()
	if !after.Connected || before.StartedAt != after.StartedAt {
		t.Fatal("UI preference restarted connection")
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(timestamp) {
		t.Fatal("preference rewrote unchanged config mirrors")
	}
	settings.Port++
	if err := a.SaveSettings(settings); err != nil {
		t.Fatal("connected port change failed", err)
	}
	settings.Port--
	settings.Language = "invalid"
	if err := a.SaveSettings(settings); err == nil {
		t.Fatal("invalid language was saved")
	}
}

func TestLegacyBackupKeepsLanguagePreference(t *testing.T) {
	a := testApp(t)
	settings := a.GetState().Config.Settings
	settings.Language = "en"
	if err := a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	c := a.GetState().Config
	c.Settings.Language = ""
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ImportBackup(string(data)); err != nil {
		t.Fatal(err)
	}
	if a.GetState().Config.Settings.Language != "en" {
		t.Fatal("legacy backup discarded language preference")
	}
	settings.Language = ""
	if err := a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if a.GetState().Config.Settings.Language != "en" {
		t.Fatal("older settings caller discarded language")
	}
}
