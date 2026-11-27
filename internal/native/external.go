package native

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zatxm/lanboat-app/internal/i18n"
)

var mobileOpen func(string) error
var mobileFile func(string) (string, error)

func SetMobileHandlers(open func(string) error, files func(string) (string, error)) {
	mobileOpen, mobileFile = open, files
}

func (u *UI) pickFile(title, pattern string, selected func(string)) {
	page, detail, editor, node, file := u.page, u.detail, u.editor, u.draft.ID, u.fileID
	u.pathDialog(title, pattern, false, func(path string) {
		if u.page == page && u.detail == detail && u.editor == editor && u.draft.ID == node && u.fileID == file {
			selected(path)
		}
	})
}

func (u *UI) pathDialog(title, pattern string, directory bool, selected func(string)) {
	if u.fileDialogOpen {
		return
	}
	u.fileDialogOpen = true
	title = i18n.Text(u.state.Config.Settings.Language, title)
	go func() {
		var path string
		var err error
		if mobileFile != nil {
			path, err = mobileFile("")
		} else {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				select {
				case <-u.done:
					cancel()
				case <-ctx.Done():
				}
			}()
			path, err = choosePath(ctx, title, pattern, directory)
		}
		u.post(update{background: true, apply: func() {
			u.fileDialogOpen = false
			if err != nil {
				u.err = err.Error()
				return
			}
			selected(path)
		}})
	}()
}

func (u *UI) saveFile(export func(string) (string, error)) {
	if u.fileDialogOpen {
		return
	}
	if mobileFile == nil {
		u.pathDialog("选择保存目录", "", true, func(directory string) {
			if directory == "" {
				return
			}
			var saved string
			u.submit(func() error { var err error; saved, err = export(directory); return err },
				func() { u.notice = "已导出到：\n" + saved })
		})
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
