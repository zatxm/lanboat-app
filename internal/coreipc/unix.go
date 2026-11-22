//go:build !windows

package coreipc

import "os/exec"

func hideWindow(*exec.Cmd) {}
