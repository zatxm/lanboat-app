package desktop

import (
	"encoding/base64"
	"fmt"
	"github.com/zatxm/lanboat-app/internal/profile"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestImportURLsAtomicallyAndRetainsRules(t *testing.T) {
	a := testApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, "proxies:\n  - {name: node, type: ss, server: 8.8.8.8, port: 443, cipher: aes-256-gcm, password: secret}\nrules:\n  - DOMAIN,retained.example,DIRECT\n")
	}))
	defer server.Close()
	if _, err := a.Import(server.URL + "/good\n" + server.URL + "/bad"); err == nil {
		t.Fatal("expected failed import")
	}
	if len(a.GetState().Config.Nodes) != 0 {
		t.Fatal("partial import")
	}
	count, err := a.Import(server.URL + "/good\nss://YWVzLTI1Ni1nY206cA@127.0.0.1:443#🇯🇵%20local")
	if err != nil || count != 2 {
		t.Fatal(count, err)
	}
	c := a.GetState().Config
	if len(c.Subscriptions) != 1 || !c.Subscriptions[0].KeepRules || len(c.Nodes) != 2 || c.Nodes[0].Source != c.Subscriptions[0].ID || c.Nodes[1].Source != "" {
		t.Fatal("incorrect import categories")
	}
	saved, err := profile.Load(a.path)
	if err != nil || saved.Nodes[0].Flag != "🇺🇸" || saved.Nodes[1].Flag != "🇯🇵" {
		t.Fatal("import did not persist the IP country and the supplied flag", err)
	}
	found := false
	for _, f := range c.RuleOverlays {
		if strings.Contains(f.Content, "DOMAIN,retained.example,DIRECT") {
			found = true
		}
	}
	if !found {
		t.Fatal("subscription routing rules lost")
	}
}

func TestImportKaringLink(t *testing.T) {
	a := testApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ss://YWVzLTI1Ni1nY206cA@example.org:443#node")
	}))
	defer server.Close()
	link := "karing://install-config?name=Example&url=" + url.QueryEscape(server.URL)
	count, err := a.Import(link)
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	c := a.GetState().Config
	if len(c.Subscriptions) != 1 || c.Subscriptions[0].Name != "Example" || c.Subscriptions[0].URL != server.URL {
		t.Fatal(c.Subscriptions)
	}
}

func TestImportRemoteProfileSchemes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ss://YWVzLTI1Ni1nY206cA@example.org:443#node")
	}))
	defer server.Close()
	for _, prefix := range []string{"stash://install-config?url=", "sing-box://import-remote-profile?url="} {
		a := testApp(t)
		count, err := a.Import(prefix + url.QueryEscape(server.URL) + "#Example")
		if err != nil || count != 1 || a.GetState().Config.Subscriptions[0].Name != "Example" {
			t.Fatal(count, err)
		}
	}
}

func TestImportClashProxyProviderAndRefresh(t *testing.T) {
	a := testApp(t)
	var version atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/nodes" {
			if r.Header.Get("Authorization") != "Bearer provider-key" {
				http.Error(w, "missing provider authentication", http.StatusUnauthorized)
				return
			}
			if version.Load() == 0 {
				fmt.Fprint(w, "proxies:\n  - {name: HK, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}\n")
			} else {
				fmt.Fprint(w, "proxies:\n  - {name: JP, type: ss, server: example.org, port: 443, cipher: aes-256-gcm, password: secret}\n")
			}
			return
		}
		fmt.Fprintf(w, `proxy-providers:
  provider:
    type: http
    url: %q
    header: {Authorization: 'Bearer provider-key'}
    health-check: {enable: true, interval: 300}
    override: {additional-prefix: ignored-}
    dialer-proxy: another-proxy
    proxy: another-proxy
    size-limit: 8
    vendor-extension: ignored
proxy-groups:
  - {name: route, type: select, use: [provider]}
rules:
  - DOMAIN,example.net,route
`, "http://"+r.Host+"/nodes")
	}))
	defer server.Close()
	count, err := a.Import(server.URL)
	if err != nil || count != 1 {
		t.Fatal(count, err)
	}
	c := a.GetState().Config
	if c.Nodes[0].Name != "HK" || len(c.RuleOverlays) != 1 || len(c.PolicyGroups) != 1 ||
		!strings.Contains(c.RuleOverlays[0].Content, c.PolicyGroups[0].Ref()) || c.PolicyGroups[0].Members[0] != "node:"+c.Nodes[0].ID {
		t.Fatal(c.Nodes, c.RuleOverlays)
	}
	ref := c.PolicyGroups[0].Ref()
	version.Store(1)
	if count, err = a.RefreshSubscription(c.Subscriptions[0].ID); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	c = a.GetState().Config
	if c.Nodes[0].Name != "JP" || len(c.RuleOverlays) != 1 || len(c.PolicyGroups) != 1 || c.PolicyGroups[0].Ref() != ref ||
		!strings.Contains(c.RuleOverlays[0].Content, ref) || c.PolicyGroups[0].Members[0] != "node:"+c.Nodes[0].ID {
		t.Fatal(c.Nodes, c.RuleOverlays)
	}
}

func TestClashProxyProviderRejectsInvalidHeaders(t *testing.T) {
	a := testApp(t)
	content := `proxy-providers:
  provider:
    type: http
    url: https://example.com/nodes
    header:
      "Bad Header": injected
proxy-groups:
  - {name: route, type: select, use: [provider]}`
	if _, err := a.expandClashProviders(content, profile.Subscription{}); err == nil || !strings.Contains(err.Error(), "header") {
		t.Fatal("invalid provider request header accepted", err)
	}
}

func TestClashProxyProviderInheritsSubscriptionDecryptionPassword(t *testing.T) {
	a := testApp(t)
	encrypted, err := os.ReadFile("../profile/testdata/karing-subscription.txt")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/provider" {
			w.Header().Set("subscription-encryption", "true")
			_, _ = w.Write(encrypted)
			return
		}
		fmt.Fprintf(w, "proxy-providers:\n  provider: {type: http, url: %q}\nproxy-groups:\n  - {name: route, type: select, use: [provider]}\nrules:\n  - DOMAIN,example.net,route\n", "http://"+r.Host+"/provider")
	}))
	defer server.Close()
	count, err := a.SaveSubscriptionOptions(profile.Subscription{Name: "encrypted provider", URL: server.URL, Password: "decrypt-secret"})
	if err != nil || count != 2 || len(a.GetState().Config.Nodes) != 2 {
		t.Fatal(count, a.GetState().Config.Nodes, err)
	}
}

func TestImportMultipleBase64Lists(t *testing.T) {
	a := testApp(t)
	x := base64.StdEncoding.EncodeToString([]byte("ss://YWVzLTI1Ni1nY206cA@example.org:443#one"))
	y := base64.StdEncoding.EncodeToString([]byte("ss://YWVzLTI1Ni1nY206cA@example.net:443#two"))
	count, err := a.Import(x + "\n" + y)
	if err != nil || count != 2 {
		t.Fatal(count, err)
	}
}

func TestImportedRulesApplyOnlyToOwnedNodes(t *testing.T) {
	a := testApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		domain := "one.test"
		action := "DIRECT"
		if r.URL.Path == "/two" {
			domain = "two.test"
			action = "PROXY"
		}
		fmt.Fprintf(w, "proxies:\n  - {name: SameName, type: ss, server: example.com, port: 443, cipher: aes-256-gcm, password: secret}\nrules:\n  - DOMAIN,%s,%s\n", domain, action)
	}))
	defer server.Close()
	if _, err := a.Import(server.URL + "/one\n" + server.URL + "/two"); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveConfigFile("", "system.conf", "[Rule]\nDOMAIN,one.test,REJECT\nDOMAIN,two.test,REJECT\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	c := a.GetState().Config
	if err := a.ActivateConfigFile(c.Files[1].ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		index      int
		host, want string
	}{{0, "one.test", "DIRECT"}, {0, "two.test", "REJECT"}, {1, "one.test", "REJECT"}, {1, "two.test", "PROXY"}} {
		if err := a.SelectNode(c.Nodes[tc.index].ID); err != nil {
			t.Fatal(err)
		}
		got, err := a.TestRule(tc.host, "")
		if err != nil || !strings.HasPrefix(got, tc.want) {
			t.Fatalf("%+v got %q: %v", tc, got, err)
		}
	}
	before := a.GetState().Config
	if err := a.DeleteSubscription(before.Subscriptions[0].ID); err != nil {
		t.Fatal(err)
	}
	after := a.GetState().Config
	if len(after.RuleOverlays) != 1 || after.RuleOverlays[0].SubscriptionID != before.Subscriptions[1].ID || after.ActiveFile != before.ActiveFile {
		t.Fatal("deleting subscription changed unrelated rules")
	}
}

func TestNodeSourceRulesSwitchWhileConnected(t *testing.T) {
	a := autoSelectApp(t)
	upstream := httptest.NewServer(http.HandlerFunc(autoProxyTunnel))
	defer upstream.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer origin.Close()
	one, two := autoHTTPNode("one", upstream.URL), autoHTTPNode("two", upstream.URL)
	a.config.Nodes = []profile.Node{one, two}
	a.config.Selected = one.ID
	a.config.Settings.Mode = "rule"
	a.config.Files = []profile.ConfigFile{{ID: "default", Name: "default.conf", Content: "[Rule]\nFINAL,DIRECT"}}
	a.config.ActiveFile = "default"
	a.config.RuleOverlays = []profile.RuleOverlay{{ID: "one", NodeIDs: []string{one.ID}, Content: "[Rule]\nIP-CIDR,127.0.0.1/32,DIRECT"}, {ID: "two", NodeIDs: []string{two.ID}, Content: "[Rule]\nIP-CIDR,127.0.0.1/32,REJECT"}}
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", a.config.Settings.Port))
	request := func() (string, error) {
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}}
		resp, err := client.Get(origin.URL)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		return string(data), err
	}
	if body, err := request(); err != nil || body != "ok" {
		t.Fatal(body, err)
	}
	if err := a.SelectNode(two.ID); err != nil {
		t.Fatal(err)
	}
	if body, err := request(); err == nil && body == "ok" {
		t.Fatal("selected node inherited the previous source's direct rule")
	}
	if err := a.SelectNode(one.ID); err != nil {
		t.Fatal(err)
	}
	if body, err := request(); err != nil || body != "ok" {
		t.Fatal(body, err)
	}
}

func TestImportedCombinedRuleRealRouting(t *testing.T) {
	a := autoSelectApp(t)
	upstream := httptest.NewServer(http.HandlerFunc(autoProxyTunnel))
	defer upstream.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer origin.Close()
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") }))
	defer other.Close()
	u, _ := url.Parse(origin.URL)
	content := fmt.Sprintf(`{"outbounds":[{"type":"shadowsocks","tag":"node","server":"example.com","server_port":443,"method":"aes-256-gcm","password":"secret"}],"route":{"rules":[{"ip_cidr":["127.0.0.1/32"],"network":"tcp","port":%s,"outbound":"block"}]}}`, u.Port())
	_, overlay, err := profile.ParseImportWithRules(content, "[Rule]\n")
	if err != nil {
		t.Fatal(err)
	}
	node := autoHTTPNode("node", upstream.URL)
	a.config.Nodes = []profile.Node{node}
	a.config.Selected = node.ID
	a.config.Settings.Mode = "rule"
	a.config.Files = []profile.ConfigFile{{ID: "default", Name: "default.conf", Content: "[Rule]\nFINAL,DIRECT"}}
	a.config.ActiveFile = "default"
	a.config.RuleOverlays = []profile.RuleOverlay{{ID: "source", NodeIDs: []string{node.ID}, Content: overlay}}
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", a.config.Settings.Port))
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}}
	if response, err := client.Get(origin.URL); err == nil {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode == http.StatusOK && string(body) == "ok" {
			t.Fatal("matching combined rule did not reject")
		}
	}
	response, err := client.Get(other.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "ok" {
		t.Fatal(string(data), err)
	}
}

func TestImportExportNodeBundle(t *testing.T) {
	a := seedBackupApp(t)
	a.config.Subscriptions = []profile.Subscription{{ID: "sub1", Name: "my feed", URL: "https://example.com/feed"}}
	a.config.Nodes[0].Source = "sub1"
	a.config.Nodes[0].Flag = "🇯🇵"
	a.config.RuleOverlays = []profile.RuleOverlay{{ID: "sub1", Name: "custom", SubscriptionID: "sub1", NodeIDs: []string{a.config.Nodes[0].ID}, Content: "[Rule]\nFINAL,DIRECT"}}
	path, err := a.ExportDataTo("nodes", "", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := testApp(t)
	for range 2 {
		if _, err := b.Import(path); err != nil {
			t.Fatal(err)
		}
	}
	if len(b.config.Nodes) != 1 || b.config.Nodes[0].Source != "sub1" || b.config.Nodes[0].Flag != "🇯🇵" || b.config.Subscriptions[0] != a.config.Subscriptions[0] || b.config.Selected != a.config.Selected || len(b.config.RuleOverlays) != 1 || len(b.config.Files) != 1 {
		t.Fatal("node transfer lost associations or imported global settings")
	}
}
