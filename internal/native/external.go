package native

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var mobileOpen func(string) error
var mobileFile func(string) (string, error)

func SetMobileHandlers(open func(string) error, files func(string) (string, error)) {
	mobileOpen, mobileFile = open, files
}

func (u *UI) pickFile(title, pattern string, selected func(string)) {
	if mobileFile == nil {
		selected(choosePath(title, pattern, false))
		return
	}
	go func() {
		path, err := mobileFile("")
		u.post(update{background: true, apply: func() {
			if err != nil {
				u.err = err.Error()
				u.notify()
				return
			}
			selected(path)
		}})
	}()
}

func (u *UI) saveFile(export func(string) (string, error)) {
	if mobileFile == nil {
		directory := choosePath("选择保存目录", "", true)
		if directory == "" {
			return
		}
		var saved string
		u.submit(func() error { var err error; saved, err = export(directory); return err },
			func() { u.notice = "已导出到：\n" + saved })
		return
	}
	u.submit(func() error {
		directory, err := os.MkdirTemp("", "lanboat-export-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(directory)
		saved, err := export(directory)
		if err != nil {
			return err
		}
		_, err = mobileFile(saved)
		return err
	}, nil)
}

func (u *UI) openExternal(target string) {
	u.submit(func() error {
		if mobileOpen != nil {
			return mobileOpen(target)
		}
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
