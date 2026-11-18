package desktop

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestUDPLatencyUsesProtocolWhileConnected(t *testing.T) {
	a := autoSelectApp(t)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer origin.Close()
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := certServer.TLS.Certificates[0]
	certServer.Close()
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	key, _ := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600)
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600)
	n := profile.Node{ID: "hy2", Type: "hysteria2", Name: "hy2", Server: "127.0.0.1", Port: autoSelectPort(t), Password: "fixture-auth"}
	n.SetOption("skip-cert-verify", true)
	n.SetOption("sni", "example.com")
	n.SetOption("obfs-password", "fixture-mask")
	config := map[string]any{"log": map[string]any{"disabled": true}, "inbounds": []any{map[string]any{"type": "hysteria2", "listen": "127.0.0.1", "listen_port": n.Port, "users": []any{map[string]any{"password": n.Password}}, "obfs": map[string]any{"type": "salamander", "password": "fixture-mask"}, "tls": map[string]any{"enabled": true, "certificate_path": certPath, "key_path": keyPath}}}, "outbounds": []any{map[string]any{"type": "direct"}}}
	b, _ := json.Marshal(config)
	path := filepath.Join(dir, "server.json")
	os.WriteFile(path, b, 0600)
	peer := exec.Command(os.Getenv("LANBOAT_TEST_CORE"), "run", "-c", path)
	if err := peer.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { peer.Process.Kill(); peer.Wait() }()
	time.Sleep(150 * time.Millisecond)
	a.config.Settings.TestURL = origin.URL
	a.config.Settings.TestMethod = "tcp"
	a.config.Settings.TestTimeout = 3
	proxy := httptest.NewServer(http.HandlerFunc(autoProxyTunnel))
	defer proxy.Close()
	active := autoHTTPNode("active", proxy.URL)
	a.config.Nodes = []profile.Node{active, n}
	a.config.Selected = active.ID
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	before := a.GetStatus()
	ms, err := a.TestNode(n.ID)
	if err != nil || ms < 1 {
		t.Fatal("UDP protocol probe failed", ms, err)
	}
	for _, legacy := range []string{"http", "icmp", "tcp", "connect", ""} {
		a.config.Settings.TestMethod = legacy
		if _, err := a.TestNode(n.ID); err != nil {
			t.Fatal("legacy method changed protocol probe", legacy, err)
		}
	}
	if after := a.GetStatus(); !after.Connected || after.StartedAt != before.StartedAt || after.ActiveNode != active.ID {
		t.Fatal("probe disturbed active connection")
	}
	a.config.Settings.TestMethod = "tcp"
	id, err := a.fastestNode(a.connectionCtx, profile.Config{Nodes: []profile.Node{n}, Settings: a.config.Settings, Selected: n.ID})
	if err != nil || id != n.ID {
		t.Fatal("automatic selection skipped UDP node", id, err)
	}
}
