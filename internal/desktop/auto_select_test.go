package desktop

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
)

func autoSelectPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func autoSelectApp(t *testing.T) *App {
	t.Helper()
	core := os.Getenv("LANBOAT_TEST_CORE")
	if core == "" {
		t.Skip("set LANBOAT_TEST_CORE for local core integration")
	}
	t.Setenv("LANBOAT_CORE", core)
	a, err := New(t.TempDir(), core, zap.NewNop(), &Logs{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Shutdown(nil) })
	a.config.Settings.Port = autoSelectPort(t)
	a.config.Settings.Mode = "global"
	a.config.Settings.TestMethod = "http"
	a.config.Settings.TestURL = "http://probe.invalid/health"
	a.config.Settings.TestTimeout = 1
	return a
}

func autoHTTPNode(id, address string) profile.Node {
	u, _ := url.Parse(address)
	port, _ := strconv.Atoi(u.Port())
	return profile.Node{ID: id, Type: "http", Name: id, Server: u.Hostname(), Port: port}
}

func autoProxyTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	upstream, err := net.DialTimeout("tcp", r.Host, time.Second)
	if err != nil {
		w.WriteHeader(502)
		return
	}
	defer upstream.Close()
	client, buffer, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	_, _ = buffer.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
	_ = buffer.Flush()
	go func() { _, _ = io.Copy(upstream, buffer); _ = upstream.Close() }()
	_, _ = io.Copy(client, upstream)
}

func TestFastestNodeSkipsUnreachableAndUnsupported(t *testing.T) {
	a := testApp(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(autoProxyTunnel))
	defer proxy.Close()
	good := autoHTTPNode("good", proxy.URL)
	a.config.Settings.TestURL = origin.URL
	bad := good
	bad.ID, bad.Port = "bad", autoSelectPort(t)
	unsupported := good
	unsupported.ID, unsupported.Type = "unsupported", "not-a-protocol"
	legacy := good
	legacy.ID, legacy.Type, legacy.Method, legacy.Password = "legacy", "ss", "aes-256-gcm", "test"
	legacy.SetOption("ota", true)
	c := a.config
	c.Nodes, c.Selected = []profile.Node{bad, unsupported, legacy, good}, bad.ID
	id, err := a.fastestNode(context.Background(), c, "")
	if err != nil || id != good.ID {
		t.Fatal(id, err)
	}
	if a.nodeDelays[bad.ID] != -1 || a.nodeDelays[unsupported.ID] != -1 || a.nodeDelays[legacy.ID] != -1 || a.nodeDelays[good.ID] < 1 {
		t.Fatal(a.nodeDelays)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.fastestNode(ctx, c, ""); err != context.Canceled {
		t.Fatal("cancellation ignored", err)
	}
}

func TestAutoSelectConnectedAndNextConnect(t *testing.T) {
	a := autoSelectApp(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer origin.Close()
	a.config.Settings.TestURL = origin.URL
	var calls atomic.Int32
	counts := map[string]*atomic.Int32{"slow": {}, "fast": {}, "bad": {}}
	proxy := func(id string, delay time.Duration, status int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			counts[id].Add(1)
			time.Sleep(delay)
			if status != 204 {
				w.WriteHeader(status)
				return
			}
			autoProxyTunnel(w, r)
		}))
	}
	slow, fast, bad := proxy("slow", 100*time.Millisecond, 204), proxy("fast", 2*time.Millisecond, 204), proxy("bad", 0, 503)
	defer slow.Close()
	defer fast.Close()
	defer bad.Close()
	a.config.Nodes = []profile.Node{autoHTTPNode("slow", slow.URL), autoHTTPNode("bad", bad.URL), autoHTTPNode("fast", fast.URL)}
	a.config.Selected = "slow"
	if err := a.commit(a.config); err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAutoSelect("", true); err != nil {
		t.Fatal(err)
	}
	state, status := a.GetState(), a.GetStatus()
	if !status.Connected || status.ActiveNode != "fast" || state.Config.Selected != "fast" || !state.Config.Settings.AutoSelectEnabled("") {
		t.Fatal(state.Config.Settings, status)
	}
	if state.NodeDelays["fast"] >= state.NodeDelays["slow"] || state.NodeDelays["bad"] != -1 {
		t.Fatal(state.NodeDelays)
	}
	assertProxy := func(expected string) {
		t.Helper()
		before := counts[expected].Load()
		p, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(a.config.Settings.Port))
		transport := &http.Transport{Proxy: http.ProxyURL(p)}
		defer transport.CloseIdleConnections()
		response, err := (&http.Client{Transport: transport, Timeout: 2 * time.Second}).Get(a.config.Settings.TestURL)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 204 || counts[expected].Load() <= before {
			t.Fatal("traffic used the wrong proxy", response.StatusCode, expected)
		}
	}
	assertProxy("fast")
	before := calls.Load()
	if err := a.SetAutoSelect("", false); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before || a.GetStatus().ActiveNode != "fast" || a.GetState().Config.Settings.AutoSelectEnabled("") {
		t.Fatal("disabling restarted or tested nodes")
	}
	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if err := a.SelectNode("slow"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAutoSelect("", true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before {
		t.Fatal("disconnected toggle unexpectedly probed")
	}
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	assertProxy("fast")
	if a.GetStatus().ActiveNode != "fast" {
		t.Fatal("connect ignored automatic selection")
	}
}

func TestAutoSelectIsScopedToSelectedGroup(t *testing.T) {
	a := testApp(t)
	t.Cleanup(func() { a.Shutdown(nil) })
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer origin.Close()
	a.config.Settings.Port = autoSelectPort(t)
	a.config.Settings.Mode = "global"
	a.config.Settings.TestURL = origin.URL
	a.config.Settings.TestTimeout = 2

	var localCalls, subCalls atomic.Int32
	node := func(id, source string, delay time.Duration, calls *atomic.Int32) profile.Node {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			time.Sleep(delay)
			autoProxyTunnel(w, r)
		}))
		t.Cleanup(server.Close)
		n := autoHTTPNode(id, server.URL)
		n.Source = source
		return n
	}
	a.config.Subscriptions = []profile.Subscription{{ID: "feed", Name: "Feed", URL: "https://example.com/feed"}}
	a.config.Nodes = []profile.Node{
		node("local", "orphan", 0, &localCalls),
		node("sub-slow", "feed", 100*time.Millisecond, &subCalls),
		node("sub-fast", "feed", 40*time.Millisecond, &subCalls),
	}
	a.config.Selected = "local"
	if err := a.SetAutoSelect("", true); err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	if localCalls.Load() == 0 || subCalls.Load() != 0 || a.GetStatus().ActiveNode != "local" {
		t.Fatal("local auto selection probed or switched to the subscription")
	}
	if err := a.SetAutoSelect("feed", true); err != nil {
		t.Fatal(err)
	}
	if subCalls.Load() != 0 || a.GetStatus().ActiveNode != "local" {
		t.Fatal("enabling a different group disturbed the active connection")
	}
	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if err := a.SelectNode("sub-slow"); err != nil {
		t.Fatal(err)
	}
	initialLocalCalls := localCalls.Load()
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	state := a.GetState()
	if localCalls.Load() != initialLocalCalls || subCalls.Load() == 0 || a.GetStatus().ActiveNode != "sub-fast" {
		t.Fatal("subscription auto selection did not stay within the subscription")
	}
	if !state.Config.Settings.AutoSelectEnabled("") || !state.Config.Settings.AutoSelectEnabled("feed") ||
		state.NodeDelays["local"] < 1 || state.NodeDelays["sub-fast"] < 1 {
		t.Fatal("group settings or previous measurements were lost")
	}
	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteSubscription("feed"); err != nil {
		t.Fatal(err)
	}
	settings := a.GetState().Config.Settings
	if settings.AutoSelectEnabled("feed") || !settings.AutoSelectEnabled("") {
		t.Fatal("deleting the subscription affected the wrong setting")
	}
}

func TestAutoSelectFailurePreservesOriginalConnection(t *testing.T) {
	a := autoSelectApp(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer origin.Close()
	a.config.Settings.TestURL = origin.URL
	var fail atomic.Bool
	var slowCalls atomic.Int32
	proxy := func(id string, delay time.Duration) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id == "slow" {
				slowCalls.Add(1)
			}
			if fail.Load() {
				w.WriteHeader(503)
				return
			}
			time.Sleep(delay)
			autoProxyTunnel(w, r)
		}))
	}
	slow, fast := proxy("slow", 80*time.Millisecond), proxy("fast", time.Millisecond)
	defer slow.Close()
	defer fast.Close()
	a.config.Nodes = []profile.Node{autoHTTPNode("slow", slow.URL), autoHTTPNode("fast", fast.URL)}
	a.config.Selected = "slow"
	if err := a.commit(a.config); err != nil {
		t.Fatal(err)
	}
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := a.SetAutoSelect("", true); err == nil {
		t.Fatal("all failed probes were accepted")
	}
	if status := a.GetStatus(); !status.Connected || status.ActiveNode != "slow" || a.GetState().Config.Settings.AutoSelectEnabled("") {
		t.Fatal("failed detection lost original connection", status)
	}
	if os.Geteuid() == 0 {
		return
	}
	fail.Store(false)
	dir := filepath.Dir(a.path)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)
	if err := a.SetAutoSelect("", true); err == nil || !strings.Contains(err.Error(), "已恢复原节点") {
		t.Fatal("failed persistence did not trigger connection rollback", err)
	}
	if status := a.GetStatus(); !status.Connected || status.ActiveNode != "slow" || a.GetState().Config.Settings.AutoSelectEnabled("") {
		t.Fatal("save failure did not restore original node", status)
	}
	p, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(a.config.Settings.Port))
	before := slowCalls.Load()
	transport := &http.Transport{Proxy: http.ProxyURL(p)}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport, Timeout: 2 * time.Second}).Get(a.config.Settings.TestURL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 204 || slowCalls.Load() <= before {
		t.Fatal("rollback did not restore forwarding")
	}
}

func TestDisconnectCancelsAutomaticSelection(t *testing.T) {
	a := autoSelectApp(t)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { entered <- struct{}{}; <-release; w.WriteHeader(204) }))
	defer server.Close()
	defer close(release)
	a.config.Nodes = []profile.Node{autoHTTPNode("slow", server.URL)}
	a.config.Selected = "slow"
	a.config.Settings.TestTimeout = 30
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- a.SetAutoSelect("", true) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not start")
	}
	start := time.Now()
	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("disconnect waited for probe timeout")
	}
	if err := <-done; err == nil {
		t.Fatal("canceled selection reported success")
	}
	if status := a.GetStatus(); status.Connected || status.ActiveNode != "" {
		t.Fatal("cancellation restarted a core", status)
	}
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(a.path), "runtime"))
	if len(entries) != 0 {
		t.Fatal("temporary runtime credentials retained", entries)
	}
}

func TestAutoSelectDirectModeNeedsNoNodes(t *testing.T) {
	a := autoSelectApp(t)
	a.config.Settings.Mode = "direct"
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAutoSelect("", true); err != nil {
		t.Fatal(err)
	}
	if status := a.GetStatus(); !status.Connected || status.ActiveNode != "" {
		t.Fatal(status)
	}
}
