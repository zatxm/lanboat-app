//go:build linux && fltk

package native

/*
#cgo CXXFLAGS: -std=c++17 -I${SRCDIR}/../../tools/fltk/include
#cgo LDFLAGS: ${SRCDIR}/../../tools/fltk/lib/libfltk.a -lstdc++ -lm -lpthread
#cgo pkg-config: x11 xft fontconfig freetype2 xext xrender xcursor xfixes xinerama
#include <stdlib.h>
#include "fltk_bridge.h"
*/
import "C"

import (
	_ "embed"
	"errors"
	"github.com/zatxm/lanboat-app/internal/i18n"
	"runtime"
	"slices"
	"strings"
	"unsafe"

	"github.com/zatxm/lanboat-app/internal/desktop"
)

//go:embed icon.rgba
var iconRGBA []byte

type option struct{ value, label string }
type props struct {
	text, value, placeholder, classes              string
	horizontal, expand, disabled, secret, readOnly bool
	gap                                            int
}
type widget struct {
	pointer                     *C.LBWidget
	id                          int
	kind, text, classes         string
	expand, visible, enabled    bool
	loading                     bool
	parent                      *widget
	event                       func(string, string)
	update                      func()
	view                        *nativeView
	raw                         bool
	placeholder, tooltip, value string
	options                     []option
}
type control struct {
	widget           *widget
	config, disabled bool
}
type nativeView struct {
	theme                                               string
	language                                            string
	window                                              *C.LBWidget
	ui                                                  *UI
	widgets                                             map[int]*widget
	next                                                int
	shell, scroll, main, body                           *widget
	editorScroll                                        *widget
	popupRoot, popupError                               *widget
	popupShown                                          *popup
	bannerRows                                          [3]*widget
	dismissedCoreError                                  string
	title, alert, notice, coreError, back, header, save *widget
	nav                                                 map[string]*widget
	controls                                            []control
	inputs                                              map[string]*widget
	connection                                          *widget
	power                                               *widget
	routing                                             *widget
	download                                            *widget
	upload                                              *widget
	totalDown                                           *widget
	totalUp                                             *widget
	connections                                         *widget
	uptime                                              *widget
	nodeContainer, pagination                           *widget
	delayLabels                                         map[string]*widget
	logContainer, logPager                              *widget
	logSignature                                        string
}

func newView(u *UI) (*nativeView, error) {
	window := C.lb_window((*C.uchar)(unsafe.Pointer(&iconRGBA[0])), 64)
	if window == nil {
		return nil, errors.New("无法连接桌面显示服务，请在桌面图形会话中运行")
	}
	v := &nativeView{window: window, ui: u, widgets: map[int]*widget{}, nav: map[string]*widget{}}
	u.view = v
	v.applyTheme()
	v.buildRoot()
	return v, nil
}

var active *nativeView

func str(s string) *C.char { return C.CString(s) }
func free(p *C.char)       { C.free(unsafe.Pointer(p)) }

func chooseLuaFile() string {
	return chooseFile("导入 Lua 文件", "*.lua")
}
func chooseFile(title, pattern string) string {
	language := "zh"
	if active != nil {
		language = active.language
	}
	t, p := str(i18n.Text(language, title)), str(pattern)
	defer free(t)
	defer free(p)
	path := C.lb_choose_file(t, p)
	if path == nil {
		return ""
	}
	return C.GoString(path)
}
func flag(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func requestIdle() { C.lb_awake() }

func Run(backend *desktop.App) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u := newUI(backend, requestIdle)
	u.flagsEnabled = true
	v, err := newView(u)
	if err != nil {
		return err
	}
	active = v
	defer func() { close(u.done); active = nil; v.dispose() }()
	v.render()
	go u.poll()
	go u.backend.RunRuleUpdates(u.done)
	C.lb_run()
	return nil
}

//export goEvent
func goEvent(id C.int, event, value *C.char) {
	v := active
	if v == nil {
		return
	}
	w := v.widgets[int(id)]
	if w == nil || w.event == nil {
		return
	}
	previous := v.ui.actionID
	v.ui.actionID = w.id
	w.event(C.GoString(event), C.GoString(value))
	v.ui.actionID = previous
	if !v.ui.dirty && !v.ui.nodesDirty {
		v.refresh()
	}
}

//export goIdle
func goIdle() C.int {
	if active != nil {
		active.ui.drain()
		active.flush()
	}
	return 0
}

func (v *nativeView) applyTheme() {
	theme := v.ui.state.Config.Settings.Appearance
	if theme == "" {
		theme = "light"
	}
	if theme != v.theme {
		C.lb_theme(flag(theme == "dark"))
		v.theme = theme
	}
}
func (v *nativeView) create(kind string, p props) *widget {
	v.next++
	k := str(kind)
	defer free(k)
	w := &widget{view: v, placeholder: p.placeholder, id: v.next, kind: kind, text: p.text, classes: p.classes, expand: p.expand, visible: true, enabled: !p.disabled}
	v.widgets[w.id] = w
	t, value, ph, classes := str(w.displayText()), str(p.value), str(v.translate(p.placeholder)), str(p.classes)
	defer free(t)
	defer free(value)
	defer free(ph)
	defer free(classes)
	w.pointer = C.lb_create(k, C.int(w.id), t, value, ph, classes, flag(p.horizontal), C.int(p.gap), flag(p.expand), flag(p.disabled), flag(p.secret), flag(p.readOnly))
	return w
}
func (v *nativeView) pack(parent *widget, children ...*widget) {
	for _, child := range children {
		C.lb_pack(parent.pointer, child.pointer)
		child.parent = parent
	}
}
func (v *nativeView) clear(parent *widget) {
	ids := []int{}
	for id, w := range v.widgets {
		for p := w.parent; p != nil; p = p.parent {
			if p == parent {
				ids = append(ids, id)
				break
			}
		}
	}
	for _, id := range ids {
		delete(v.widgets, id)
	}
	controls := v.controls[:0]
	for _, c := range v.controls {
		if _, ok := v.widgets[c.widget.id]; ok {
			controls = append(controls, c)
		}
	}
	v.controls = controls
	for key, w := range v.inputs {
		if _, ok := v.widgets[w.id]; !ok {
			delete(v.inputs, key)
		}
	}
	C.lb_clear(parent.pointer)
}
func (v *nativeView) dispose() {
	C.lb_close()
	clear(v.widgets)
}
func (w *widget) setText(text string) {
	if w == nil || w.text == text {
		return
	}
	w.text = text
	w.localize()
}
func (w *widget) setClasses(classes string) {
	if w == nil || w.classes == classes {
		return
	}
	w.classes = classes
	s := str(classes)
	defer free(s)
	C.lb_classes(w.pointer, s)
}
func (w *widget) setTooltip(text string) {
	if w == nil {
		return
	}
	w.tooltip = text
	s := str(w.view.translate(text))
	defer free(s)
	C.lb_tooltip(w.pointer, s)
}
func (w *widget) setEnabled(enabled bool) {
	if w.enabled == enabled {
		return
	}
	w.enabled = enabled
	C.lb_enabled(w.pointer, flag(enabled))
}
func (w *widget) setVisible(show bool) {
	if w.visible == show {
		return
	}
	w.visible = show
	C.lb_visible(w.pointer, flag(show))
}
func (w *widget) setExpand(expand bool) {
	if w.expand != expand {
		w.expand = expand
		C.lb_expand(w.pointer, flag(expand))
	}
}
func (w *widget) setOptions(options []option, value string) {
	if w.value == value && slices.Equal(w.options, options) {
		return
	}
	w.options = append([]option(nil), options...)
	w.value = value
	w.localizeOptions()
}
func (w *widget) localizeOptions() {
	options, value := w.options, w.value
	rows := make([]string, 0, len(options))
	for _, o := range options {
		rows = append(rows, o.value+"\t"+w.view.translate(o.label))
	}
	opts, val := str(strings.Join(rows, "\n")), str(value)
	defer free(opts)
	defer free(val)
	C.lb_options(w.pointer, opts, val)
}
func (v *nativeView) show()          { C.lb_show() }
func (v *nativeView) top()           { C.lb_top(v.scroll.pointer) }
func (w *widget) focus()             { C.lb_focus(w.pointer) }
func (w *widget) click()             { C.lb_click(w.pointer) }
func (w *widget) edit(value string)  { s := str(value); defer free(s); C.lb_edit(w.pointer, s) }
func (w *widget) nativeText() string { return C.GoString(C.lb_get_text(w.pointer)) }
func pump() {
	for range 12 {
		C.lb_pump()
	}
}
func attachWindow(v *nativeView) { C.lb_pack(v.window, v.shell.pointer) }
func (w *widget) bounds() (int, int, int, int) {
	var x, y, width, height C.int
	C.lb_bounds(w.pointer, &x, &y, &width, &height)
	return int(x), int(y), int(width), int(height)
}

func (v *nativeView) translate(text string) string {
	return i18n.Text(v.ui.state.Config.Settings.Language, text)
}
func (w *widget) displayText() string {
	if w.raw || w.kind == "icon" || w.kind == "box" {
		return w.text
	}
	return w.view.translate(w.text)
}
func (w *widget) setPlaceholder(value string) {
	w.placeholder = value
	text := str(w.view.translate(value))
	defer free(text)
	C.lb_placeholder(w.pointer, text)
}
func (w *widget) localize() {
	text := str(w.displayText())
	defer free(text)
	C.lb_text(w.pointer, text)
	if w.tooltip != "" {
		w.setTooltip(w.tooltip)
	} else if strings.Contains(w.classes, "icon-only") {
		tip := str(w.displayText())
		C.lb_tooltip(w.pointer, tip)
		free(tip)
	}
}
func (v *nativeView) refreshLanguage() {
	language := v.ui.state.Config.Settings.Language
	if language == v.language {
		return
	}
	v.language = language
	for _, w := range v.widgets {
		w.localize()
		if w.placeholder != "" {
			text := str(v.translate(w.placeholder))
			C.lb_placeholder(w.pointer, text)
			free(text)
		}
		if w.options != nil {
			w.localizeOptions()
		}
	}
	title := str(linuxWindowTitle)
	C.lb_window_title(title)
	free(title)
	v.show()
}

func (w *widget) setValue(value string) {
	s := str(value)
	defer free(s)
	C.lb_value(w.pointer, s)
	w.value = value
}
func (w *widget) hasFocus() bool { return C.lb_has_focus(w.pointer) != 0 }
func (w *widget) scrollPosition(position int) int {
	return int(C.lb_scroll_position(w.pointer, C.int(position)))
}

func chooseDirectory(title string) string {
	t := str(i18n.Text(active.language, title))
	defer free(t)
	p := C.lb_choose_directory(t)
	if p == nil {
		return ""
	}
	return C.GoString(p)
}
func openNativeDialog(w *widget, width, height int) {
	C.lb_dialog(w.pointer, C.int(width), C.int(height))
}
func fitNativeDialog()          { C.lb_fit_dialog() }
func closeNativeDialog()        { C.lb_close_dialog() }
func copyClipboard(text string) { s := str(text); defer free(s); C.lb_copy(s) }
func blurFocus()                { C.lb_blur() }

const toolkitName = "FLTK"

func (v *nativeView) compactWindow() bool { return false }
func toolkitVersion() string              { return C.GoString(C.lb_fltk_version()) }

func (w *widget) setLoading(loading bool) {
	if w.loading == loading {
		return
	}
	w.loading = loading
	C.lb_loading(w.pointer, flag(loading))
}

func (v *nativeView) countryIcon(host string) *widget {
	w := v.dataLabel("", "flag")
	w.update = func() { w.setText(countryFlag(v.ui.flags[host].code)) }
	return w
}
