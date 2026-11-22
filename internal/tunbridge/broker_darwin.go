package tunbridge

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"

	"github.com/zatxm/lanboat-allinone/v26/native"
	"golang.org/x/sys/unix"
)

func brokerExecutable(string) (string, error) {
	return trustedBroker("/Library/PrivilegedHelperTools/org.lanboat.tun")
}

func launchBroker(path, socket, uid string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return directBroker(path, socket, uid)
	}
	const script = `on run argv
do shell script ((quoted form of item 1 of argv) & " " & (quoted form of item 2 of argv) & " " & (quoted form of item 3 of argv)) with administrator privileges
end run`
	return exec.Command("/usr/bin/osascript", "-e", script, path, socket, uid)
}

func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credential *unix.Xucred
	var optionErr error
	err = raw.Control(func(fd uintptr) {
		credential, optionErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if err != nil {
		return 0, err
	}
	if optionErr != nil {
		return 0, optionErr
	}
	return credential.Uid, nil
}

func socketProtector(*unixDevice) func(uintptr) error { return nil }

func openHostTUN(ctx context.Context) (Info, *hostLease, error) {
	info, device, err := native.OpenDesktopTUN(ctx, "")
	if err != nil {
		return Info{}, nil, err
	}
	return Info{Name: info.Name, Mark: info.Mark}, &hostLease{file: native.DesktopTUNFile(device), cleanup: func() { device.Close() }}, nil
}

func serveProtection(c *net.UnixConn, mark uint32) error {
	_, err := io.Copy(io.Discard, c)
	return err
}
