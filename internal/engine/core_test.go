package engine

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/goccy/go-json"
	allinone "github.com/zatxm/lanboat-allinone/v26"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"go.uber.org/zap"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func protocolFixture(t *testing.T, p profile.Protocol) profile.Node {
	t.Helper()
	n := profile.Node{ID: "test", Type: p.ID, Name: p.Label, Server: "127.0.0.1", Port: 443, Password: "protocol-test-secret", Method: "aes-256-gcm", Options: map[string]any{}}
	for _, field := range p.Fields {
		if field.Default != "" {
			n.SetOption(field.Key, field.Default)
		}
	}
	values := map[string]string{"username": "test", "uuid": "12345678-1234-4234-8234-123456789012", "ip": "10.0.0.2", "private-key": base64.StdEncoding.EncodeToString(append([]byte{1}, make([]byte, 31)...)), "public-key": base64.StdEncoding.EncodeToString(append([]byte{1}, make([]byte, 31)...)), "ss-method": "aes-256-gcm", "ss-password": "test-inner", "protocol": "origin", "obfs": "plain", "key": "sudoku-test-key-16bytes", "auth-key": "tskey-auth-test", "exit-node": "100.64.0.1", "script": profile.LuaHTTPExample}
	for _, field := range p.Fields {
		if field.Required {
			n.SetOption(field.Key, values[field.Key])
		}
	}
	if p.ID == "socks5" || p.ID == "socks5s" || p.ID == "http" || p.ID == "https" || p.ID == "http2" || p.ID == "openconnect" {
		n.SetOption("username", "test")
	}
	if p.ID == "ssr" {
		n.SetOption("cipher", "aes-256-cfb")
	}
	if p.ID == "tuic" {
		n.SetOption("uuid", values["uuid"])
	}
	if p.ID == "masque" {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		priv, _ := x509.MarshalECPrivateKey(key)
		pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
		n.SetOption("private-key", base64.StdEncoding.EncodeToString(priv))
		n.SetOption("public-key", base64.StdEncoding.EncodeToString(pub))
	}
	if err := n.Validate(); err != nil {
		t.Fatalf("invalid fixture %s: %v", p.ID, err)
	}
	return n
}
func TestAllProtocolsUseUnifiedCore(t *testing.T) {
	for _, p := range profile.Protocols {
		t.Run(p.ID, func(t *testing.T) {
			n := protocolFixture(t, p)
			raw, err := json.Marshal(map[string]any{"outbounds": []any{coreOutbound(n)}, "route": map[string]any{"final": "proxy"}})
			if err != nil {
				t.Fatal(err)
			}
			if err = allinone.Check(context.Background(), raw); err != nil {
				t.Fatalf("%v\n%s", err, raw)
			}
		})
	}
}
func TestEmbeddedProxyRequestLogging(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if len(body) != 100000 {
			t.Errorf("body was truncated during forwarding: %d", len(body))
		}
		io.WriteString(w, "complete")
	}))
	defer origin.Close()
	c := profile.Default()
	c.Settings.Mode = "direct"
	c.Settings.Port = freePort(t)
	c.Settings.ProxyLog = true
	store := traffic.New(t.TempDir())
	store.SetEnabled(true, false)
	e := New("", t.TempDir(), zap.NewNop())
	e.SetTrafficStore(store)
	if err := e.Start(c); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()
	proxy, _ := url.Parse("http://127.0.0.1:" + fmt.Sprint(c.Settings.Port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for i := range 2 {
		e.SetRequestLogging(true)
		target := fmt.Sprintf("%s/search?q=core&request=%d", origin.URL, i)
		req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(strings.Repeat("x", 100000)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Analysis", "present")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		entry := waitProxyRequestLog(t, store, http.MethodPost, target)
		if len(entry.Headers) != 0 || len(entry.Body) != 0 || entry.BodyTruncated {
			t.Fatalf("request %d: details recorded while disabled (headers=%d body=%d truncated=%t)", i, len(entry.Headers), len(entry.Body), entry.BodyTruncated)
		}
	}
	if !e.State().Connected {
		t.Fatal("log toggle restarted the core")
	}
}

func waitProxyRequestLog(t *testing.T, store *traffic.Store, method, target string) traffic.Entry {
	t.Helper()
	// Runtime callbacks and disk writes may finish after the client reads the response.
	deadline := time.Now().Add(3 * time.Second)
	for {
		page, err := store.Page("proxy", 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range page.Entries {
			if entry.Method == method && entry.URL == target {
				return entry
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing proxy request log for %s %s", method, target)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUnifiedProtocolRoundTrips(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "native payload") }))
	defer target.Close()
	cert := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	pair := cert.TLS.Certificates[0]
	cert.Close()
	key, _ := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	tlsServer := map[string]any{"enabled": true, "certificate": []string{string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}))}, "key": []string{string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))}}
	for _, kind := range []string{"ss", "ssr", "socks5", "socks5s", "http", "https", "vmess", "vless", "trojan", "hysteria", "hysteria2", "tuic", "juicity", "anytls", "snell", "relay", "brook"} {
		t.Run(kind, func(t *testing.T) {
			p, _ := profile.ProtocolByID(kind)
			n := protocolFixture(t, p)
			n.SetOption("username", "client")
			n.SetOption("skip-cert-verify", true)
			if kind == "vmess" || kind == "vless" {
				n.SetOption("tls", false)
			}
			out := coreOutbound(n)
			typ := out["type"].(string)
			in := map[string]any{"type": typ, "tag": "peer", "listen": "127.0.0.1", "listen_port": 0, "password": n.Password, "method": out["method"], "users": []any{map[string]any{"username": "client", "password": n.Password, "uuid": n.TextOption("uuid")}}}
			if kind == "brook" {
				delete(in, "users")
			}
			if in["method"] == nil {
				delete(in, "method")
			}
			for _, field := range []string{"protocol", "protocol_param", "obfs", "obfs_param", "version", "up_mbps", "down_mbps"} {
				if v, ok := out[field]; ok {
					in[field] = v
				}
			}
			if out["tls"] != nil {
				in["tls"] = tlsServer
			}
			raw, _ := json.Marshal(map[string]any{"inbounds": []any{in}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"final": "direct"}, "log": map[string]any{"disabled": true}})
			server, err := allinone.New(context.Background(), raw)
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			if err = server.Start(); err != nil {
				t.Fatal(err)
			}
			host, port, _ := net.SplitHostPort(server.Address("peer"))
			n.Server = host
			n.Port, _ = strconv.Atoi(port)
			engine := New("", t.TempDir(), zap.NewNop())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			probe, err := engine.ProbeClient(ctx, n)
			if err != nil {
				t.Fatal(err)
			}
			defer probe.Close()
			tr := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return probe.DialContext(ctx, address)
			}}
			defer tr.CloseIdleConnections()
			client := http.Client{Transport: tr, Timeout: 5 * time.Second}
			resp, err := client.Get(target.URL + "/actual")
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || string(body) != "native payload" {
				t.Fatal(string(body), err)
			}
		})
	}
}

func TestRoutingAndHTTPFrontUseEmbeddedCore(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "routed") }))
	defer origin.Close()
	parsed, err := routing.Parse("[Rule]\nUSER-AGENT,blocked*,REJECT\nDOMAIN,127.0.0.1,DIRECT\nFINAL,REJECT\n")
	if err != nil {
		t.Fatal(err)
	}
	c := profile.Default()
	c.Settings.Mode = "rule"
	c.Settings.Port = freePort(t)
	c.Routing = &parsed
	c.Nodes = []profile.Node{{ID: "unused", Name: "fixture", Type: "http", Server: "127.0.0.1", Port: 9}}
	c.Selected = "unused"
	e := New("", t.TempDir(), zap.NewNop())
	if err = e.Start(c); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", c.Settings.Port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, blocked := range []bool{false, true} {
		req, _ := http.NewRequest("GET", origin.URL, nil)
		if blocked {
			req.Header.Set("User-Agent", "blocked-client")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !blocked && string(body) != "routed" || blocked && resp.StatusCode != 403 {
			t.Fatal(resp.StatusCode, string(body))
		}
	}
}

func TestTransportAndFingerprintConversion(t *testing.T) {
	for _, kind := range []string{"vmess", "vless", "trojan"} {
		for _, network := range []string{"tcp", "http", "h2", "ws", "httpupgrade", "mkcp", "grpc", "quic", "xhttp"} {
			t.Run(kind+"/"+network, func(t *testing.T) {
				p, _ := profile.ProtocolByID(kind)
				n := protocolFixture(t, p)
				n.SetOption("network", network)
				n.SetOption("tls", false)
				n.SetOption("transport-path", "/wire")
				n.SetOption("transport-host", "front.test")
				if kind == "trojan" && network == "mkcp" || kind == "trojan" && network == "quic" {
					return
				}
				raw, _ := json.Marshal(map[string]any{"outbounds": []any{coreOutbound(n)}})
				if err := allinone.Check(context.Background(), raw); err != nil {
					t.Fatalf("%v\n%s", err, raw)
				}
			})
		}
	}
	for _, obfs := range []string{"none", "http", "tls", "ssl", "h1", "h2", "wss", "websocket", "httpupgrade", "xhttp"} {
		t.Run("ss/"+obfs, func(t *testing.T) {
			n := profile.Node{ID: "ss", Name: "ss", Type: "ss", Server: "127.0.0.1", Port: 443, Method: "aes-256-gcm", Password: "private"}
			n.SetOption("obfs", obfs)
			n.SetOption("obfs-host", "front.test")
			raw, _ := json.Marshal(map[string]any{"outbounds": []any{coreOutbound(n)}})
			if err := allinone.Check(context.Background(), raw); err != nil {
				t.Fatalf("%v\n%s", err, raw)
			}
		})
	}
	for _, fp := range profile.TLSFingerprints {
		t.Run(fp, func(t *testing.T) {
			n := profile.Node{Type: "trojan", Server: "127.0.0.1", Port: 443, Password: "private"}
			n.SetOption("client-fingerprint", fp)
			raw, _ := json.Marshal(map[string]any{"outbounds": []any{coreOutbound(n)}})
			if err := allinone.Check(context.Background(), raw); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDNSProbeUsesDistinctExecutablePath(t *testing.T) {
	source := filepath.Join(t.TempDir(), "client")
	if err := os.WriteFile(source, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	path, cleanup, err := prepareDNSProbe(source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if path == source {
		t.Fatal("diagnostic would bypass TUN")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "#!/bin/sh\nexit 0\n" {
		t.Fatal(err)
	}
	cleanup()
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("diagnostic executable not removed")
	}
}

func TestHTTPSLogDoesNotInventDecryptedRequests(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, "TLS response")
	}))
	defer origin.Close()
	c := profile.Default()
	c.Settings.Mode = "direct"
	c.Settings.Port = freePort(t)
	c.Settings.ProxyLog = true
	c.Settings.Proxy.Authentication = true
	c.Settings.Proxy.Username = "client"
	c.Settings.Proxy.Password = "proxy-secret"
	store := traffic.New(t.TempDir())
	store.SetEnabled(true, false)
	e := New("", t.TempDir(), zap.NewNop())
	e.SetTrafficStore(store)
	if err := e.Start(c); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()
	proxy, _ := url.Parse(fmt.Sprintf("http://client:proxy-secret@127.0.0.1:%d", c.Settings.Port))
	tr := &http.Transport{Proxy: http.ProxyURL(proxy), TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, Timeout: 3 * time.Second}
	req, _ := http.NewRequest("POST", origin.URL+"/secret?token=encrypted", strings.NewReader("private-body"))
	req.Header.Set("Authorization", "private-origin-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	waitProxyRequestLog(t, store, http.MethodConnect, strings.TrimPrefix(origin.URL, "https://"))
	page, err := store.Page("proxy", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range page.Entries {
		raw, _ := json.Marshal(entry)
		for _, secret := range []string{"proxy-secret", "private-origin-token", "private-body", "token=encrypted"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("encrypted request or proxy credential exposed")
			}
		}
		if entry.Method == "POST" {
			t.Fatal("encrypted method was invented")
		}
	}
}

func TestUnexpectedBackendExitStopsProxy(t *testing.T) {
	c := profile.Default()
	c.Settings.Mode = "direct"
	c.Settings.Port = freePort(t)
	e := New("", t.TempDir(), zap.NewNop())
	if err := e.Start(c); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()
	process, err := os.FindProcess(e.runtime.PID())
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.runtime.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("backend did not exit")
	}
	if state := e.State(); state.Connected || state.Error == "" {
		t.Fatal(state)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", c.Settings.Port))
	if err != nil {
		t.Fatal("orphaned local proxy", err)
	}
	listener.Close()
}

func TestEveryProtocolBuildsWithAppDefaults(t *testing.T) {
	for _, p := range profile.Protocols {
		t.Run(p.ID, func(t *testing.T) {
			n := protocolFixture(t, p)
			c := profile.Default()
			c.Settings.Mode = "global"
			c.Nodes = []profile.Node{n}
			c.Selected = n.ID
			raw, _ := json.Marshal(BuildConfig(c, "127.0.0.1:0", "secret"))
			if err := allinone.Check(context.Background(), raw); err != nil {
				t.Fatalf("%v\n%s", err, raw)
			}
		})
	}
}

func TestProfileIPRulesKeepProxyHostname(t *testing.T) {
	// IP predicates are resolved by the core when evaluated. An explicit resolve
	// action also replaces the destination hostname before it reaches HY2.
	rules, _ := coreProfileRules(&routing.Parsed{Rules: []routing.Rule{{Type: "IP-CIDR", Value: "192.0.2.0/24", Action: "direct"}}})
	if len(rules) != 1 || rules[0].(map[string]any)["ip_cidr"] == nil {
		t.Fatalf("unexpected route actions: %v", rules)
	}
}
