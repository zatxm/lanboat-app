package native

import (
	"fmt"
	"testing"

	"github.com/zatxm/fego/v26"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestFegoNavigationAndInput(t *testing.T) {
	u := testUI(t)
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	v.tester.Step()
	click := func(w *widget) {
		r := w.pointer.Bounds()
		if r.W < 1 || r.H < 1 {
			t.Fatal("control has no bounds", w.text, r)
		}
		x, y := r.X+r.W/2, r.Y+r.H/2
		v.send(fego.Event{Kind: fego.PointerDown, X: x, Y: y, Button: 1})
		v.send(fego.Event{Kind: fego.PointerUp, X: x, Y: y, Button: 1})
		v.tester.Step()
	}
	settings := v.nav["settings"]
	r := settings.pointer.Bounds()
	x, y := int(r.X-2), int(r.Y+r.H/2)
	before := v.tester.Snapshot().RGBAAt(x, y)
	click(settings)
	if u.page != "settings" {
		t.Fatal("sidebar click did not navigate")
	}
	if got := v.tester.Snapshot().RGBAAt(x, y); got != before {
		t.Fatal("sidebar click left a focus ring outside the menu item", got, before)
	}
	u.editNode(profile.Node{Type: "ss"})
	v.flush()
	v.tester.Step()
	address := v.inputs["server"]
	click(address)
	v.tester.Type("example.org")
	if u.field("server").Text() != "example.org" {
		t.Fatal("typing did not reach node editor", address.pointer.Bounds())
	}
	u.popup = &popup{kind: "message", title: "Info", link: "test"}
	v.refresh()
	v.tester.Step()
	if v.popupRoot == nil {
		t.Fatal("missing dialog")
	}
	eventView{v.events, v}.Event(fego.Event{Kind: fego.KeyDown, Key: "Escape"})
	if u.popup != nil || v.popupRoot != nil {
		t.Fatal("Escape did not clear modal state")
	}
}

func TestFegoHomeUsesViewport(t *testing.T) {
	for _, size := range []struct {
		width, height int
		mobile        bool
	}{{1160, 800, false}, {800, 560, false}, {390, 620, true}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			u := testUI(t)
			u.state.Config.Subscriptions = []profile.Subscription{{ID: "feed", Name: "Feed"}}
			u.expanded["feed"] = true
			for i := range pageSize + 18 {
				id := fmt.Sprintf("node-%02d", i)
				n := profile.Node{
					ID: id, Name: id, Type: "ss", Method: "aes-256-gcm",
				}
				if i >= pageSize-2 {
					n.Source = "feed"
				}
				u.state.Config.Nodes = append(u.state.Config.Nodes, n)
			}
			u.state.Config.Selected = u.state.Config.Nodes[0].ID
			v, err := newView(u)
			if err != nil {
				t.Fatal(err)
			}
			defer v.dispose()
			v.compact = size.width < 940
			if size.mobile {
				v.mobile = true
				v.buildRoot()
			}
			v.render()
			v.tester.Width, v.tester.Height = size.width, size.height
			v.tester.Send(fego.Event{Kind: fego.Resize})
			scroll := v.nodeScroll.pointer.Bounds()
			search := v.inputs["search"].pointer.Bounds()
			power := v.power.pointer.Bounds()
			first := v.delayLabels["node-00"].pointer.Bounds()
			second := v.delayLabels["node-01"].pointer.Bounds()
			last := u.state.Config.Nodes[len(u.state.Config.Nodes)-1].ID
			if scroll.Y < search.Y+search.H || scroll.H < float32(size.height)/2 || search.Y > 150 ||
				first.Y < scroll.Y || second.Y <= first.Y || second.Y+second.H > scroll.Y+scroll.H {
				t.Fatalf("content does not use the viewport: scroll=%v search=%v first=%v second=%v", scroll, search, first, second)
			}
			if len(v.delayLabels) != len(u.state.Config.Nodes) {
				t.Fatalf("node list is incomplete: %d nodes", len(v.delayLabels))
			}
			var add *widget
			for _, w := range v.widgets {
				if hasClass(w.classes, "page-bar") {
					t.Fatal("home has pagination")
				}
				if w.kind == "button" && w.text == "添加节点" {
					add = w
				}
			}
			if add == nil || add.buttonCaption != nil || add.icon != "add" {
				t.Fatal("expected icon-only Add node control")
			}
			if b := add.pointer.Bounds(); b.X+b.W > scroll.X+scroll.W {
				t.Fatal("Add button extends beyond the content", b)
			}
			v.tester.Send(fego.Event{Kind: fego.Scroll, X: scroll.X + 20, Y: scroll.Y + scroll.H/2, DY: 240})
			if v.delayLabels["node-00"].pointer.Bounds().Y >= first.Y {
				t.Fatal("node list did not scroll")
			}
			// Clicking the scrollbar track below its thumb must reach the bottom.
			v.tester.Click(scroll.X+scroll.W-4, scroll.Y+scroll.H-2)
			bottom := v.delayLabels[last].pointer.Bounds()
			if bottom.Y < scroll.Y || bottom.Y+bottom.H > scroll.Y+scroll.H {
				t.Fatal("last subscription node is outside the viewport", bottom, scroll)
			}
			if got := v.power.pointer.Bounds(); got != power {
				t.Fatal("connection toolbar moved while scrolling", power, got)
			}
			if got := v.inputs["search"].pointer.Bounds(); got != search {
				t.Fatal("search box moved while scrolling", search, got)
			}
			v.inputs["search"].edit(last)
			v.inputs["search"].dispatch("submit", "")
			v.tester.Step()
			if len(v.delayLabels) != 1 || v.delayLabels[last] == nil || v.nodeScroll.scrollPosition(-1) != 0 {
				t.Fatal("search did not filter all nodes and return the list to the top")
			}
		})
	}
}

func TestFegoHomeRendersNodeFlagOutsideName(t *testing.T) {
	u := testUI(t)
	const name = "🇭🇰 【亚洲】香港01 | V6"
	const host = "192.0.2.1"
	u.state.Config.Nodes = []profile.Node{{ID: "hk", Name: name, Server: host, Type: "trojan"}}
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()
	v.render()
	var title, flag *widget
	for _, w := range v.widgets {
		if hasClass(w.classes, "node-title") {
			title = w
		}
		if hasClass(w.classes, "country-flag") {
			flag = w
		}
	}
	if title == nil || title.pointer.Text() != "【亚洲】香港01 | V6" {
		t.Fatal("node title should show the remark without the flag glyphs")
	}
	if flag == nil || flag.value != "hk" {
		t.Fatal("node title's flag should be used when the server region is unknown")
	}
	u.flags[host] = flagEntry{code: "jp"}
	v.refresh()
	if flag.value != "jp" || u.state.Config.Nodes[0].Name != name {
		t.Fatal("GeoIP should control the icon without changing the original remark")
	}
}
