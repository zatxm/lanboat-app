package appmeta

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"os"
	"path/filepath"
	"testing"
)

func TestPathsAndMigration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LANBOAT_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "old-config"))
	t.Setenv("TMPDIR", os.Getenv("TMPDIR"))
	t.Setenv("XDG_CACHE_HOME", os.Getenv("XDG_CACHE_HOME"))
	dir, err := DataDir()
	if err != nil || dir != filepath.Join(home, ".lanboat") {
		t.Fatalf("%q %v", dir, err)
	}
	if err = Prepare(dir); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"", "logs", "runtime", "cache"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("directory %s: %v %v", sub, info, err)
		}
	}
	if os.Getenv("TMPDIR") != filepath.Join(dir, "runtime") || os.Getenv("XDG_CACHE_HOME") != filepath.Join(dir, "cache") {
		t.Fatal("runtime paths escape application root")
	}
	old := filepath.Join(home, "old-config", "z-fq-l", "config.json")
	c := profile.Default()
	c.Settings.Port = 12000
	if err = profile.Save(old, c); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(old)
	if err = MigrateLegacy(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := profile.Load(filepath.Join(dir, "config.json"))
	if err != nil || loaded.Settings.Port != 12000 {
		t.Fatalf("migration: %+v %v", loaded, err)
	}
	after, _ := os.ReadFile(old)
	if string(after) != string(before) {
		t.Fatal("legacy config changed")
	}
	c.Settings.Port = 13000
	_ = profile.Save(old, c)
	if err = MigrateLegacy(dir); err != nil {
		t.Fatal(err)
	}
	loaded, _ = profile.Load(filepath.Join(dir, "config.json"))
	if loaded.Settings.Port != 12000 {
		t.Fatal("new configuration overwritten")
	}
	override := filepath.Join(home, "isolated")
	t.Setenv("LANBOAT_HOME", override)
	got, err := DataDir()
	if err != nil || got != override {
		t.Fatal(got, err)
	}
	if err = MigrateLegacy(override); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(override, "config.json")); !os.IsNotExist(err) {
		t.Fatal("isolated root imported user data")
	}
}
