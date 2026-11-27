package native

import (
	"slices"

	"github.com/zatxm/fego/v26/ui"
	"github.com/zatxm/lanboat-app/internal/profile"
)

type mobileNodeEntry struct {
	title       string
	header      *ui.Element
	node        *profile.Node
	last, local bool
}

type visibleNodeRow struct {
	widgets  []*widget
	controls []control
}

// Keep the complete list searchable and scrollable while instantiating a
// handful of rows. A proxy subscription can contain thousands of nodes.
func (v *nativeView) buildMobileNodes(entries []mobileNodeEntry) {
	v.mobileRows = make(map[int]visibleNodeRow)
	v.mobileVisible = [2]int{-1, -1}
	v.mobileNodes = ui.VirtualList(ui.VirtualListOptions{
		Count: len(entries), EstimatedHeight: listRowHeight + 1,
		Name: func(i int) string {
			entry := entries[i]
			if entry.node == nil {
				return entry.title
			}
			name, _ := profile.SplitNodeFlag(entry.node.Name)
			return name
		},
	}, func(i int) *ui.Element {
		entry := entries[i]
		if entry.node == nil {
			return entry.header
		}
		var built visibleNodeRow
		controls := v.controls
		v.controls, v.buildingNode = nil, &built.widgets
		row, _ := v.homeNodeRow(*entry.node, entry.last, entry.local)
		// A plain row’s click handler is normally attached by pack.
		row.pointer.OnClick(row.activate)
		built.controls = v.controls
		v.controls, v.buildingNode = controls, nil
		v.mobileRows[i] = built
		v.refreshMobileNodeRow(built)
		return ui.Column(ui.Separator(), row.pointer).Gap(0)
	})
	v.nodeViewport.pointer.Children(v.mobileNodes.Height(0).MinHeight(0).Grow(1))
}

func (v *nativeView) pruneMobileNodeRows() {
	if v.mobileNodes == nil {
		return
	}
	first, last := v.mobileNodes.Visible()
	visible := [2]int{first, last}
	if visible == v.mobileVisible {
		return
	}
	v.mobileVisible = visible
	for i := range v.mobileRows {
		if i < first-2 || i > last+2 {
			delete(v.mobileRows, i)
		}
	}
	if first >= 0 && v.window != nil {
		requestIdle()
	}
}

func (v *nativeView) containsWidget(w *widget) bool {
	if v.widgets[w.id] == w {
		return true
	}
	for _, row := range v.mobileRows {
		if slices.Contains(row.widgets, w) {
			return true
		}
	}
	return false
}

func (v *nativeView) refreshMobileNodeRow(row visibleNodeRow) {
	u := v.ui
	for _, w := range row.widgets {
		w.setLoading(u.busy && w.id == u.busyControl)
		if w.update != nil && !u.busy {
			w.update()
		}
	}
	for _, c := range row.controls {
		c.widget.setEnabled(!u.busy && !c.disabled)
	}
}

func (v *nativeView) refreshMobileNodeRows() {
	if v.mobileNodes == nil {
		return
	}
	v.pruneMobileNodeRows()
	for _, row := range v.mobileRows {
		v.refreshMobileNodeRow(row)
	}
}
