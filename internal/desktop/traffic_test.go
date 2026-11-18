package desktop

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
	"io"
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

func TestIndependentTrafficSettingsAndClear(t *testing.T) {
	core := os.Getenv("LANBOAT_TEST_CORE")
	if core == "" {
		t.Skip("native core required")
	}
	dir := t.TempDir()
	a, err := New(dir, core, zap.NewNop(), &Logs{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Shutdown(nil)
	c := a.GetState().Config
	s := c.Settings
	s.Mode = "direct"
	s.Port = autoSelectPort(t)
	s.ProxyLog = true
	s.DNSLog = true
	if err = a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if err = a.Connect(); err != nil {
		t.Fatal(err)
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") }))
	defer origin.Close()
	proxy, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(s.Port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 5 * time.Second}
	request := func() {
		t.Helper()
		r, err := client.Get(origin.URL)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}
	request()
	_, err = a.DNSLookup("localhost")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		p, _ := a.NetworkLogs("proxy", 0)
		if p.Total > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("proxy traffic not recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	before, _ := filepath.Glob(filepath.Join(dir, "runtime", ".core-*.json"))
	s.ProxyLog = false
	if err = a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	after, _ := filepath.Glob(filepath.Join(dir, "runtime", ".core-*.json"))
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatal("recording toggle restarted connection")
	}
	p, _ := a.NetworkLogs("proxy", 0)
	request()
	time.Sleep(50 * time.Millisecond)
	p2, _ := a.NetworkLogs("proxy", 0)
	d, _ := a.NetworkLogs("dns", 0)
	if p.Total != p2.Total || d.Total == 0 {
		t.Fatal("recording not independent")
	}
	loaded, err := profile.Load(a.path)
	if err != nil || loaded.Settings.ProxyLog || !loaded.Settings.DNSLog {
		t.Fatal("logging settings not persisted", err)
	}
	if err = a.ClearLogs(); err != nil {
		t.Fatal(err)
	}
	p, _ = a.NetworkLogs("proxy", 0)
	d, _ = a.NetworkLogs("dns", 0)
	if p.Total != 0 || d.Total != 0 {
		t.Fatal("clear left logs")
	}
}
