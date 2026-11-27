package native

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDesktopFilePickerKeepsUIResponsive(t *testing.T) {
	dir := t.TempDir()
	selected := filepath.Join(dir, "fixture.json")
	release := filepath.Join(dir, "release")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LANBOAT_PICK_FIXTURE", selected)
	t.Setenv("LANBOAT_PICK_RELEASE", release)
	if err := os.WriteFile(filepath.Join(dir, "zenity"), []byte(`#!/bin/sh
for _ in $(seq 100); do
  if [ -f "$LANBOAT_PICK_RELEASE" ]; then
    printf '%s\n' "$LANBOAT_PICK_FIXTURE"
    exit 0
  fi
  sleep 0.02
done
exit 1
`), 0700); err != nil {
		t.Fatal(err)
	}

	u := testUI(t)
	u.openBackups()
	got := ""
	u.pickFile("导入备份", "*", func(path string) { got = path })
	if !u.fileDialogOpen || got != "" {
		t.Fatal("file picker blocked the UI or completed before the chooser")
	}
	u.navigate("home")
	if u.detail != "" {
		t.Fatal("navigation did not work with the file chooser open")
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	wait := func() {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); u.fileDialogOpen && time.Now().Before(deadline); {
			u.drain()
			time.Sleep(time.Millisecond)
		}
		if u.fileDialogOpen || u.err != "" {
			t.Fatalf("chooser failed to finish: %q", u.err)
		}
	}
	wait()
	if got != "" {
		t.Fatal("a chooser applied its result after leaving the backup page")
	}

	u.openBackups()
	u.pickFile("导入备份", "*", func(path string) { got = path })
	wait()
	if got != selected {
		t.Fatal("chooser did not deliver its result to the active page")
	}
}
