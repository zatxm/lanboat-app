//go:build !fltk

package native

import (
	"fmt"
	"testing"

	"github.com/zatxm/fego"
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
		v.tester.Click(r.X+r.W/2, r.Y+r.H/2)
		v.tester.Step()
	}
	click(v.nav["settings"])
	if u.page != "settings" {
		t.Fatal("sidebar click did not navigate")
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
	for _, size := range [][2]int{{1160, 800}, {800, 560}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			u := testUI(t)
			for i := range 24 {
				id := fmt.Sprintf("node-%d", i)
				u.state.Config.Nodes = append(u.state.Config.Nodes, profile.Node{
					ID: id, Name: id, Type: "ss", Method: "aes-256-gcm",
				})
			}
			u.state.Config.Selected = u.state.Config.Nodes[0].ID
			v, err := newView(u)
			if err != nil {
				t.Fatal(err)
			}
			defer v.dispose()
			v.compact = size[0] < 940
			v.render()
			v.tester.Width, v.tester.Height = size[0], size[1]
			v.tester.Send(fego.Event{Kind: fego.Resize})
			scroll := v.scroll.pointer.Bounds()
			search := v.inputs["search"].pointer.Bounds()
			first := v.delayLabels["node-0"].pointer.Bounds()
			second := v.delayLabels["node-1"].pointer.Bounds()
			if scroll.Y > 1 || scroll.H < float32(size[1]-1) || search.Y > 150 ||
				first.Y <= search.Y || second.Y <= first.Y || second.Y+second.H > scroll.H {
				t.Fatalf("content does not use the viewport: scroll=%v search=%v first=%v second=%v", scroll, search, first, second)
			}
			var add *widget
			for _, w := range v.widgets {
				if w.kind == "button" && w.text == "添加节点" {
					add = w
					break
				}
			}
			if add == nil || add.buttonCaption != nil || add.icon != "add" {
				t.Fatal("expected icon-only Add node control")
			}
			if b := add.pointer.Bounds(); b.X+b.W > float32(size[0])-16 {
				t.Fatal("Add button extends beyond the content", b)
			}
			v.tester.Send(fego.Event{Kind: fego.Scroll, X: scroll.X + 20, Y: scroll.H / 2, DY: 240})
			if v.delayLabels["node-0"].pointer.Bounds().Y >= first.Y {
				t.Fatal("node list did not scroll")
			}
		})
	}
}
