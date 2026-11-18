package native

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestBackupOverwriteAndRestoreFlow(t *testing.T) {
	u := testUI(t)
	for i := 0; i < desktop.MaxBackups; i++ {
		if _, err := u.backend.CreateBackup("nodes"); err != nil {
			t.Fatal(err)
		}
	}
	u.state = u.backend.GetState()
	u.createBackup()
	if u.busy || u.confirm == nil || u.confirm.kind != "backup-overwrite" || !strings.Contains(u.confirm.prompt, "覆盖最早的备份") {
		t.Fatalf("missing overwrite warning: %+v", u.confirm)
	}
	d := u.confirm
	u.confirm = nil
	d.run()
	waitJob(t, u)
	if u.err != "" || u.notice != "" || len(u.state.Backups) != desktop.MaxBackups {
		t.Fatalf("overwrite: error=%q notice=%q backups=%d", u.err, u.notice, len(u.state.Backups))
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
		if _, err := u.backend.CreateBackup("nodes"); err != nil {
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
	clicked := false
	for _, c := range v.controls {
		if strings.HasPrefix(c.widget.text, "立即备份") {
			clicked = true
			c.widget.event("push", "")
		}
	}
	pump()
	if !clicked || u.confirm == nil || u.confirm.kind != "backup-overwrite" {
		t.Fatalf("missing overwrite warning: %v %+v", clicked, u.confirm)
	}
	label := ""
	for _, c := range v.controls {
		if c.widget.text == "确认" {
			label = c.widget.nativeText()
		}
	}
	if label == "" || strings.ContainsFunc(label, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		t.Fatal("untranslated overwrite action", label)
	}
	d := u.confirm
	u.confirm = nil
	d.run()
	wait()
	if len(u.state.Backups) != desktop.MaxBackups || u.notice != "" {
		t.Fatalf("overwrite kept %d backups", len(u.state.Backups))
	}
}
