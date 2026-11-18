//go:build linux

// The privileged worker owns only the TUN session; EOF from the GUI closes it.
package tunworker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	allinone "github.com/zatxm/lanboat-allinone"
	"github.com/zatxm/lanboat-app/internal/tunmode"
	"golang.org/x/sys/unix"
)

func Run() error {
	if os.Geteuid() != 0 {
		return errors.New("TUN requires administrator authentication")
	}
	reader := bufio.NewReaderSize(os.Stdin, 128<<10)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return errors.New("invalid TUN session request")
	}
	var request tunmode.Request
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid trailing TUN request data")
	}
	if err = request.Validate(); err != nil {
		return err
	}
	owner, err := os.Stat("/proc/" + strconv.Itoa(request.OwnerPID))
	if err != nil {
		return err
	}
	if uid := os.Getenv("PKEXEC_UID"); uid != "" && uid != strconv.FormatUint(uint64(owner.Sys().(*syscall.Stat_t).Uid), 10) {
		return errors.New("TUN owner does not match authenticated user")
	}
	ownerPath, err := os.Readlink("/proc/" + strconv.Itoa(request.OwnerPID) + "/exe")
	if err != nil {
		return err
	}
	ns, err := os.Stat("/proc/self/ns/net")
	if err != nil {
		return err
	}
	lockPath := filepath.Join(os.TempDir(), fmt.Sprintf(".lanboat-tun-ns-%d.lock", ns.Sys().(*syscall.Stat_t).Ino))
	fd, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Geteuid()) || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return errors.New("invalid TUN session lock ownership")
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("a TUN session is already running")
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	if _, err = net.InterfaceByName(tunmode.Interface); err == nil {
		return errors.New("TUN interface already exists")
	}
	if err = checkRoutingSpace(); err != nil {
		return err
	}
	config := request.Config(ownerPath)
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() { io.Copy(io.Discard, reader); cancel() }()
	runtime, err := allinone.New(ctx, data)
	if err != nil {
		return err
	}
	defer runtime.Close()
	if err = runtime.Start(); err != nil {
		return err
	}
	if _, err = net.InterfaceByName(tunmode.Interface); err != nil {
		return err
	}
	fmt.Println("LANBOAT_TUN_READY")
	<-ctx.Done()
	return nil
}

type ipRule struct {
	Priority int `json:"priority"`
	Table    any `json:"table"`
}

func listRules(family string) ([]ipRule, error) {
	data, err := exec.Command("ip", family, "-j", "rule", "show").Output()
	if err != nil {
		return nil, err
	}
	var rules []ipRule
	err = json.Unmarshal(data, &rules)
	return rules, err
}
func checkRoutingSpace() error {
	for _, family := range []string{"-4", "-6"} {
		rules, err := listRules(family)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if rule.Priority >= tunmode.RuleStart && rule.Priority <= tunmode.RuleEnd || fmt.Sprint(rule.Table) == strconv.Itoa(tunmode.Table) {
				return errors.New("TUN routing table or priority is already in use")
			}
		}
		data, _ := exec.Command("ip", family, "route", "show", "table", strconv.Itoa(tunmode.Table)).Output()
		if len(bytes.TrimSpace(data)) != 0 {
			return errors.New("TUN routing table is already in use")
		}
	}
	return nil
}
