package desktop

import (
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
