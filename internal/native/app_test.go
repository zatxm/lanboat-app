package native

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
)

func testUI(t *testing.T) *UI {
	t.Helper()
	backend, err := desktop.New(t.TempDir(), "", zap.NewNop(), &desktop.Logs{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { backend.Shutdown(nil) })
	settings := backend.GetState().Config.Settings
	// Tests assert Chinese labels unless they explicitly switch language.
	settings.Language = "zh"
	if err := backend.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	u := newUI(backend, func() {})
	t.Cleanup(func() { close(u.done) })
	return u
}

func waitJob(t *testing.T, u *UI) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for (u.busy || u.testing || u.logPending || u.backingUp) && time.Now().Before(deadline) {
		u.drain()
		time.Sleep(time.Millisecond)
	}
	if u.busy || u.testing || u.logPending || u.backingUp {
		t.Fatal("UI job did not finish")
	}
}

func TestNodeEditorPreservesPassword(t *testing.T) {
	u := testUI(t)
	u.editNode(profile.Node{})
	u.field("name").SetText("local")
	u.field("server").SetText("localhost")
	u.field("password").SetText("private-secret")
	u.saveEditor()
	waitJob(t, u)
	if u.err != "" || u.editor != "" || len(u.state.Config.Nodes) != 1 {
		t.Fatalf("save failed: %s", u.err)
	}
	node := u.state.Config.Nodes[0]
	if node.Password != "" {
		t.Fatal("password exposed in UI state")
	}
	u.editNode(node)
	u.field("name").SetText("renamed")
	u.saveEditor()
	waitJob(t, u)
	uri, err := u.backend.ExportNode(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := profile.ParseURI(uri)
	if err != nil || decoded.Password != "private-secret" || decoded.Name != "renamed" {
		t.Fatal("edit lost credentials")
	}
}

func TestSaveErrorRetainsEditor(t *testing.T) {
	u := testUI(t)
	u.editNode(profile.Node{})
	u.field("name").SetText("bad")
	u.field("nodePort").SetText("invalid")
	u.saveEditor()
	if !strings.Contains(u.err, "数字") || u.busy {
		t.Fatal("invalid port started a job")
	}
	u.field("nodePort").SetText("8388")
	u.saveEditor()
	waitJob(t, u)
	if u.editor != "node" || u.err == "" || len(u.state.Config.Nodes) != 0 {
		t.Fatal("validation failure discarded editor or saved invalid node")
	}
}

func TestFilterAndPagination(t *testing.T) {
	nodes := []profile.Node{{Name: "香港", Server: "example.com", Method: "aes-256-gcm"}, {Name: "Other", Server: "localhost", Method: "aes-128-gcm"}}
	if len(filterNodes(nodes, "EXAMPLE")) != 1 || len(filterNodes(nodes, "香港")) != 1 || len(filterNodes(nodes, "missing")) != 0 {
		t.Fatal("filter mismatch")
	}
	for _, row := range []struct{ count, page, start, end, clamped int }{{0, 0, 0, 0, 0}, {40, 2, 0, 40, 0}, {41, 1, 40, 41, 1}, {1000, 99, 960, 1000, 24}} {
		start, end, page := pageBounds(row.count, row.page)
		if start != row.start || end != row.end || page != row.clamped {
			t.Fatalf("bad page: %+v", row)
		}
	}
}

func TestEditorClearsSecretsAndBusyPreventsDuplicateWork(t *testing.T) {
	u := testUI(t)
	u.field("password").SetText("secret")
	u.field("content").SetText("ss://secret")
	u.back()
	if u.field("password").Text() != "" || u.field("content").Text() != "" {
		t.Fatal("secret retained after closing editor")
	}
	gate := make(chan struct{})
	u.submit(func() error { <-gate; return nil }, nil)
	u.submit(func() error { t.Error("duplicate work ran"); return nil }, nil)
	close(gate)
	waitJob(t, u)
}

func TestBackPreservesDraftAndConfigComments(t *testing.T) {
	u := testUI(t)
	u.editNode(profile.Node{Type: "subscribe"})
	u.field("name").SetText("draft subscription")
	u.field("subURL").SetText("https://example.com/sub")
	u.openEditor("import")
	u.field("content").SetText("temporary private text")
	u.back()
	if u.editor != "node" || u.draft.Type != "subscribe" || u.field("name").Text() != "draft subscription" || u.field("subURL").Text() != "https://example.com/sub" {
		t.Fatal("back lost draft")
	}
	u.back()
	if u.editor != "" || len(u.history) != 0 || u.field("content").Text() != "" {
		t.Fatal("back retained child content")
	}
	input := "# Shadowrocket comment\n[Rule]\nDOMAIN,shadowrocket.example,DIRECT\n"
	output := displayConfigContent(input)
	if strings.Contains(output, "# Shadowrocket") || !strings.Contains(output, "DOMAIN,shadowrocket.example,DIRECT") {
		t.Fatal("display rename changed executable rule")
	}
}

func TestGroupTestsOnlyItsOwnNodes(t *testing.T) {
	u := testUI(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for _, name := range []string{"sub-one", "orphan", "local"} {
		if err := u.backend.SaveNode(profile.Node{Name: name, Server: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Method: "aes-256-gcm", Password: "fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	u.changeSettings(func(s *profile.Settings) { s.TestTimeout = 1 })
	waitJob(t, u)
	u.state = u.backend.GetState()
	nodes := u.state.Config.Nodes
	u.state.Config.Subscriptions = []profile.Subscription{{ID: "one"}, {ID: "two"}}
	u.state.Config.Nodes[0].Source = "one"
	u.state.Config.Nodes[1].Source = "two"
	u.testGroupNodes("one")
	waitJob(t, u)
	if len(u.delays) != 1 || u.delays[nodes[0].ID] != -1 {
		t.Fatal("subscription test crossed group boundary", u.delays)
	}
	u.delays = map[string]int64{}
	u.state.Config.Subscriptions = []profile.Subscription{{ID: "one"}}
	u.state.Config.Nodes[0].Source = "one"
	u.state.Config.Nodes[1].Source = "missing-subscription"
	u.testGroupNodes("")
	waitJob(t, u)
	if len(u.delays) != 2 || u.delays[nodes[1].ID] != -1 || u.delays[nodes[2].ID] != -1 {
		t.Fatal("local group did not include orphan and local nodes", u.delays)
	}
	if _, exists := u.delays[nodes[0].ID]; exists {
		t.Fatal("local test included a subscription node")
	}
}

func TestSettingsSaveKeepsNodeSnapshotAndErrorsKeepOldSettings(t *testing.T) {
	u := testUI(t)
	u.editNode(profile.Node{})
	u.field("name").SetText("fixture")
	u.field("server").SetText("localhost")
	u.field("password").SetText("private")
	u.saveEditor()
	waitJob(t, u)
	first := &u.state.Config.Nodes[0]
	u.dirty = false
	u.nodesDirty = false
	u.changeSettings(func(s *profile.Settings) { s.Mode = "global" })
	waitJob(t, u)
	if u.err != "" || &u.state.Config.Nodes[0] != first || u.dirty || u.nodesDirty {
		t.Fatal("setting fetched or rebuilt node state")
	}
	u.changeSettings(func(s *profile.Settings) { s.Port = 1 })
	waitJob(t, u)
	if u.err == "" || u.state.Config.Settings.Port == 1 || &u.state.Config.Nodes[0] != first || u.savingSettings {
		t.Fatal("failed save replaced settings or nodes")
	}
}
