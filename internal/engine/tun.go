package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/tunmode"
)

type tunSession struct {
	cmd  *exec.Cmd
	pipe io.WriteCloser
	done chan struct{}
	err  error
}
type tunLog struct {
	mu   sync.Mutex
	text string
}

func (l *tunLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.text += string(p)
	if len(l.text) > 4096 {
		l.text = l.text[len(l.text)-4096:]
	}
	return len(p), nil
}
func (l *tunLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return strings.TrimSpace(l.text) }

func (e *Engine) startTUN(ctx context.Context, c profile.Config) error {
	if runtime.GOOS != "linux" {
		return errors.New("此桌面版本的全局+TUN模式需要Linux，请选择规则、全局或直连模式")
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("ip"); err != nil {
		return errors.New("TUN 需要安装 iproute2")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	controlPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	if !c.Settings.Proxy.Authentication {
		c.Settings.Proxy.Username = ""
		c.Settings.Proxy.Password = ""
	}
	request := tunmode.Request{DNSPort: e.tunDNSPort, UDP: c.Settings.UDP, ProxyPort: c.Settings.Port, ControlPort: controlPort, Secret: profile.ID(), ProxyUsername: c.Settings.Proxy.Username, ProxyPassword: c.Settings.Proxy.Password, OwnerPID: os.Getpid()}
	cmd := exec.Command(binary, "--tun-worker")
	if os.Geteuid() != 0 {
		pkexec, err := exec.LookPath("pkexec")
		if err != nil {
			return errors.New("TUN 需要 polkit（pkexec）以请求管理员授权")
		}
		cmd = exec.Command(pkexec, "--disable-internal-agent", binary, "--tun-worker")
	}
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		pipe.Close()
		return err
	}
	log := &tunLog{}
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		pipe.Close()
		return fmt.Errorf("启动 TUN 授权程序失败：%w", err)
	}
	session := &tunSession{cmd: cmd, pipe: pipe, done: make(chan struct{})}
	e.tun = session
	ready := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(output)
		for scanner.Scan() {
			if scanner.Text() == "LANBOAT_TUN_READY" {
				close(ready)
				break
			}
		}
		io.Copy(io.Discard, output)
	}()
	go func() { session.err = cmd.Wait(); close(session.done) }()
	if err = json.NewEncoder(pipe).Encode(request); err != nil {
		e.stopTUN()
		return err
	}
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		e.stopTUN()
		return ctx.Err()
	case <-session.done:
		message := log.String()
		e.stopTUN()
		if message != "" {
			return fmt.Errorf("TUN 启动失败或授权已取消：%s", message)
		}
		return errors.New("TUN 管理员授权已取消或组件启动失败")
	case <-time.After(90 * time.Second):
		e.stopTUN()
		return errors.New("等待 TUN 管理员授权超时")
	}
}
func (e *Engine) stopTUN() {
	if e.tun == nil {
		return
	}
	session := e.tun
	e.tun = nil
	session.pipe.Close()
	select {
	case <-session.done:
	case <-time.After(8 * time.Second):
		session.cmd.Process.Kill()
		select {
		case <-session.done:
		case <-time.After(time.Second):
		}
	}
}
