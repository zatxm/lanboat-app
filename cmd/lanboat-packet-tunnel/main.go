//go:build ios && cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
int lbTunnelWrite(int64_t id, const void *packet, int size);
void lbTunnelStarted(int64_t id, const char *errorText);
void lbTunnelFailed(int64_t id, const char *errorText);
void lbTunnelStopped(int64_t id);
void lbTunnelReply(int64_t ticket, const void *data, int size);
*/
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
	"unsafe"

	json "github.com/goccy/go-json"
	"github.com/zatxm/lanboat-allinone/v26/native"
	"github.com/zatxm/lanboat-app/internal/corebackend"
	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/mobilevpn"
	"github.com/zatxm/lanboat-app/internal/traffic"
)

// The core expects an address-family header on Darwin's utun device.
type utunPackets struct{ *mobilevpn.Packets }

func (d utunPackets) Read(buf []byte) (int, error) {
	n, err := d.Packets.Read(buf[4:])
	if err != nil {
		return 0, err
	}
	family := uint32(2)
	if buf[4]>>4 == 6 {
		family = 30
	}
	binary.BigEndian.PutUint32(buf[:4], family)
	return n + 4, nil
}

func (d utunPackets) Write(buf []byte) (int, error) {
	n, err := d.Packets.Write(buf[4:])
	if err != nil {
		return n, err
	}
	return n + 4, nil
}

func tunnelInterface() (string, error) {
	list, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range list {
		if !strings.HasPrefix(iface.Name, "utun") {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err == nil && prefix.Addr() == netip.MustParseAddr("172.29.45.1") {
				return iface.Name, nil
			}
		}
	}
	return "", errors.New("iOS did not expose the Lanboat tunnel interface")
}

type tunnelRun struct {
	id       int64
	device   *mobilevpn.Packets
	cancel   context.CancelFunc
	mu       sync.Mutex
	session  *coreipc.Client
	logs     []traffic.Entry
	finished chan struct{}
}

var tunnels sync.Map

func main() {}

func tunnelReply(ticket C.int64_t, response coreipc.Response, logs []traffic.Entry) {
	data, _ := json.Marshal(struct {
		Response coreipc.Response
		Logs     []traffic.Entry
	}{response, logs})
	C.lbTunnelReply(ticket, unsafe.Pointer(unsafe.SliceData(data)), C.int(len(data)))
}

//export LanboatTunnelStart
func LanboatTunnelStart(id C.int64_t, data unsafe.Pointer, size C.int) {
	var req coreipc.Request
	if err := json.Unmarshal(C.GoBytes(data, size), &req); err != nil || req.Op != "start" || !req.TUN {
		started(id, errors.New("Invalid VPN configuration"))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &tunnelRun{id: int64(id), cancel: cancel, finished: make(chan struct{})}
	t.device = mobilevpn.NewPackets(func(packet []byte) bool {
		return len(packet) > 0 && C.lbTunnelWrite(id, unsafe.Pointer(unsafe.SliceData(packet)), C.int(len(packet))) != 0
	})
	tunnels.Store(t.id, t)
	go func() {
		defer close(t.finished)
		defer t.device.Close()
		defer tunnels.Delete(t.id)
		defer cancel()
		startup, finish := context.WithTimeout(ctx, 20*time.Second)
		session, err := coreipc.LaunchEmbedded(startup, req, t.record, func(lifetime context.Context, in io.Reader, out io.Writer) error {
			return corebackend.RunAttached(lifetime, in, out, func(context.Context) (corebackend.Tunnel, error) {
				name, err := tunnelInterface()
				if err != nil {
					return corebackend.Tunnel{}, err
				}
				return corebackend.Tunnel{Device: utunPackets{t.device}, Info: native.DesktopTUNInfo{Name: name}}, nil
			})
		})
		finish()
		if err != nil {
			started(id, err)
			return
		}
		t.mu.Lock()
		t.session = session
		t.mu.Unlock()
		started(id, nil)
		select {
		case <-ctx.Done():
		case <-session.Done():
			if ctx.Err() == nil {
				err := session.Err()
				if err == nil {
					err = errors.New("Lanboat network core stopped")
				}
				message := C.CString(err.Error())
				C.lbTunnelFailed(id, message)
				C.free(unsafe.Pointer(message))
			}
		}
		session.Close()
	}()
}

func started(id C.int64_t, err error) {
	if err == nil {
		C.lbTunnelStarted(id, nil)
		return
	}
	text := C.CString(err.Error())
	defer C.free(unsafe.Pointer(text))
	C.lbTunnelStarted(id, text)
}

func (t *tunnelRun) record(log traffic.Entry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.logs) == 96 {
		copy(t.logs, t.logs[1:])
		t.logs = t.logs[:95]
	}
	t.logs = append(t.logs, log)
}

//export LanboatTunnelPacket
func LanboatTunnelPacket(id C.int64_t, packet unsafe.Pointer, size C.int) {
	if t, ok := tunnels.Load(int64(id)); ok && size > 0 && size <= 65575 {
		t.(*tunnelRun).device.Push(C.GoBytes(packet, size))
	}
}

//export LanboatTunnelStop
func LanboatTunnelStop(id C.int64_t) {
	if value, ok := tunnels.Load(int64(id)); ok {
		t := value.(*tunnelRun)
		t.cancel()
		go func() {
			<-t.finished
			C.lbTunnelStopped(id)
		}()
	} else {
		C.lbTunnelStopped(id)
	}
}

//export LanboatTunnelMessage
func LanboatTunnelMessage(id C.int64_t, ticket C.int64_t, data unsafe.Pointer, size C.int) {
	var req coreipc.Request
	value, ok := tunnels.Load(int64(id))
	if !ok || json.Unmarshal(C.GoBytes(data, size), &req) != nil {
		tunnelReply(ticket, coreipc.Response{Error: "iOS VPN is not connected"}, nil)
		return
	}
	t := value.(*tunnelRun)
	go func() {
		t.mu.Lock()
		session := t.session
		t.mu.Unlock()
		if session == nil {
			tunnelReply(ticket, coreipc.Response{Error: "iOS VPN is still starting"}, nil)
			return
		}
		deadline := time.Now().Add(30 * time.Second)
		if req.Deadline != 0 && time.Unix(0, req.Deadline).Before(deadline) {
			deadline = time.Unix(0, req.Deadline)
		}
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		response, err := session.Call(ctx, req)
		if err != nil {
			response.Error = err.Error()
		}
		t.mu.Lock()
		logs := t.logs
		t.logs = nil
		t.mu.Unlock()
		tunnelReply(ticket, response, logs)
	}()
}
