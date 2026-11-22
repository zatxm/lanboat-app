package desktop

import (
	"context"
	"github.com/goccy/go-json"
	"github.com/zatxm/lanboat-app/internal/profile"
	"os"
	"testing"
)

func TestConfiguredHysteriaLatency(t *testing.T) {
	path := os.Getenv("LANBOAT_TEST_HY2_NODE")
	if path == "" {
		t.Skip("private live fixture required")
	}
	var f struct {
		Node     profile.Node
		Settings profile.Settings
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	a := testApp(t)
	ms, err := a.testNodeContext(context.Background(), f.Node, f.Settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("HY2 protocol HTTP latency", ms, "ms")
}
