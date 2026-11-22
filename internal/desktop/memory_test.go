package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResidentBytesUsesVmRSS(t *testing.T) {
	status := "VmSize:\t100000 kB\nVmHWM:\t70000 kB\nVmRSS:\t   48500 kB\nRssAnon:\t18000 kB\n"
	if got := residentBytes(status); got != 48500*1024 {
		t.Fatalf("got %d bytes from %q", got, status)
	}
}

func TestMemoryFindsThreadChildrenAndGrandchildren(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "child.pid")
	started := make(chan *exec.Cmd)
	release := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		cmd := exec.Command("sh", "-c", `sleep 30 & echo $! > "$1"; wait`, "fixture", pidfile)
		if err := cmd.Start(); err != nil {
			started <- nil
			return
		}
		started <- cmd
		<-release
	}()
	cmd := <-started
	if cmd == nil {
		close(release)
		t.Fatal("cannot start fixture")
	}
	var child int
	defer func() {
		if child > 0 {
			p, _ := os.FindProcess(child)
			_ = p.Kill()
		}
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		close(release)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidfile); err == nil {
			child, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if child > 0 {
				break
			}
		}
		time.Sleep(time.Millisecond)
	}
	if child == 0 {
		t.Fatal("grandchild missing")
	}
	memory := ProcessMemory(os.Getpid())
	childMemory := ProcessMemory(cmd.Process.Pid)
	if memory.Processes < 3 || childMemory.Processes != 2 || memory.RSS < childMemory.RSS {
		t.Fatalf("missed process: parent=%+v child=%+v", memory, childMemory)
	}
}
