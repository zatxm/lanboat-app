//go:build !windows

package native

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

func choosePath(ctx context.Context, title, pattern string, directory bool) (string, error) {
	cmd := fileDialogCommand(ctx, title, pattern, directory)
	out, err := cmd.Output()
	if err != nil {
		if _, cancelled := err.(*exec.ExitError); cancelled {
			return "", nil
		}
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

func fileDialogCommand(ctx context.Context, title, pattern string, directory bool) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		script := "POSIX path of (choose file)"
		if directory {
			script = "POSIX path of (choose folder)"
		}
		return exec.CommandContext(ctx, "osascript", "-e", script)
	}
	args := []string{"--file-selection", "--title", title}
	if directory {
		args = append(args, "--directory")
	} else if pattern != "" {
		args = append(args, "--file-filter", pattern)
	}
	return exec.CommandContext(ctx, "zenity", args...)
}
