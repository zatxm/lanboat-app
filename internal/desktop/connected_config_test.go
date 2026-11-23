package desktop

import (
	"context"
	"github.com/zatxm/lanboat-app/internal/profile"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestConnectedEditsProtectOnlyActiveNode(t *testing.T) {
	a := autoSelectApp(t)
	upstream := httptest.NewServer(http.HandlerFunc(autoProxyTunnel))
	defer upstream.Close()
	n := autoHTTPNode("active", upstream.URL)
	other := n.Clone()
	other.ID = "other"
	a.config.Nodes = []profile.Node{n, other}
	a.config.Selected = n.ID
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	before := a.GetStatus()
	if before.StartedAt <= 0 {
		t.Fatal("connected session lacks a start time")
	}
	n.Name = "blocked"
	if err := a.SaveNode(n); err == nil {
		t.Fatal("active edit allowed")
	}
	if err := a.DeleteNode(n.ID); err == nil {
		t.Fatal("active delete allowed")
	}
	if err := a.DeleteLocalNodes(); err == nil || len(a.GetState().Config.Nodes) != 2 {
		t.Fatal("bulk delete must keep all nodes while a local node is active")
	}
	other.Name = "editable"
	if err := a.SaveNode(other); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveConfigFile("", "other.conf", "[Rule]\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	s := a.config.Settings
	s.TestTimeout = 4
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if after := a.GetStatus(); after.StartedAt != before.StartedAt {
		t.Fatal("unrelated setting restarted connection")
	}
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	s.Port = busy.Addr().(*net.TCPAddr).Port
	if err := a.SaveSettings(s); err == nil {
		t.Fatal("occupied port accepted")
	}
	if a.config.Settings.Port == s.Port || !a.GetStatus().Connected {
		t.Fatal("failed setting lost original connection")
	}
	s.Port = autoSelectPort(t)
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if after := a.GetStatus(); !after.Connected || after.StartedAt != before.StartedAt {
		t.Fatal("live port update reset the connection timer")
	}
	if err := a.SaveRules([]profile.Rule{{Type: "domain", Value: "fixture.invalid", Action: "reject"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SelectNode(other.ID); err != nil {
		t.Fatal(err)
	}
	if after := a.GetStatus(); after.ActiveNode != other.ID || after.StartedAt != before.StartedAt {
		t.Fatal("node selection failed or restarted connection timer")
	}
	if err := a.SaveNode(other); err != nil {
		t.Fatal("unchanged active node rejected", err)
	}
	n.Name = "now editable"
	if err := a.SaveNode(n); err != nil {
		t.Fatal(err)
	}
	if err := a.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if after := a.GetStatus(); after.Connected || after.StartedAt != 0 {
		t.Fatal("disconnect kept a running timer")
	}
}

func TestSubscriptionRefreshRetainsActiveNode(t *testing.T) {
	a := testApp(t)
	body := "hy2://new-password@127.0.0.1:443#updated\nss://YWVzLTI1Ni1nY206c2VjcmV0@127.0.0.1:8388#new"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	defer server.Close()
	a.client = server.Client()
	n := profile.Node{ID: "active", Type: "hysteria2", Name: "old", Server: "127.0.0.1", Port: 443, Password: "original", Source: "sub"}
	a.config.Nodes = []profile.Node{n}
	a.config.Selected = n.ID
	a.activeNode = n.ID
	a.connectionCtx, a.connectionCancel = context.WithCancel(context.Background())
	a.config.Subscriptions = []profile.Subscription{{ID: "sub", Name: "fixture", URL: server.URL}}
	if _, err := a.RefreshSubscription("sub"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, saved := range a.config.Nodes {
		if saved.ID == n.ID {
			found = true
			if !reflect.DeepEqual(saved, n) {
				t.Fatal("refresh modified active node")
			}
		}
	}
	if !found || len(a.config.Nodes) != 3 {
		t.Fatal("refresh did not retain active and update other nodes")
	}
}
