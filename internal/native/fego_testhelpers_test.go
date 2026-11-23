package native

import (
	"slices"
	"testing"

	"github.com/zatxm/fego/v26"
	"github.com/zatxm/fego/v26/ui"
)

type pagerHost struct {
	*ui.Tester
	nodes []fego.AccessNode
}

func (h *pagerHost) SetAccessibility(nodes []fego.AccessNode) { h.nodes = slices.Clone(nodes) }

// Click the actual Fego navigation button, including when the list ends off-screen.
func pageClicker(t *testing.T, v *nativeView) func(string) {
	t.Helper()
	tester := &ui.Tester{Document: v.doc, Width: 1160, Height: 800, Scale: 1}
	host := &pagerHost{Tester: tester}
	v.tester = tester
	v.doc.Mount(host)
	tester.Send(fego.Event{Kind: fego.Resize})
	return func(label string) {
		t.Helper()
		v.scroll.scrollPosition(1 << 20)
		tester.Step()
		for _, item := range host.nodes {
			if item.Label == label && !item.Disabled {
				p := item.Bounds.Min.Add(item.Bounds.Size().Div(2))
				tester.Click(float32(p.X), float32(p.Y))
				return
			}
		}
		t.Fatalf("no enabled page control %q", label)
	}
}

func (w *widget) click() { w.dispatch("click", "") }
func (w *widget) edit(s string) {
	if w.readonly {
		return
	}
	if w.kind != "select" {
		w.setValue(s)
	}
	w.dispatch("change", s)
}
func (w *widget) hasFocus() bool { return w.view.tester != nil && w.view.tester.Editing }
