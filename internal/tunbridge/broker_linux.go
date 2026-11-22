package tunbridge

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func brokerExecutable(executable string) (string, error) {
	// A root caller already controls its command namespace. This also serves isolated integration tests.
	if os.Geteuid() == 0 {
		neighbor := filepath.Join(filepath.Dir(executable), "lanboat-tun-worker")
		if st, err := os.Stat(neighbor); err == nil && st.Mode().IsRegular() {
			return neighbor, nil
		}
	}
	return trustedBroker("/usr/libexec/lanboat-tun-worker")
}

func launchBroker(path, socket, uid string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return directBroker(path, socket, uid)
	}
	return exec.Command("/usr/bin/pkexec", "--disable-internal-agent", path, socket, uid)
}

func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credential *unix.Ucred
	var optionErr error
	err = raw.Control(func(fd uintptr) {
		credential, optionErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return 0, err
	}
	if optionErr != nil {
		return 0, optionErr
	}
	return credential.Uid, nil
}

func socketProtector(d *unixDevice) func(uintptr) error {
	c := d.control
	return func(fd uintptr) error {
		d.mutex.Lock()
		defer d.mutex.Unlock()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		if err := sendFD(c, fd); err != nil {
			c.Close()
			return err
		}
		var b [4]byte
		if _, err := io.ReadFull(c, b[:]); err != nil {
			c.Close()
			return err
		}
		if errno := binary.BigEndian.Uint32(b[:]); errno != 0 {
			return unix.Errno(errno)
		}
		c.SetDeadline(time.Time{})
		return nil
	}
}

func serveProtection(c *net.UnixConn, mark uint32) error {
	for {
		fd, err := recvFD(c)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		domain, e := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_DOMAIN)
		errno := unix.EINVAL
		if e == nil && (domain == unix.AF_INET || domain == unix.AF_INET6) {
			errno = 0
			if e := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_MARK, int(mark)); e != nil {
				errno = e.(unix.Errno)
			}
		}
		unix.Close(fd)
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(errno))
		if _, err = c.Write(b[:]); err != nil {
			return err
		}
	}
}
