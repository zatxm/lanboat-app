package desktop

import (
	"encoding/json"
	"github.com/zatxm/lanboat-app/internal/profile"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestConnectedDNSSettingsApplyAndStayPrivate(t *testing.T) {
	a := autoSelectApp(t)
	a.config.Settings.Mode = "direct"
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	s := a.GetState().Config.Settings
	d := profile.DefaultDNS()
	d.Cache = false
	d.ProxyRoute = "direct"
	s.DNS = &d
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	// Mutating a caller-owned settings value must not change the live configuration.
	d.Capacity = 1234
	if a.GetState().Config.Settings.DNS.Capacity != 4096 {
		t.Fatal("saved DNS aliases caller")
	}
	snapshot := a.GetState()
	snapshot.Config.Settings.DNS.Capacity = 5678
	if a.GetState().Config.Settings.DNS.Capacity != 4096 {
		t.Fatal("state DNS aliases live settings")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(a.path), "runtime", ".core-*.json"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	json.Unmarshal(raw, &config)
	dns := config["dns"].(map[string]any)
	if dns["disable_cache"] != true || dns["cache_capacity"].(float64) != 4096 {
		t.Fatal("DNS not applied to running core", dns)
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer target.Close()
	proxy, _ := url.Parse("http://127.0.0.1:" + strconv.Itoa(a.config.Settings.Port))
	client := http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}
	defer client.CloseIdleConnections()
	res, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatal(res.StatusCode)
	}
}
