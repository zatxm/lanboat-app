package tunbridge

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/zatxm/lanboat-allinone/v26/native"
	"golang.org/x/sys/windows"
)

type packetStream struct {
	net.Conn
	writes sync.Mutex
	once   sync.Once
	done   chan struct{}
}

func (p *packetStream) Read(b []byte) (int, error) {
	n, err := readFrame(p.Conn, b)
	if err != nil && p.done != nil {
		p.once.Do(func() { close(p.done) })
	}
	return n, err
}
func (p *packetStream) Write(b []byte) (int, error) {
	p.writes.Lock()
	defer p.writes.Unlock()
	if err := writeFrame(p.Conn, b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func Open(ctx context.Context, executable string) (*Device, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	var nonce [32]byte
	rand.Read(nonce[:])
	port := listener.Addr().(*net.TCPAddr).Port
	// UAC may wait for the user. Cancel the listener even if the prompt is still open.
	elevation := make(chan error, 1)
	go func() {
		path := filepath.Join(base, "Lanboat", "TUN", "lanboat-tun-worker.exe")
		arguments := []string{strconv.Itoa(port), hex.EncodeToString(nonce[:]), strconv.Itoa(os.Getpid())}
		if windows.GetCurrentProcessToken().IsElevated() {
			cmd := exec.Command(path, arguments...)
			elevation <- cmd.Start()
			if cmd.Process != nil {
				_ = cmd.Wait()
			}
		} else {
			elevation <- elevate(path, strings.Join(arguments, " "))
		}
	}()
	go func() {
		if <-elevation != nil {
			listener.Close()
		}
	}()
	for {
		c, err := listener.Accept()
		if err != nil {
			return nil, errors.New("TUN administrator authorization was canceled or timed out")
		}
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		var auth [32]byte
		if _, err = io.ReadFull(c, auth[:]); err != nil || subtle.ConstantTimeCompare(auth[:], nonce[:]) != 1 {
			c.Close()
			continue
		}
		deadline, _ := ctx.Deadline()
		c.SetReadDeadline(deadline)
		info, err := receiveInfo(c)
		if err != nil {
			c.Close()
			return nil, err
		}
		c.SetDeadline(time.Time{})
		stream := &packetStream{Conn: c, done: make(chan struct{})}
		return &Device{ReadWriteCloser: stream, Info: info, Done: stream.done}, nil
	}
}

func Run(ctx context.Context, args []string) error {
	if !windows.GetCurrentProcessToken().IsElevated() || len(args) != 3 {
		return errors.New("TUN broker requires Windows administrator authorization")
	}
	port, err := strconv.ParseUint(args[0], 10, 16)
	nonce, decodeErr := hex.DecodeString(args[1])
	if err != nil || port < 1024 || decodeErr != nil || len(nonce) != 32 {
		return errors.New("invalid TUN broker endpoint")
	}
	c, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", "127.0.0.1:"+strconv.FormatUint(port, 10))
	if err != nil {
		return err
	}
	defer c.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	if _, err = c.Write(nonce); err != nil {
		return err
	}
	owner, err := backendProcess(args[2])
	if err != nil {
		_ = sendInfo(c, Info{}, err)
		return err
	}
	info, lease, err := native.OpenDesktopTUN(ctx, owner)
	if err != nil {
		_ = sendInfo(c, Info{Name: info.Name, Mark: info.Mark}, err)
		return err
	}
	defer lease.Close()
	if err = sendInfo(c, Info{Name: info.Name, Mark: info.Mark}, nil); err != nil {
		return err
	}
	packets := &packetStream{Conn: c}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer c.Close()
		forwardPackets(lease, packets)
	}()
	forwardPackets(packets, lease)
	lease.Close()
	<-done
	return nil
}

func forwardPackets(dst io.Writer, src io.Reader) {
	b := make([]byte, 65575)
	for {
		n, err := src.Read(b)
		if err != nil {
			return
		}
		if _, err = dst.Write(b[:n]); err != nil {
			return
		}
	}
}

func elevate(path, arguments string) error {
	type shellExecuteInfo struct {
		size, mask                        uint32
		window                            uintptr
		verb, file, parameters, directory *uint16
		show                              int32
		instance, idList                  uintptr
		class                             *uint16
		classKey                          uintptr
		hotKey                            uint32
		icon, process                     uintptr
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	parameters, _ := windows.UTF16PtrFromString(arguments)
	info := shellExecuteInfo{mask: 0x100 | 0x400, verb: verb, file: file, parameters: parameters}
	info.size = uint32(unsafe.Sizeof(info))
	call := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")
	if ok, _, e := call.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		return e
	}
	return nil
}

func backendProcess(value string) (string, error) {
	pid, err := strconv.ParseUint(value, 10, 32)
	if err != nil || pid == 0 {
		return "", errors.New("invalid TUN broker owner process")
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(proc)
	var path [32768]uint16
	length := uint32(len(path))
	if err = windows.QueryFullProcessImageName(proc, 0, &path[0], &length); err != nil {
		return "", err
	}
	name := windows.UTF16ToString(path[:length])
	if !strings.EqualFold(filepath.Base(name), "lanboat-core.exe") {
		return "", errors.New("TUN broker owner is not Lanboat's network backend")
	}
	return name, nil
}
