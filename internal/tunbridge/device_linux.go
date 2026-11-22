package tunbridge

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/zatxm/lanboat-app/internal/tunmode"
	"golang.org/x/sys/unix"
)

const interfaceName = tunmode.Interface

func openHostTUN(ctx context.Context) (Info, *hostLease, error) {
	path := "/usr/sbin/ip"
	if _, err := os.Stat(path); err != nil {
		path = "/usr/bin/ip"
	}
	ip := func(ctx context.Context, args ...string) (string, error) {
		call, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(call, path, args...)
		cmd.Env = []string{"LC_ALL=C"}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return string(out), nil
	}
	var err error
	var undo [][]string
	var device *os.File
	var revertDNS func()
	success := false
	cleanup := func() {
		if revertDNS != nil {
			revertDNS()
		}
		for i := len(undo) - 1; i >= 0; i-- {
			_, _ = ip(context.Background(), undo[i]...)
		}
		if device != nil {
			device.Close()
		}
	}
	defer func() {
		if !success {
			cleanup()
		}
	}()
	for _, family := range []string{"-4", "-6"} {
		rules, err := ip(ctx, family, "rule", "show")
		if err != nil {
			return Info{}, nil, err
		}
		for line := range strings.SplitSeq(rules, "\n") {
			priority, _, _ := strings.Cut(line, ":")
			id, _ := strconv.Atoi(priority)
			if id >= tunmode.RuleStart && id <= tunmode.RuleEnd || strings.HasSuffix(strings.TrimSpace(line), "lookup 20245") {
				return Info{}, nil, errors.New("Lanboat TUN route priority or table already in use")
			}
		}
		routes, err := ip(ctx, family, "route", "show", "table", "20245")
		if strings.TrimSpace(routes) != "" && (err == nil || !strings.Contains(routes, "FIB table does not exist")) {
			return Info{}, nil, errors.New("Lanboat TUN routing table already in use")
		}
	}
	tunFD, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return Info{}, nil, err
	}
	device = os.NewFile(uintptr(tunFD), interfaceName)
	ifr, _ := unix.NewIfreq(interfaceName)
	ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI | unix.IFF_TUN_EXCL)
	if err = unix.IoctlIfreq(tunFD, unix.TUNSETIFF, ifr); err != nil {
		return Info{}, nil, err
	}
	commands := [][]string{
		{"link", "set", "dev", interfaceName, "mtu", "1500", "up"},
		{"-4", "address", "add", "172.29.45.1/30", "dev", interfaceName},
		{"-6", "address", "add", "fdfe:29:45::1/126", "dev", interfaceName, "nodad"},
		{"-4", "route", "add", "default", "dev", interfaceName, "table", "20245"},
		{"-6", "route", "add", "default", "dev", interfaceName, "table", "20245"},
	}
	for _, cmd := range commands {
		if _, err = ip(ctx, cmd...); err != nil {
			return Info{}, nil, err
		}
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return Info{}, nil, err
	}
	mark := uint32(0x4c420000) | uint32(iface.Index&65535)
	for _, family := range []string{"-4", "-6"} {
		policy := [][]string{
			{"priority", "9045", "fwmark", fmt.Sprintf("0x%x", mark), "goto", "9050"},
			{"priority", "9047", "iif", interfaceName, "goto", "9050"},
			{"priority", "9049", "lookup", "20245"},
			{"priority", "9050", "lookup", "main"},
		}
		if family == "-6" {
			policy = append(policy, []string{"priority", "9046", "ipproto", "58", "lookup", "main", "suppress_prefixlength", "0"})
		}
		for _, rule := range policy {
			if _, err = ip(ctx, append([]string{family, "rule", "add"}, rule...)...); err != nil {
				return Info{}, nil, err
			}
			undo = append(undo, append([]string{family, "rule", "del"}, rule...))
		}
	}
	if own, _ := os.Readlink("/proc/self/ns/net"); own != "" {
		initial, _ := os.Readlink("/proc/1/ns/net")
		if own == initial {
			revertDNS = configureLinuxDNS(ctx)
		}
	}
	success = true
	return Info{Name: interfaceName, Mark: mark}, &hostLease{file: device, cleanup: cleanup}, nil
}

func configureLinuxDNS(ctx context.Context) func() {
	const tool = "/usr/bin/resolvectl"
	revert := func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanup, tool, "revert", interfaceName).Run()
	}
	if err := exec.CommandContext(ctx, tool, "dns", interfaceName, "172.29.45.2", "fdfe:29:45::2").Run(); err != nil {
		return nil
	}
	if err := exec.CommandContext(ctx, tool, "domain", interfaceName, "~.").Run(); err != nil {
		revert()
		return nil
	}
	if err := exec.CommandContext(ctx, tool, "default-route", interfaceName, "yes").Run(); err != nil {
		revert()
		return nil
	}
	return revert
}
