package native

import (
	"fmt"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestLargeMobileNodeListKeepsOnlyVisibleWidgets(t *testing.T) {
	u := testUI(t)
	u.state.Config.Nodes = make([]profile.Node, 2500)
	for i := range u.state.Config.Nodes {
		u.state.Config.Nodes[i] = profile.Node{
			ID: fmt.Sprintf("node-%d", i), Name: fmt.Sprintf("Node %04d", i),
			Type: "socks5", Server: "127.0.0.1", Port: 1080,
		}
	}
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	v.mobile, v.compact = true, true
	defer v.dispose()
	v.render()
	v.tester.Step()
	list := v.mobileNodes
	if list == nil {
		t.Fatal("large mobile list should use Fego virtualization")
	}
	first, last := list.Visible()
	if first != 0 || last <= first || last > 40 || len(v.mobileRows) > 44 {
		t.Fatalf("initial visible rows %d–%d, %d live node rows", first, last, len(v.mobileRows))
	}
	list.ScrollToEnd()
	v.tester.Step()
	v.pruneMobileNodeRows()
	_, last = list.Visible()
	if last != len(u.state.Config.Nodes) || len(v.mobileRows) > 44 {
		t.Fatalf("last visible row %d, %d live node rows", last, len(v.mobileRows))
	}
	for _, c := range v.mobileRows[last].controls {
		if c.widget.text == "节点详情" {
			c.widget.click()
			if u.draft.ID != "node-2499" {
				t.Fatalf("opened the wrong node: %s", u.draft.ID)
			}
			return
		}
	}
	t.Fatal("the last row lost its details action")
}
