package native

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestBackupLimitAndRestoreFlow(t *testing.T) {
	u := testUI(t)
	for i := 0; i < desktop.MaxBackups; i++ {
		if _, err := u.backend.CreateBackup(); err != nil {
			t.Fatal(err)
		}
	}
	u.state = u.backend.GetState()
	u.createBackup()
	if u.busy || u.confirm != nil || len(u.backend.Backups()) != desktop.MaxBackups {
		t.Fatal("a full backup list accepted another backup")
	}
	id := u.state.Backups[0].ID
	u.restoreBackup(id)
	waitJob(t, u)
	if u.err != "" || u.notice != "备份已还原" || len(u.state.Backups) != desktop.MaxBackups {
		t.Fatalf("restore: error=%q notice=%q backups=%d", u.err, u.notice, len(u.state.Backups))
	}
	u.notice = "previous message"
	u.deleteBackup(id)
	waitJob(t, u)
	if u.err != "" || u.notice != "" || len(u.state.Backups) != desktop.MaxBackups-1 {
		t.Fatalf("delete: error=%q notice=%q backups=%d", u.err, u.notice, len(u.state.Backups))
	}
}

func TestNativeWindowBackups(t *testing.T) {
	if os.Getenv("LANBOAT_FLTK_TEST") != "1" {
		t.Skip("isolated display required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("TZ", "Asia/Shanghai")
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for u.busy && time.Now().Before(deadline) {
			pump()
			time.Sleep(time.Millisecond)
		}
		pump()
		if u.busy || u.err != "" {
			t.Fatalf("job: busy=%v error=%s", u.busy, u.err)
		}
	}
	for i := 0; i < desktop.MaxBackups; i++ {
		if _, err := u.backend.CreateBackup(); err != nil {
			t.Fatal(err)
		}
	}
	u.state = u.backend.GetState()
	u.navigate("settings")
	u.openBackups()
	v.render()
	pump()
	for _, text := range []string{"还原备份", "下载备份", "删除备份"} {
		found := false
		for _, w := range v.widgets {
			if w.text == text {
				found = true
			}
		}
		if !found {
			t.Fatal("missing backup action", text)
		}
	}
	u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
	wait()
	assertEnglishWidgets(t, v)
	for _, c := range v.controls {
		button := c.widget
		if button.text == "立即备份" {
			if button.enabled || button.tooltip != backupLimitHint {
				t.Fatal("full backup list lacks a disabled add button and hint")
			}
			return
		}
	}
	t.Fatal("add backup button missing")
}
