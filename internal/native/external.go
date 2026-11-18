package native

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func (u *UI) openExternal(target string) {
	u.submit(func() error {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", target)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
		default:
			cmd = exec.Command("xdg-open", target)
		}
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	}, nil)
}
func displayDataDir(dir string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		rel, err := filepath.Rel(home, dir)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return "~"
			}
			return "~" + string(filepath.Separator) + rel
		}
	}
	return dir
}
