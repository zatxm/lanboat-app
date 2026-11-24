package desktop

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestSubscriptionOptionsPersistRefreshAndAtomicFailure(t *testing.T) {
	a := testApp(t)
	fixture, err := os.ReadFile("../profile/testdata/karing-subscription.txt")
	if err != nil {
		t.Fatal(err)
	}
	content := string(fixture)

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.UserAgent() != "Provider-Agent/1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("subscription-encryption", "true")
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()
	sub := profile.Subscription{Name: "notes", URL: server.URL, Password: "decrypt-secret", UserAgent: "Provider-Agent/1", Insecure: "true", UDP: "false", FastOpen: "true", Filter: "hk", MaxNodes: 1, KeepRules: true}
	if count, err := a.SaveSubscriptionOptions(sub); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	c, err := profile.Load(a.path)
	if err != nil {
		t.Fatal(err)
	}
	stored := c.Subscriptions[0]
	if stored.Password != sub.Password || stored.Filter != sub.Filter || !stored.KeepRules || stored.UDP != "false" {
		t.Fatal("options not persisted")
	}
	if a.GetState().Config.Subscriptions[0].Password != "" {
		t.Fatal("password exposed in UI state")
	}
	if c.ActiveFile != "default" || len(c.Files) != 1 || len(c.RuleOverlays) != 1 || c.RuleOverlays[0].SubscriptionID != stored.ID || !strings.Contains(c.RuleOverlays[0].Content, "sub.lanboat.test") {
		t.Fatal("subscription rules not kept separately")
	}
	nodeID := c.Nodes[0].ID
	if _, err := a.RefreshSubscription(stored.ID); err != nil {
		t.Fatal(err)
	}
	if a.GetState().Config.Nodes[0].ID != nodeID {
		t.Fatal("refresh lost stable node ID")
	}
	for _, change := range []func(*profile.Subscription){func(s *profile.Subscription) { s.Filter = "nomatch" }, func(s *profile.Subscription) { s.Insecure = "false" }, func(s *profile.Subscription) { s.Password = "wrong-password" }} {
		s := stored
		change(&s)
		before, _ := os.ReadFile(a.path)
		if _, err := a.SaveSubscriptionOptions(s); err == nil {
			t.Fatal("invalid update accepted")
		}
		after, _ := os.ReadFile(a.path)
		if string(before) != string(after) {
			t.Fatal("failed update changed original")
		}
	}
	if err := a.CopyConfigFile(c.ActiveFile); err != nil {
		t.Fatal(err)
	}
	state := a.GetState()
	copy := state.Config.Files[1]
	if err := a.ActivateConfigFile(copy.ID); err != nil {
		t.Fatal(err)
	}
	stored.Password = "" // UI edits preserve the masked decryption password.
	stored.Name, stored.KeepRules = "new notes", false
	if _, err := a.SaveSubscriptionOptions(stored); err != nil {
		t.Fatal(err)
	}
	c = a.GetState().Config
	if len(c.Files) != 2 || len(c.RuleOverlays) != 0 || c.ActiveFile != copy.ID || c.Files[1] != copy || c.Files[0].ID != "default" {
		t.Fatal("disabling retained rules changed user's configuration")
	}
	if err := a.DeleteSubscription(stored.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.GetState().Config.Nodes) != 0 || len(a.GetState().Config.Files) != 2 {
		t.Fatal("subscription removal lost user copy")
	}
}

func TestSubscriptionSelectsUserAgentFromLink(t *testing.T) {
	app := testApp(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := r.URL.Query().Get("want"); !strings.HasPrefix(r.UserAgent(), want) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("socks5://proxy.example:1080#Test"))
	}))
	defer server.Close()

	flagged := server.URL + "/?flag=Shadowrocket&want=Shadowrocket%2F"
	if count, err := app.ImportSubscription(flagged, false); err != nil || count != 1 {
		t.Fatalf("import flagged link: count %d, error %v", count, err)
	}
	id := app.GetState().Config.Subscriptions[0].ID
	if count, err := app.RefreshSubscription(id); err != nil || count != 1 {
		t.Fatalf("refresh flagged link: count %d, error %v", count, err)
	}
	for _, test := range []struct{ query, agent string }{
		{"?want=Lanboat%2F", ""},
		{"?flag=shadowrocket&want=Provider-Agent", "Provider-Agent"},
	} {
		sub := profile.Subscription{Name: "test", URL: server.URL + "/" + test.query, UserAgent: test.agent}
		if count, err := app.SaveSubscriptionOptions(sub); err != nil || count != 1 {
			t.Fatalf("save subscription %s: count %d, error %v", test.query, count, err)
		}
	}
}

func TestClashMetaSubscriptionRetriesCompatibleEndpoint(t *testing.T) {
	app := testApp(t)
	agents := make(chan string, 3)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents <- r.UserAgent()
		if r.UserAgent() != "ClashMeta" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte("proxies:\n  - {name: Test, type: socks5, server: proxy.example, port: 1080}\n"))
	}))
	defer server.Close()
	app.client.Transport = server.Client().Transport
	address := server.URL + "/api/v1/client/subscribe?token=fixture"
	if count, err := app.ImportSubscription(address, false); err != nil || count != 1 {
		t.Fatalf("ClashMeta import: %d nodes, %v", count, err)
	}
	if len(agents) != 2 || !strings.HasPrefix(<-agents, "Lanboat/") || <-agents != "ClashMeta" {
		t.Fatal("expected fallback to ClashMeta on an HTTP 403")
	}
	if _, err := app.ImportSubscriptionOptions(profile.Subscription{URL: address, UserAgent: "Explicit"}); err == nil {
		t.Fatal("automatic fallback overrode the configured user agent")
	}
	if len(agents) != 1 || <-agents != "Explicit" {
		t.Fatal("explicit user agent triggered an automatic retry")
	}
}

// Saving a subscription with certificate checks enabled must negotiate HTTP/2
// just like importing the URL directly. The optional insecure path must keep it.
func TestSubscriptionSaveAndImportUseHTTP2(t *testing.T) {
	app := testApp(t)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			w.WriteHeader(http.StatusHTTPVersionNotSupported)
			return
		}
		_, _ = w.Write([]byte("socks5://proxy.example:1080#HTTP2"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	certs := x509.NewCertPool()
	certs.AddCert(server.Certificate())
	app.client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: certs}

	address := server.URL + "/subscription?flag=Shadowrocket"
	if count, err := app.ImportSubscription(address, false); err != nil || count != 1 {
		t.Fatalf("HTTP/2 import: count %d, error %v", count, err)
	}
	for _, insecure := range []string{"false", "true"} {
		sub := profile.Subscription{Name: "HTTP/2", URL: address, Insecure: insecure, UserAgent: "Shadowrocket/1.0"}
		if count, err := app.SaveSubscriptionOptions(sub); err != nil || count != 1 {
			t.Fatalf("HTTP/2 save, insecure %s: count %d, error %v", insecure, count, err)
		}
	}
}
