package native

import (
	"strconv"

	"github.com/zatxm/fego/v26/ui"
)

// Desktop lists index pages from zero; Fego numbers them from one.
func (v *nativeView) pageBar(page, pages, total int, change func(int)) *widget {
	bar := v.box("page-bar", false, 0, false)
	bar.content.FillWidth().Append(ui.PaginationWithTotal(page+1, pages, total, func(next int) { change(next - 1) }))
	v.controls = append(v.controls, control{widget: bar})
	return bar
}

func (v *nativeView) listRange(key string, total int) (int, int, *widget) {
	u := v.ui
	n, _ := strconv.Atoi(u.field("page." + key).Text())
	start, end, n := pageBounds(total, n)
	u.field("page." + key).SetText(strconv.Itoa(n))
	var pager *widget
	if total > pageSize {
		pager = v.pageBar(n, (total+pageSize-1)/pageSize, total, func(next int) {
			u.field("page." + key).SetText(strconv.Itoa(next))
			u.resetScroll = true
			u.render()
		})
	}
	return start, end, pager
}

// The frame fits short lists while its viewport caps long lists to the page.
func (v *nativeView) listViewport(list *widget) {
	v.editorScroll = v.create("scroll", props{classes: "editor-scroll", expand: true})
	v.pack(v.editorScroll, list)
	v.pack(v.body, v.box("panel group list-frame", false, 0, true, v.editorScroll))
}
