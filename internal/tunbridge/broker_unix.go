//go:build linux || darwin

package tunbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type unixDevice struct {
	io.ReadWriteCloser
	control *net.UnixConn
	once    sync.Once
	mutex   sync.Mutex
}

func (d *unixDevice) Close() error {
	d.once.Do(func() {
		d.ReadWriteCloser.Close()
		d.mutex.Lock()
		defer d.mutex.Unlock()
		d.control.SetDeadline(time.Now().Add(6 * time.Second))
		_ = d.control.CloseWrite()
		_, _ = io.Copy(io.Discard, d.control)
		d.control.Close()
	})
	return nil
}

func Open(ctx context.Context, executable string) (*Device, error) {
	path, err := brokerExecutable(executable)
	if err != nil {
		return nil, err
	}
	temp, err := os.MkdirTemp("/tmp", "lanboat-tun-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temp)
	socket := filepath.Join(temp, "broker")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	cmd := launchBroker(path, socket, strconv.Itoa(os.Getuid()))
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_ = cmd.Wait()
		listener.Close()
	}()
	c, err := listener.AcceptUnix()
	if err != nil {
		return nil, fmt.Errorf("TUN authorization failed: %w", err)
	}
	success := false
	defer func() {
		if !success {
			c.Close()
		}
	}()
	uid, err := peerUID(c)
	if err != nil || uid != 0 {
		return nil, errors.New("TUN broker did not authenticate as administrator")
	}
	deadline, _ := ctx.Deadline()
	c.SetDeadline(deadline)
	stopConnection := context.AfterFunc(ctx, func() { c.Close() })
	defer stopConnection()
	info, err := receiveInfo(c)
	if err != nil {
		return nil, err
	}
	fd, err := recvFD(c)
	if err != nil {
		return nil, err
	}
	if err = unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	file := os.NewFile(uintptr(fd), "lanboat-tun-device")
	c.SetDeadline(time.Time{})
	dev := &unixDevice{ReadWriteCloser: file, control: c}
	success = true
	return &Device{ReadWriteCloser: dev, Info: info, Protect: socketProtector(dev), Done: exited}, nil
}

func Run(ctx context.Context, args []string) error {
	if os.Geteuid() != 0 || len(args) != 2 {
		return errors.New("TUN broker requires administrator authorization")
	}
	want, err := parseUID(args[1])
	if err != nil {
		return err
	}
	if value := os.Getenv("PKEXEC_UID"); value != "" && value != args[1] {
		return errors.New("TUN broker owner does not match authorization")
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", args[0])
	if err != nil {
		return err
	}
	c := conn.(*net.UnixConn)
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	uid, err := peerUID(c)
	if err != nil || uid != want {
		return errors.New("TUN broker caller UID mismatch")
	}
	c.SetWriteDeadline(time.Now().Add(15 * time.Second))
	info, lease, err := openHostTUN(ctx)
	if err != nil {
		_ = sendInfo(c, info, err)
		return err
	}
	defer lease.Close()
	if err = sendInfo(c, info, nil); err != nil {
		return err
	}
	if err = sendFD(c, lease.file.Fd()); err != nil {
		return err
	}
	c.SetWriteDeadline(time.Time{})
	return serveProtection(c, info.Mark)
}

func sendFD(c *net.UnixConn, fd uintptr) error {
	_, _, err := c.WriteMsgUnix([]byte{1}, unix.UnixRights(int(fd)), nil)
	return err
}

func recvFD(c *net.UnixConn) (int, error) {
	var data [1]byte
	var ancillary [64]byte
	n, size, flags, _, err := c.ReadMsgUnix(data[:], ancillary[:])
	if err != nil {
		return -1, err
	}
	messages, err := unix.ParseSocketControlMessage(ancillary[:size])
	var fds []int
	for _, m := range messages {
		found, e := unix.ParseUnixRights(&m)
		if e == nil {
			fds = append(fds, found...)
		}
	}
	if err != nil || n != 1 || data[0] != 1 || flags&unix.MSG_CTRUNC != 0 || len(fds) != 1 {
		for _, fd := range fds {
			unix.Close(fd)
		}
		return -1, errors.New("invalid TUN broker descriptor")
	}
	unix.CloseOnExec(fds[0])
	return fds[0], nil
}

func trustedBroker(path string) (string, error) {
	for file := path; file != "/"; file = filepath.Dir(file) {
		var st unix.Stat_t
		err := unix.Lstat(file, &st)
		kind := uint32(unix.S_IFDIR)
		if file == path {
			kind = unix.S_IFREG
		}
		if err != nil || st.Uid != 0 || uint32(st.Mode)&unix.S_IFMT != kind || st.Mode&0o022 != 0 {
			return "", fmt.Errorf("install Lanboat to use its trusted TUN broker (%s)", path)
		}
	}
	return path, nil
}

func directBroker(path, socket, uid string) *exec.Cmd {
	return exec.Command(path, socket, uid)
}
