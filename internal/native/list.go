package native

import (
	"fmt"
	"strconv"
)

func (v *nativeView) listRange(key string, total int) (int, int, *widget) {
	u := v.ui
	n, _ := strconv.Atoi(u.field("page." + key).Text())
	pages := max(1, (total+pageSize-1)/pageSize)
	n = min(max(0, n), pages-1)
	u.field("page." + key).SetText(strconv.Itoa(n))
	start := n * pageSize
	var pager *widget
	if total > pageSize {
		pager = v.box("", true, 8, false, v.label(fmt.Sprintf("第 %d / %d 页 · %d 项", n+1, pages, total), "help"), v.spacer(), v.button("上一页", "link icon-up", false, n == 0, func() { u.field("page." + key).SetText(strconv.Itoa(n - 1)); u.resetScroll = true; u.render() }), v.button("下一页", "link icon-down", false, n == pages-1, func() { u.field("page." + key).SetText(strconv.Itoa(n + 1)); u.resetScroll = true; u.render() }))
	}
	return start, min(total, start+pageSize), pager
}

// The frame fits short lists while its viewport caps long lists to the page.
func (v *nativeView) listViewport(list *widget) {
	v.editorScroll = v.create("scroll", props{classes: "editor-scroll", expand: true})
	v.pack(v.editorScroll, list)
	v.pack(v.body, v.box("panel group list-frame", false, 0, true, v.editorScroll))
}
