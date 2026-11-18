package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestTUNAuthorizationCancellationAndContext(t *testing.T) {
	// A private pkexec stand-in exercises GUI authorization control flow without host routes.
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "denied", true: "application-cancelled"}[cancelled], func(t *testing.T) {
			dir := t.TempDir()
			capture := filepath.Join(dir, "args")
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LANBOAT_AUTH_TEST_CAPTURE\"\nIFS= read -r request\n"
			if cancelled {
				script += "cat >/dev/null\n"
			} else {
				script += "printf 'Authorization cancelled\\n' >&2\nexit 126\n"
			}
			for _, name := range []string{"pkexec", "lanboat-tun"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("LANBOAT_AUTH_TEST_CAPTURE", capture)
			e := &Engine{path: filepath.Join(dir, "sing-box")}
			config := profile.Default()
			ctx := context.Background()
			var cancel context.CancelFunc
			if cancelled {
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			start := time.Now()
			err := e.startTUN(ctx, config)
			if err == nil || e.tun != nil || time.Since(start) > 2*time.Second {
				t.Fatal("authorization left a session or blocked", err)
			}
			if !cancelled && !strings.Contains(err.Error(), "Authorization cancelled") {
				t.Fatal("authorization failure was not surfaced", err)
			}
			data, err := os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			if os.Geteuid() != 0 && !strings.Contains(string(data), "--disable-internal-agent\n"+FindCore()+"\n--tun-worker") {
				t.Fatal("pkexec did not receive the fixed helper path", string(data))
			}
		})
	}
}
