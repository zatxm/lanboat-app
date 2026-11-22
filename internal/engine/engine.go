package engine

import (
	"context"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	allinone "github.com/zatxm/lanboat-allinone"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"go.uber.org/zap"
)

type Status struct {
	Connected     bool   `json:"connected"`
	StartedAt     int64  `json:"startedAt"`
	Upload        int64  `json:"upload"`
	Download      int64  `json:"download"`
	UploadRate    int64  `json:"uploadRate"`
	DownloadRate  int64  `json:"downloadRate"`
	Connections   int    `json:"connections"`
	Error         string `json:"error"`
	CoreAvailable bool   `json:"coreAvailable"`
}

type Engine struct {
	mu      sync.Mutex
	runtime *allinone.Runtime
	cancel  context.CancelFunc

	traffic                    *traffic.Store
	trafficQueue               *trafficQueue
	dnsPorts                   []int
	dnsServers                 []any
	dnsConfig                  profile.Config
	tun                        *tunSession
	fallbackGroups             []fallbackGroup
	fallbackReservations       []net.Listener
	front                      *proxyFront
	path, dir, control, secret string
	done                       chan struct{}
	tunDNSPort                 int
	status                     Status
	lastSample                 time.Time
	log                        *zap.Logger
}

func New(path, dir string, log *zap.Logger) *Engine {
	if log == nil {
		log = zap.NewNop()
	}
	return &Engine{path: path, dir: dir, log: log}
}
func FindCore() string                         { exe, _ := os.Executable(); return exe }
func (e *Engine) CorePath() string             { return e.path }
func (e *Engine) Start(c profile.Config) error { return e.StartContext(context.Background(), c) }
func (e *Engine) StartContext(ctx context.Context, c profile.Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.runtime != nil {
		return errors.New("请先断开连接")
	}
	if err := os.MkdirAll(e.dir, 0700); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			e.cleanup()
		}
	}()
	node := selectedNode(c)
	if c.Settings.Mode != "direct" && node.ID == "" {
		return errors.New("请先选择一个节点")
	}
	if c.Settings.Mode == "rule" {
		var parsed routing.Parsed
		var err error
		if c.Routing != nil {
			parsed = *c.Routing
		} else {
			parsed, err = profile.EffectiveRouting(c)
			if err != nil {
				return err
			}
		}
		if err = profile.InsertCustomRouting(&parsed, c.Rules); err != nil {
			return err
		}
		c.Rules = nil
		if err = routing.ResolveSets(ctx, &parsed, e.path, e.dir); err != nil {
			return err
		}
		c.Routing = &parsed
	}
	publicPort := c.Settings.Port
	useFront := c.Settings.Mode == "rule" && c.Routing != nil && len(c.Routing.Rewrites) > 0
	if c.Routing != nil {
		for _, rule := range c.Routing.Rules {
			useFront = useFront || c.Settings.Mode == "rule" && rule.Type == "USER-AGENT"
		}
	}
	// Reserve the public port before allocating private listeners.
	reservation, portErr := net.Listen("tcp", net.JoinHostPort(c.Settings.Proxy.ListenHost(), fmt.Sprint(publicPort)))
	if portErr != nil {
		return fmt.Errorf("本地代理端口不可用：%w", portErr)
	}
	var reservations = []net.Listener{reservation}
	releaseReservations := func() {
		for _, l := range reservations {
			l.Close()
		}
		reservations = nil
	}
	defer releaseReservations()
	reserve := func() (int, error) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		reservations = append(reservations, l)
		return l.Addr().(*net.TCPAddr).Port, nil
	}
	if useFront {
		var err error
		c.Settings.Port, err = reserve()
		if err != nil {
			return err
		}
		c.ForceProxyPort, err = reserve()
		if err != nil {
			return err
		}
		c.ForceDirectPort, err = reserve()
		if err != nil {
			return err
		}
		c.PolicyPorts = map[string]int{}
		for _, rule := range c.Routing.Rules {
			if rule.Type == "USER-AGENT" && rule.Action != "proxy" && rule.Action != "direct" && rule.Action != "reject" && c.PolicyPorts[rule.Action] == 0 {
				c.PolicyPorts[rule.Action], err = reserve()
				if err != nil {
					return err
				}
			}
		}
	}
	e.secret = profile.ID()
	controlPort, controlErr := reserve()
	if controlErr != nil {
		return controlErr
	}
	config := BuildConfig(c, net.JoinHostPort("127.0.0.1", fmt.Sprint(controlPort)), e.secret)
	if c.Settings.DNS != nil && (c.Settings.DNS.Persist || c.Settings.DNS.FakeIP) {
		config["experimental"].(map[string]any)["cache_file"] = map[string]any{"enabled": true, "path": filepath.Join(e.dir, "dns-cache"), "cache_id": "lanboat", "store_dns": c.Settings.DNS.Persist, "store_fakeip": c.Settings.DNS.FakeIP}
	}
	var err error
	if useFront {
		in := config["inbounds"].([]any)[0].(map[string]any)
		in["listen"] = "127.0.0.1"
		delete(in, "users")
	}
	if err := e.preparePolicies(ctx, c, config); err != nil {
		return err
	}
	if c.Settings.Mode == "tun" {
		e.tunDNSPort, err = reserve()
		if err != nil {
			return err
		}
		config["inbounds"] = append(config["inbounds"].([]any), map[string]any{"type": "direct", "tag": "tun-in", "listen": "127.0.0.1", "listen_port": e.tunDNSPort, "network": "tcp"})
		route := config["route"].(map[string]any)
		route["rules"] = append([]any{map[string]any{"inbound": []string{"tun-in"}, "action": "hijack-dns"}}, route["rules"].([]any)...)
	}
	ports, release, err := configureDNSChecks(config)
	if err != nil {
		return err
	}
	defer release()
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	// The lifetime belongs to Stop; a completed UI operation must not terminate the core.
	runCtx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	stopStartup := context.AfterFunc(ctx, cancel)
	defer stopStartup()
	runtime, err := allinone.NewWithLogger(runCtx, raw, zap.NewNop())
	if err != nil {
		return err
	}
	e.runtime = runtime
	if e.trafficQueue != nil {
		e.trafficQueue.close()
		e.trafficQueue = nil
	}
	if e.traffic != nil {
		e.trafficQueue = newTrafficQueue(e.traffic, e.log)
	}
	e.configureRequestLog(c.Settings.ProxyLog)
	release()
	releaseReservations()
	if err = runtime.Start(); err != nil {
		e.cleanup()
		return err
	}
	e.control = runtime.ControllerAddress()
	e.done = make(chan struct{})
	for _, group := range e.fallbackGroups {
		go monitorFallback(e.control, e.secret, e.done, group)
	}
	e.dnsPorts = ports
	e.dnsServers = config["dns"].(map[string]any)["servers"].([]any)
	e.dnsConfig = c
	if useFront {
		e.front, err = newProxyFront(publicPort, c.Settings.Port, c.ForceProxyPort, c.ForceDirectPort, *c.Routing, c.Settings.Proxy, c.PolicyPorts, e.path)
		if err != nil {
			e.cleanup()
			return err
		}
	}
	if c.Settings.Mode == "tun" {
		// The privileged worker reaches the same authenticated mixed listener.
		if useFront {
			c.Settings.Port = publicPort
		}
		if err = e.startTUN(ctx, c); err != nil {
			e.cleanup()
			return err
		}
	}
	stopStartup()
	if err = ctx.Err(); err != nil {
		return err
	}
	success = true
	e.status = Status{Connected: true, CoreAvailable: true, StartedAt: time.Now().UnixMilli()}
	e.lastSample = time.Now()
	return nil
}
func (e *Engine) cleanup() {
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.stopTUN()
	if e.done != nil {
		close(e.done)
		e.done = nil
	}
	e.closePolicies()
	if e.front != nil {
		e.front.Close()
		e.front = nil
	}
	if e.runtime != nil {
		e.runtime.Close()
		e.runtime = nil
	}
	// After the runtime stops logging, flush its connection records.
	if e.trafficQueue != nil {
		e.trafficQueue.close()
		e.trafficQueue = nil
	}
	e.dnsPorts = nil
	e.dnsServers = nil
}
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cleanup()
	e.status = Status{CoreAvailable: true}
}
func (e *Engine) Connected() bool { return e.State().Connected }
func (e *Engine) State() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status.CoreAvailable = true
	e.status.Connected = e.connected()
	return e.status
}
func (e *Engine) Snapshot() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.connected() {
		e.status.CoreAvailable = true
		return e.status
	}
	stats := e.runtime.Stats()
	now := time.Now()
	elapsed := now.Sub(e.lastSample).Seconds()
	up, down := int64(stats.Uplink), int64(stats.Downlink)
	if elapsed > 0 {
		e.status.UploadRate = int64(float64(up-e.status.Upload) / elapsed)
		e.status.DownloadRate = int64(float64(down-e.status.Download) / elapsed)
	}
	e.status.Upload, e.status.Download, e.status.Connections = up, down, int(stats.Connections)
	e.lastSample = now
	return e.status
}
func selectedNode(c profile.Config) profile.Node {
	for _, n := range c.Nodes {
		if n.ID == c.Selected {
			return n
		}
	}
	return profile.Node{}
}

func (e *Engine) connected() bool {
	if e.runtime == nil {
		return false
	}
	if e.tun != nil {
		select {
		case <-e.tun.done:
			message := "TUN会话意外结束"
			if e.tun.err != nil {
				message += "：" + e.tun.err.Error()
			}
			e.cleanup()
			e.status = Status{CoreAvailable: true, Error: message}
			return false
		default:
		}
	}
	return true
}
