package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func seedBackupApp(t *testing.T) *App {
	t.Helper()
	a := testApp(t)
	if err := a.SaveNode(profile.Node{Name: "original", Server: "example.com", Port: 443, Method: "aes-256-gcm", Password: "private"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveConfigFile("", "active.conf", "[Rule]\nDOMAIN,backup.test,REJECT\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	if err := a.ActivateConfigFile(a.GetState().Config.Files[1].ID); err != nil {
		t.Fatal(err)
	}
	d := profile.DefaultDNS()
	d.Capacity = 2048
	s := a.GetState().Config.Settings
	s.DNS = &d
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	return a
}
func TestRoutingBackupContainsAndRestoresGenerations(t *testing.T) {
	a := seedBackupApp(t)
	root := a.ruleDataDir()
	if err := ruledata.Ensure(root); err != nil {
		t.Fatal(err)
	}
	first, err := ruledata.Current(root)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	if err = ruledata.WriteGeneration(first, &compressed); err != nil {
		t.Fatal(err)
	}
	second, err := ruledata.Unpack(root, bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err = ruledata.Activate(root, second); err != nil {
		t.Fatal(err)
	}
	info, err := a.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(info.ID, ".tar.gz") {
		t.Fatal(info.ID)
	}
	if _, err = os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("old generation not compacted: %v", err)
	}
	if _, err = os.Stat(first + ".tar.gz"); err != nil {
		t.Fatal(err)
	}
	path, _ := a.backupPath(info.ID)
	b, stage, active, err := stageRoutingBackup(path, root)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stage)
	if b.Version != 1 || active != filepath.Base(second) {
		t.Fatal(b.Version, active)
	}
	if _, err = os.Stat(filepath.Join(stage, filepath.Base(first)+".tar.gz")); err != nil {
		t.Fatal(err)
	}
	// Change the active data before restoring; the snapshot must not overwrite it.
	third, err := ruledata.Unpack(root, bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err = ruledata.Activate(root, third); err != nil {
		t.Fatal(err)
	}
	if err = a.RestoreBackup(info.ID); err != nil {
		t.Fatal(err)
	}
	current, err := ruledata.Current(root)
	if err != nil || current == third {
		t.Fatal(current, err)
	}
	if err = ruledata.VerifyDir(current); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var dirs, archives int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "generation-") {
			if e.IsDir() {
				dirs++
			} else if strings.HasSuffix(e.Name(), ".tar.gz") {
				archives++
			}
		}
	}
	if dirs != 1 || archives < 3 {
		t.Fatalf("generation directories %d archives %d", dirs, archives)
	}
	exported, err := a.ExportBackupTo(info.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile(path)
	got, _ := os.ReadFile(exported)
	if !bytes.Equal(want, got) {
		t.Fatal("export differs")
	}
}

func TestCorruptRoutingBackupDoesNotChangeCurrentGeneration(t *testing.T) {
	a := seedBackupApp(t)
	if err := ruledata.Ensure(a.ruleDataDir()); err != nil {
		t.Fatal(err)
	}
	info, err := a.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	path, _ := a.backupPath(info.ID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data[:len(data)-16], 0600); err != nil {
		t.Fatal(err)
	}
	before, err := ruledata.Current(a.ruleDataDir())
	if err != nil {
		t.Fatal(err)
	}
	config := a.GetState().Config
	if err = a.RestoreBackup(info.ID); err == nil {
		t.Fatal("corrupt backup accepted")
	}
	after, err := ruledata.Current(a.ruleDataDir())
	if err != nil || after != before {
		t.Fatal(after, err)
	}
	if !reflect.DeepEqual(config, a.GetState().Config) {
		t.Fatal("config changed")
	}
}

func TestRoutingBackupRestoreWhileConnected(t *testing.T) {
	a := autoSelectApp(t)
	proxy := httptest.NewServer(http.HandlerFunc(autoProxyTunnel))
	defer proxy.Close()
	n := autoHTTPNode("active", proxy.URL)
	a.config.Nodes = []profile.Node{n}
	a.config.Selected = n.ID
	a.config.Settings.Mode = "rule"
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	info, err := a.CreateBackup()
	if err != nil {
		t.Fatal(err)
	}
	before, err := ruledata.Current(a.ruleDataDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = a.RestoreBackup(info.ID); err != nil {
		t.Fatal(err)
	}
	after, err := ruledata.Current(a.ruleDataDir())
	if err != nil || after == before || !a.GetStatus().Connected {
		t.Fatal(after, err, "connected", a.GetStatus().Connected)
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer origin.Close()
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", a.config.Settings.Port))
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || string(data) != "ok" {
		t.Fatal(string(data), err)
	}
}

func TestSettingsSnapshots(t *testing.T) {
	a := seedBackupApp(t)
	for i := 0; i < MaxBackups; i++ {
		if _, err := a.CreateBackup(); err != nil {
			t.Fatal(err)
		}
	}
	list := a.Backups()
	if _, err := a.CreateBackup(); err == nil || !reflect.DeepEqual(list, a.Backups()) {
		t.Fatal("creating a fourth backup changed the saved records")
	}
	if len(list) != MaxBackups {
		t.Fatal(len(list))
	}
	path, err := a.ExportBackupTo(list[0].ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other := testApp(t)
	if err := other.ImportBackup(path); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.config, other.config) {
		t.Fatal("incomplete settings roundtrip")
	}
	before, _ := json.Marshal(other.config)
	if err := other.ImportBackup("{}"); err == nil {
		t.Fatal("accepted invalid backup")
	}
	after, _ := json.Marshal(other.config)
	if !bytes.Equal(before, after) {
		t.Fatal("modified config")
	}
}

func TestFullSettingsTransfer(t *testing.T) {
	a := seedBackupApp(t)
	c := a.config
	id := c.Nodes[0].ID
	c.Selected = id
	c.Nodes[0].Source = "subscription"
	c.Subscriptions = []profile.Subscription{{ID: "subscription", Name: "test", URL: "https://feed.example/test", Password: "fixture-key", UserAgent: "Lanboat", Filter: "example", MaxNodes: 37, KeepRules: true, UDP: "true"}}
	c.RuleOverlays = []profile.RuleOverlay{{ID: "extra", Name: "test", SubscriptionID: "subscription", NodeIDs: []string{id}, Content: "[Rule]\nDOMAIN,extra.example,REJECT"}}
	c.Rules = []profile.Rule{{ID: "custom", Type: "domain", Value: "direct.example", Action: "direct"}}
	off, read := false, 8192
	s := &c.Settings
	s.Port, s.Mode, s.Language, s.Appearance = 32108, "tun", "en", "dark"
	s.AutoSelect, s.UpdateOnStart, s.ProxyLog, s.DNSLog, s.LogRetention = true, true, true, true, 17
	s.RuleAutoUpdate, s.RuleUpdateDays, s.RuleRegion = &off, 6, "us"
	s.SpeedURL, s.TestURL, s.TestTimeout, s.TestConcurrency = "https://speed.example/", "https://delay.example/test", 9, 7
	s.Proxy = profile.ProxySettings{AllowLAN: true, Authentication: true, Username: "user", Password: "local-secret", Upstream: profile.UpstreamProxy{Enabled: true, Type: "http", Server: "proxy.example", Port: 8081, Username: "upstream", Password: "upstream-secret"}}
	s.TCP = profile.TCPSettings{Fingerprint: "Firefox120", MaxRead: &read}
	s.UDP = profile.UDPSettings{Relay: &off, BlockSTUN: true, Ports: "53,443", Timeout: 43}
	s.DNS.ProxyRoute, s.DNS.ProxyNode, s.DNS.Policy = "node", id, "DOMAIN-SUFFIX,test.example,direct"
	s.DNS.FakeIP, s.DNS.Persist = true, true
	if err := a.commit(c); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "settings {export}")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"backup JSON", "archive", "old duplicate archive"} {
		t.Run(format, func(t *testing.T) {
			if format != "backup JSON" {
				if err := ruledata.Ensure(a.ruleDataDir()); err != nil {
					t.Fatal(err)
				}
			}
			b, err := a.CreateBackup()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = a.ExportBackupTo(b.ID, dir); err != nil {
				t.Fatal(err)
			}
			path, err := a.ExportBackupTo(b.ID, dir)
			if err != nil {
				t.Fatal(err)
			}
			if format != "backup JSON" && !strings.HasSuffix(path, "-2.tar.gz") {
				t.Fatal(path)
			}
			if format == "old duplicate archive" {
				old := strings.TrimSuffix(path, "-2.tar.gz") + ".tar-2.gz"
				if err = os.Rename(path, old); err != nil {
					t.Fatal(err)
				}
				path = old
			}
			target := testApp(t)
			if err := target.ImportBackup(path); err != nil {
				t.Fatal(err)
			}
			reopened, err := New(filepath.Dir(target.path), "", target.log, &Logs{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(c, reopened.config) {
				t.Fatal("persisted configuration differs after restore and restart")
			}
		})
	}
}
