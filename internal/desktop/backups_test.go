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
func TestBackupsRestoreOnlyTheirCategory(t *testing.T) {
	for _, kind := range []string{"nodes", "routing", "dns"} {
		t.Run(kind, func(t *testing.T) {
			a := seedBackupApp(t)
			original := a.GetState().Config
			info, err := a.CreateBackup(kind)
			if err != nil || info.Kind != kind {
				t.Fatal(info, err)
			}
			snapshot, err := a.readBackup(info.ID)
			if err != nil || snapshot.validate() != nil {
				t.Fatal(err)
			}
			if info.Kind != "nodes" && len(snapshot.Nodes) != 0 || info.Kind != "routing" && snapshot.Routing != nil || info.Kind != "dns" && snapshot.DNS != nil {
				t.Fatal("mixed-category backup")
			}
			if err = a.SaveNode(profile.Node{Name: "later", Server: "later.test", Port: 443, Method: "aes-256-gcm", Password: "later"}); err != nil {
				t.Fatal(err)
			}
			if err = a.ActivateConfigFile("default"); err != nil {
				t.Fatal(err)
			}
			s := a.GetState().Config.Settings
			d := *s.DNS
			d.Capacity = 8192
			s.DNS = &d
			if err = a.SaveSettings(s); err != nil {
				t.Fatal(err)
			}
			before := a.GetState().Config
			if err = a.RestoreBackup(info.ID); err != nil {
				t.Fatal(err)
			}
			after := a.GetState().Config
			switch kind {
			case "nodes":
				if len(after.Nodes) != 1 || after.Nodes[0].Name != "original" || after.ActiveFile != before.ActiveFile || !reflect.DeepEqual(after.Settings.DNS, before.Settings.DNS) {
					t.Fatal("node restore touched other categories")
				}
				stored, _ := profile.Load(a.path)
				if stored.Nodes[0].Password != "private" {
					t.Fatal("credentials lost")
				}
			case "routing":
				if after.ActiveFile != original.ActiveFile || !reflect.DeepEqual(after.Nodes, before.Nodes) || !reflect.DeepEqual(after.Settings.DNS, before.Settings.DNS) {
					t.Fatal("routing restore touched other categories")
				}
			case "dns":
				if after.Settings.DNS.Capacity != 2048 || !reflect.DeepEqual(after.Nodes, before.Nodes) || after.ActiveFile != before.ActiveFile {
					t.Fatal("DNS restore touched other categories")
				}
			}
		})
	}
}
func TestBackupRetentionIsPerCategory(t *testing.T) {
	a := seedBackupApp(t)
	for _, kind := range []string{"nodes", "routing", "dns"} {
		for i := 0; i < MaxBackups+1; i++ {
			if _, err := a.CreateBackup(kind); err != nil {
				t.Fatal(err)
			}
		}
	}
	list := a.Backups()
	if len(list) != 3*MaxBackups {
		t.Fatal(len(list))
	}
	counts := map[string]int{}
	for _, b := range list {
		counts[b.Kind]++
	}
	for _, n := range counts {
		if n != MaxBackups {
			t.Fatal(counts)
		}
	}
}
func TestBackupExportAndValidation(t *testing.T) {
	a := seedBackupApp(t)
	info, err := a.CreateBackup("dns")
	if err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(filepath.Join(backupDir(a.path), info.ID))
	if stat.Mode().Perm() != 0600 {
		t.Fatal("backup permissions")
	}
	path, err := a.ExportBackupTo(info.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if err = a.RestoreBackup(info.ID); err != nil {
		t.Fatal(err)
	}
	if err = a.DeleteBackup(info.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("export changed")
	}
	for _, id := range []string{"", "../config.json", "backup-x.json/../config.json", "config.json"} {
		if a.DeleteBackup(id) == nil {
			t.Fatal("unsafe delete", id)
		}
		if a.RestoreBackup(id) == nil {
			t.Fatal("unsafe restore", id)
		}
	}
	for i, b := range []Backup{{Version: 1, Kind: "nodes"}, {Version: 2, Kind: "unknown"}, {Version: 2, Kind: "dns"}, {Version: 2, Kind: "routing"}, {Version: 2, Kind: "nodes", DNS: func() *profile.DNSSettings { d := profile.DefaultDNS(); return &d }()}} {
		b.CreatedAt = time.Now()
		raw, _ := json.Marshal(b)
		p := filepath.Join(backupDir(a.path), fmt.Sprintf("backup-bad-%d.json", i))
		os.WriteFile(p, raw, 0600)
		before, _ := os.ReadFile(a.path)
		if a.RestoreBackup(filepath.Base(p)) == nil {
			t.Fatal("invalid snapshot accepted", i)
		}
		now, _ := os.ReadFile(a.path)
		if !bytes.Equal(before, now) {
			t.Fatal("failed restore changed profile")
		}
	}
}
func TestNodeBackupIncludesSubscriptions(t *testing.T) {
	a := seedBackupApp(t)
	a.mu.Lock()
	a.config.Subscriptions = []profile.Subscription{{ID: "sub", Name: "fixture", URL: "https://example.test/sub"}}
	a.mu.Unlock()
	b, err := a.CreateBackup("nodes")
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.config.Subscriptions = nil
	a.mu.Unlock()
	if err = a.RestoreBackup(b.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.GetState().Config.Subscriptions) != 1 {
		t.Fatal("subscription lost")
	}
}

func TestRoutingBackupRejectsLegacyJSON(t *testing.T) {
	a := testApp(t)
	b := Backup{Version: 2, Kind: "routing", CreatedAt: time.Now(), Routing: &BackupRouting{Name: "default.conf", Content: "[Rule]\nFINAL,DIRECT"}}
	raw, _ := json.Marshal(b)
	if err := os.MkdirAll(backupDir(a.path), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(backupDir(a.path), "backup-routing-old.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.RestoreBackup(filepath.Base(path)); err == nil {
		t.Fatal("accepted deprecated JSON routing backup")
	}
	if len(a.Backups()) != 0 {
		t.Fatal("deprecated routing backup listed")
	}
	a.pruneBackups("routing")
	if _, err := os.Stat(path); err != nil {
		t.Fatal("deprecated backup was deleted", err)
	}
}

func TestRoutingArchiveRestoresAnOlderDefaultExactly(t *testing.T) {
	a := testApp(t)
	b := Backup{Version: 3, Kind: "routing", CreatedAt: time.Now(), Routing: &BackupRouting{Name: "default.conf", Content: "[Rule]\nDOMAIN,old-default.test,REJECT\nFINAL,DIRECT"}}
	if err := os.MkdirAll(backupDir(a.path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ruledata.Ensure(a.ruleDataDir()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(backupDir(a.path), "backup-routing-old.tar.gz")
	if err := writeRoutingBackup(path, b, a.ruleDataDir()); err != nil {
		t.Fatal(err)
	}
	if err := a.RestoreBackup(filepath.Base(path)); err != nil {
		t.Fatal(err)
	}
	c := a.GetState().Config
	if c.ActiveFile == "default" {
		t.Fatal("backup content replaced by today's bundled default")
	}
	for _, f := range c.Files {
		if f.ID == c.ActiveFile && f.Content != b.Routing.Content {
			t.Fatal("backup content changed")
		}
	}
}
func TestNodeRestoreDoesNotSilentlyResetDNSOutbound(t *testing.T) {
	a := seedBackupApp(t)
	snapshot, err := a.CreateBackup("nodes")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.SaveNode(profile.Node{Name: "dns-node", Server: "dns.test", Port: 443, Method: "aes-256-gcm", Password: "private"}); err != nil {
		t.Fatal(err)
	}
	c := a.GetState().Config
	d := profile.DefaultDNS()
	d.ProxyRoute = "node"
	d.ProxyNode = c.Nodes[len(c.Nodes)-1].ID
	s := c.Settings
	s.DNS = &d
	if err = a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(a.path)
	if a.RestoreBackup(snapshot.ID) == nil {
		t.Fatal("lost DNS outbound node")
	}
	after, _ := os.ReadFile(a.path)
	if !bytes.Equal(before, after) {
		t.Fatal("failed restore changed data")
	}
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
	info, err := a.CreateBackup("routing")
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
	if b.Version != 3 || active != filepath.Base(second) {
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
	info, err := a.CreateBackup("routing")
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
	info, err := a.CreateBackup("routing")
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

func TestNodeBackupRestoresSourceRules(t *testing.T) {
	a := seedBackupApp(t)
	a.mu.Lock()
	node := a.config.Nodes[0]
	a.config.RuleOverlays = []profile.RuleOverlay{{ID: "import", Name: "local", NodeIDs: []string{node.ID}, Content: "[Rule]\nDOMAIN,backup.test,REJECT"}}
	a.mu.Unlock()
	info, err := a.CreateBackup("nodes")
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.config.RuleOverlays = nil
	a.mu.Unlock()
	if err = a.RestoreBackup(info.ID); err != nil {
		t.Fatal(err)
	}
	restored := a.GetState().Config
	if len(restored.RuleOverlays) != 1 || restored.RuleOverlays[0].NodeIDs[0] != node.ID {
		t.Fatal("node source rules not restored")
	}
	p, err := profile.EffectiveRouting(restored)
	if err != nil || p.Rules[0].Value != "backup.test" {
		t.Fatal(p, err)
	}
}
