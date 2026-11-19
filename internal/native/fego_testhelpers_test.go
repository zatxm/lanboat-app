package native

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
