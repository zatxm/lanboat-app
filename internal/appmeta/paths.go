package appmeta

import (
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

const Name = "Lanboat"
const ID = "lanboat"

var commitTime = buildCommitTime()
var Version = buildVersion()

func CommitTime() time.Time { return commitTime }

func buildVersion() string {
	if commitTime.IsZero() {
		return "dev"
	}
	return commitTime.In(time.FixedZone("UTC+8", 8*60*60)).Format("06.0102.150405")
}

func buildCommitTime() time.Time {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				committed, _ := time.Parse(time.RFC3339, setting.Value)
				return committed
			}
		}
	}
	return time.Time{}
}

func DataDir() (string, error) {
	if override := os.Getenv("LANBOAT_HOME"); override != "" {
		return filepath.Abs(override)
	}
	if dir, err := installedDataDir(); dir != "" || err != nil {
		return dir, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lanboat"), nil
}
func Prepare(dir string) error {
	for _, name := range []string{"", "logs", "runtime", "cache"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(path, 0700); err != nil {
			return err
		}
		if err := os.Chmod(path, 0700); err != nil {
			return err
		}
	}
	// Child cores and library caches stay inside the same application data directory.
	for key, value := range map[string]string{"TMPDIR": filepath.Join(dir, "runtime"), "XDG_CACHE_HOME": filepath.Join(dir, "cache")} {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}
func MigrateLegacy(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if os.Getenv("LANBOAT_HOME") != "" {
		return nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	old := filepath.Join(base, "z-fq-l", "config.json")
	if _, err = os.Stat(old); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	config, err := profile.Load(old)
	if err != nil {
		return err
	}
	return profile.Save(filepath.Join(dir, "config.json"), config)
}
