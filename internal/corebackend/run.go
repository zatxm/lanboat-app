package corebackend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	allinone "github.com/zatxm/lanboat-allinone/v26"
	"github.com/zatxm/lanboat-allinone/v26/native"
	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/resident"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"github.com/zatxm/lanboat-app/internal/tunbridge"
	"go.uber.org/zap"
)

type Tunnel struct {
	Device  io.ReadWriteCloser
	Info    native.DesktopTUNInfo
	Protect func(uintptr) error
	Done    <-chan struct{}
}

type OpenTunnel func(context.Context) (Tunnel, error)

func Run(ctx context.Context, input io.Reader, output io.Writer) error {
	return RunAttached(ctx, input, output, desktopTunnel)
}

func desktopTunnel(ctx context.Context) (Tunnel, error) {
	executable, err := os.Executable()
	if err != nil {
		return Tunnel{}, err
	}
	device, err := tunbridge.Open(ctx, executable)
	if err != nil {
		return Tunnel{}, err
	}
	return Tunnel{Device: device, Info: native.DesktopTUNInfo{Name: device.Info.Name, Mark: device.Info.Mark},
		Protect: device.Protect, Done: device.Done}, nil
}

func RunAttached(ctx context.Context, input io.Reader, output io.Writer, open OpenTunnel) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	enc := json.NewEncoder(output)
	send := func(response coreipc.Response) error {
		mu.Lock()
		defer mu.Unlock()
		return enc.Encode(response)
	}
	logEntry := func(entry traffic.Entry) { _ = send(coreipc.Response{Traffic: &entry}) }

	dec := json.NewDecoder(input)
	var init coreipc.Request
	if err := dec.Decode(&init); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	fail := func(err error) error {
		_ = send(coreipc.Response{ID: init.ID, Error: err.Error()})
		return err
	}
	if init.Op == "check" {
		if err := allinone.Check(ctx, init.Config); err != nil {
			return fail(err)
		}
		return send(coreipc.Response{ID: init.ID})
	}
	if init.Op == "probe" {
		return send(probe(ctx, init, nil))
	}
	if init.Op != "start" {
		return fail(errors.New("invalid network backend start command"))
	}
	requests := make(chan coreipc.Request)
	go func() {
		defer cancel()
		defer close(requests)
		for {
			var req coreipc.Request
			if err := dec.Decode(&req); err != nil {
				return
			}
			select {
			case requests <- req:
			case <-ctx.Done():
				return
			}
		}
	}()

	if init.TUN {
		if err := allinone.Check(ctx, init.Config); err != nil {
			return fail(err)
		}
		device, err := open(ctx)
		if err != nil {
			return fail(err)
		}
		defer device.Device.Close()
		ctx = native.WithDesktopTUN(ctx, device.Info, device.Device, device.Protect)
		if device.Done != nil {
			go func() {
				select {
				case <-device.Done:
					cancel()
				case <-ctx.Done():
				}
			}()
		}
	}
	rt, err := allinone.NewWithLogger(ctx, init.Config, zap.NewNop())
	if err != nil {
		return fail(err)
	}
	defer rt.Close()
	setLogging := func(flags coreipc.Logging) {
		if flags.Proxy {
			rt.SetConnectionLogger(func(c allinone.ConnectionLog) {
				if entry, ok := ConnectionEntry(c); ok {
					logEntry(entry)
				}
			}, true)
		} else {
			rt.SetConnectionLogger(nil, false)
		}
		if flags.Request {
			rt.SetRequestLogger(func(q allinone.RequestLog) {
				logEntry(traffic.Entry{Time: time.Now().Format(time.RFC3339Nano), Kind: "proxy", URL: q.URL, Method: q.Method, Network: "http", Incomplete: q.Incomplete})
			}, false)
		} else {
			rt.SetRequestLogger(nil, false)
		}
		if flags.DNS {
			rt.SetDNSLogger(func(q allinone.DNSLog) {
				if entry, ok := dnsEntry(q); ok {
					logEntry(entry)
				}
			})
		} else {
			rt.SetDNSLogger(nil)
		}
	}
	setLogging(init.Logging)
	if err = rt.Start(); err != nil {
		return fail(err)
	}
	if err = send(coreipc.Response{ID: init.ID, Controller: rt.ControllerAddress()}); err != nil {
		return err
	}
	go func() {
		debug.FreeOSMemory()
		resident.ReleaseExecutablePages()
	}()
	var probes sync.WaitGroup
	defer func() { cancel(); probes.Wait() }()
	slots := make(chan struct{}, 20)
	for {
		var req coreipc.Request
		select {
		case <-ctx.Done():
			return nil
		case next, ok := <-requests:
			if !ok {
				return nil
			}
			req = next
		}
		response := coreipc.Response{ID: req.ID}
		switch req.Op {
		case "probe":
			select {
			case slots <- struct{}{}:
				probes.Go(func() {
					defer func() { <-slots }()
					_ = send(probe(ctx, req, rt))
				})
				continue
			default:
				response.Error = "too many network probes"
			}
		case "stats":
			s := rt.Stats()
			response.Stats = &coreipc.Stats{Uplink: s.Uplink, Downlink: s.Downlink, Connections: s.Connections}
		case "logging":
			setLogging(req.Logging)
		default:
			response.Error = "unknown network backend command"
		}
		if err = send(response); err != nil {
			return err
		}
	}
}

func probe(ctx context.Context, req coreipc.Request, parent *allinone.Runtime) coreipc.Response {
	response := coreipc.Response{ID: req.ID}
	deadline := time.Now().Add(time.Minute)
	if req.Deadline != 0 {
		if requested := time.Unix(0, req.Deadline); requested.Before(deadline) {
			deadline = requested
		}
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	rt, err := allinone.NewWithLogger(ctx, req.Config, zap.NewNop())
	if err == nil {
		defer rt.Close()
		if parent != nil {
			parent.ShareDesktopEgress(rt)
		}
		err = rt.Start()
	}
	if err != nil {
		response.Error = err.Error()
		return response
	}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return rt.Dial(ctx, network, addr)
	}}
	defer transport.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, req.Target, nil)
	if err != nil {
		response.Error = err.Error()
		return response
	}
	start := time.Now()
	result, err := (&http.Client{Transport: transport}).Do(request)
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(result.Body, 65536))
		result.Body.Close()
		if result.StatusCode < 200 || result.StatusCode >= 400 {
			err = fmt.Errorf("连通性测试返回HTTP状态%d", result.StatusCode)
		}
	}
	if err != nil {
		response.Error = err.Error()
		return response
	}
	response.Latency = max(1, time.Since(start).Milliseconds())
	return response
}
