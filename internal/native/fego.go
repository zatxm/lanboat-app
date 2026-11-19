package native

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zatxm/fego"
	"github.com/zatxm/fego/ui"
	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/i18n"
)

type option struct{ value, label string }
type props struct {
	text, value, placeholder, classes              string
	horizontal, expand, disabled, secret, readOnly bool
	gap                                            int
}
type widget struct {
	pointer                     *ui.Element
	activityIcon                *ui.Element
	activityFramePending        bool
	buttonCaption               *ui.Element
	buttonValue                 *ui.Element
	icon                        string
	iconColor                   ui.Color
	content                     *ui.Element
	readonly                    bool
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
type systemTray struct {
	relabel func(string, string)
	close   func()
}

type nativeView struct {
	tray                                                *systemTray
	theme                                               string
	language                                            string
	window                                              *fego.Window
	compact                                             bool
	events                                              fego.View
	doc                                                 *ui.Document
	tester                                              *ui.Tester
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

var active *nativeView

const toolkitName = "Fego"

func toolkitVersion() string { return "1.0.2" }

func newView(u *UI) (*nativeView, error) {
	v := &nativeView{ui: u, widgets: map[int]*widget{}, nav: map[string]*widget{}}
	u.view = v
	v.buildRoot()
	v.applyTheme()
	return v, nil
}
func attachWindow(v *nativeView) { v.doc = ui.View(v.shell.pointer.Fill()) }
func Run(backend *desktop.App) error {
	u := newUI(backend, requestIdle)
	u.flagsEnabled = true
	v, err := newView(u)
	if err != nil {
		return err
	}
	active = v
	defer func() { close(u.done); active = nil; v.dispose() }()
	var openErr error
	fego.App.WhenReady(func() {
		title := fmt.Sprintf("Lanboat (%d)", os.Getpid())
		v.window, openErr = fego.App.NewWindow(fego.WindowOptions{Title: title, Width: 1160, Height: 800, Content: display{v}})
		if openErr != nil {
			return
		}
		showWindow := installWindowIcon(title)
		title = "Lanboat"
		if runtime.GOOS == "linux" {
			title = linuxWindowTitle
		}
		v.window.SetTitle(title)
		v.render()
		v.tray = startSystemTray(v.translate("显示主窗口"), v.translate("退出"), showWindow)
		go u.poll()
		go u.backend.RunRuleUpdates(u.done)
	})
	err = fego.App.Run()
	if openErr != nil {
		return openErr
	}
	return err
}

var idleQueued atomic.Bool

func requestIdle() {
	if idleQueued.CompareAndSwap(false, true) {
		fego.App.Post(func() { idleQueued.Store(false); pump() })
	}
}
func pump() {
	if active != nil {
		active.ui.drain()
		active.flush()
		if active.tester != nil {
			active.tester.Step()
		}
	}
}
func (v *nativeView) applyTheme() {
	t := v.ui.state.Config.Settings.Appearance
	if t == "" {
		t = "light"
	}
	if v.theme == t {
		return
	}
	v.theme = t
	v.doc.Theme(lanboatTheme(t))
	for _, w := range v.widgets {
		w.style()
	}
}
func (w *widget) dispatch(event, value string) {
	if !w.enabled || w.event == nil {
		return
	}
	v := w.view
	last := v.ui.actionID
	v.ui.actionID = w.id
	w.event(event, value)
	v.ui.actionID = last
	if v.tester != nil {
		v.flush()
	} else {
		requestIdle()
	}
}
func (v *nativeView) create(kind string, p props) *widget {
	v.next++
	w := &widget{view: v, id: v.next, kind: kind, text: p.text, classes: p.classes, placeholder: p.placeholder, value: p.value, visible: true, enabled: !p.disabled, expand: p.expand, readonly: p.readOnly}
	switch kind {
	case "box", "qr":
		if p.horizontal {
			w.pointer = ui.Row().AlignItems(ui.Center)
		} else {
			w.pointer = ui.Column()
		}
		w.pointer.Gap(float32(p.gap))
		if hasClass(p.classes, "routing-file-row") {
			w.activityIcon = w.lineIcon(18).Hidden(true)
			w.content = w.pointer.Height(routingFileRowHeight).Padding(11, 16).Clip().Append(w.activityIcon)
			background := ui.Canvas(1, routingFileRowHeight, w.paintRoutingFileSelection)
			// Keep the selection canvas behind the row without affecting the row height.
			w.pointer = ui.Column(background, w.content).Gap(-routingFileRowHeight)
		}
	case "scroll":
		w.content = ui.Column().Gap(0)
		w.pointer = ui.ScrollView(w.content)
	case "entry", "text":
		if p.readOnly {
			w.content = ui.Text(p.value)
			if kind == "text" {
				w.pointer = ui.ScrollView(w.content).MinHeight(200)
			} else {
				w.pointer = w.content
			}
		} else {
			if kind == "text" {
				w.pointer = ui.TextArea(p.value)
			} else {
				w.pointer = ui.TextInput(p.value)
			}
			w.pointer.Password(p.secret).Placeholder(v.translate(p.placeholder)).OnChange(func(val string) { w.value = val; w.dispatch("change", val) })
			if hasClass(p.classes, "search-input") {
				// Clip the input's built-in outline and let the single outer field
				// outline enclose both the magnifier and editable text.
				// Fego positions single-line text 10px from the input's top.
				w.content = w.pointer.Height(35).Radius(0).Label(v.translate("搜索节点"))
				w.pointer = ui.Box(w.content).Padding(-1).Clip()
			}
		}
	case "select":
		w.pointer = ui.Box().Gap(0)
	case "separator":
		w.pointer = ui.Separator()
	case "icon":
		w.icon = p.text
		w.pointer = w.lineIcon(20)
	case "button":
		if strings.Contains(p.classes, "switch") {
			w.pointer = ui.Switch("", false).OnToggle(func(bool) { w.dispatch("click", "") })
		} else if lineButton(p.classes) {
			w.activityIcon = w.lineIcon(18)
			w.pointer = ui.Row(w.activityIcon).Gap(7).Clip().OnClick(func() { w.dispatch("click", "") })
			if !hasClass(p.classes, "icon-only") {
				w.buttonCaption = ui.Text("").Clip()
				w.pointer.Append(w.buttonCaption)
				if hasClass(p.classes, "chevron") {
					w.buttonValue = ui.Text("").MaxWidth(300).Clip()
					w.pointer.Append(ui.Box().Grow(1), w.buttonValue, ui.Canvas(15, 18, func(p *ui.Painter) {
						w.paintLineIcon(p, "chevron")
					}))
				}
			}
		} else {
			w.pointer = ui.Button(w.displayText(), func() { w.dispatch("click", "") })
		}
	default:
		w.pointer = ui.Text(w.displayText())
	}
	if w.content == nil {
		w.content = w.pointer
	}
	w.pointer.Disabled(p.disabled)
	if w.expand {
		w.pointer.Grow(1)
	}
	v.widgets[w.id] = w
	w.style()
	w.localize()
	return w
}
func (v *nativeView) pack(parent *widget, children ...*widget) {
	for _, c := range children {
		parent.content.Append(c.pointer)
		c.parent = parent
		if c.event != nil && c.kind == "box" && c != v.popupRoot {
			c.pointer.OnClick(func() { c.dispatch("click", "") })
		}
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
	parent.content.Children()
}

func (v *nativeView) dispose() {
	if v.tray != nil {
		v.tray.close()
	}
	if v.tester != nil {
		v.tester.Close()
	}
	clear(v.widgets)
}
func (v *nativeView) show() {
	if v.window == nil && v.tester == nil {
		v.tester = ui.NewDocumentTester(v.doc, 1160, 800)
	}
}
func (v *nativeView) translate(s string) string {
	return i18n.Text(v.ui.state.Config.Settings.Language, s)
}
func (w *widget) displayText() string {
	if w.raw || w.kind == "icon" || w.kind == "box" {
		return w.text
	}
	return w.view.translate(w.text)
}

var glyphs = map[string]string{
	"home": "⌂", "folder": "▱", "data": "▥", "settings": "⚙", "sail": "◭", "server": "▤", "route": "⇄", "search": "⚲", "back": "←", "forward": "→", "chevron": "›", "down": "↓", "up": "↑", "close": "×", "delete": "×", "import": "⇧", "export": "⇩", "download": "↓", "upload": "↑", "share": "↗", "refresh": "↻", "check": "✓", "save": "✓", "add": "+", "edit": "✎", "test": "◷", "speed": "◴", "info": "ⓘ", "copy": "▣", "link": "↗", "logs": "▤", "tcp": "⇄", "udp": "⇆", "dns": "◎", "moon": "◐", "globe": "◎", "layers": "▤", "file": "▱", "play": "▷",
}

func (w *widget) localize() {
	if w.kind != "button" && w.kind != "label" {
		return
	}
	label := w.displayText()
	if strings.Contains(w.classes, "switch") {
		if w.tooltip != "" {
			label = w.view.translate(w.tooltip)
			w.pointer.Tooltip(label)
		}
		w.pointer.Label(label)
		return
	}
	if w.kind == "button" {
		icon := buttonIcon(w.classes)
		if lineButton(w.classes) {
			w.icon = icon
			if w.buttonValue != nil {
				title, value, _ := strings.Cut(w.text, "\t")
				w.buttonCaption.SetText(w.view.translate(title))
				w.buttonValue.SetText(w.view.translate(value))
				w.buttonValue.Hidden(value == "")
			} else if w.buttonCaption != nil {
				w.buttonCaption.SetText(label)
			}
			accessible, tip := label, ""
			if w.tooltip != "" {
				accessible = w.view.translate(w.tooltip)
				tip = accessible
			} else if hasClass(w.classes, "icon-only") {
				// The caption is hidden, so the hint names the action.
				tip = label
			}
			w.pointer.Label(accessible).Tooltip(tip).Refresh()
			return
		}
		if g := glyphs[icon]; g != "" {
			label = g + " " + label
		}
	}
	if w.loading {
		label = "… " + label
	}
	w.pointer.SetText(label)
}

func (w *widget) setText(s string) {
	if w == nil || w.text == s {
		return
	}
	w.text = s
	if w.kind == "qr" {
		lines := strings.Fields(s)
		n := len(lines)
		img := image.NewGray(image.Rect(0, 0, n, n))
		for y, line := range lines {
			for x, c := range line {
				if c == '0' {
					img.SetGray(x, y, color.Gray{255})
				}
			}
		}
		w.pointer.Children(ui.Image(img).Width(200).Height(200))
		return
	}
	w.localize()
}
func (w *widget) setClasses(s string) {
	if w.classes != s {
		w.classes = s
		w.style()
		w.localize()
	}
}
func (w *widget) setTooltip(s string) {
	w.tooltip = s
	text := w.view.translate(s)
	w.pointer.Label(text).Tooltip(text)
	if w.kind == "button" {
		w.localize()
	}
}
func (w *widget) setEnabled(b bool) {
	if w.enabled != b {
		w.enabled = b
		w.pointer.Disabled(!b)
		w.style()
	}
}
func (w *widget) setVisible(b bool) {
	if w.visible != b {
		w.visible = b
		w.pointer.Hidden(!b)
	}
}
func (w *widget) setExpand(b bool) {
	if w.expand != b {
		w.expand = b
		if b {
			w.pointer.Grow(1)
		} else {
			w.pointer.Grow(0)
		}
	}
}
func (w *widget) setOptions(o []option, val string) {
	if w.value == val && slices.Equal(o, w.options) {
		return
	}
	w.options = slices.Clone(o)
	w.value = val
	w.localizeOptions()
}
func (w *widget) localizeOptions() {
	labels := make([]string, len(w.options))
	selected := -1
	for i, o := range w.options {
		labels[i] = w.view.translate(o.label)
		if o.value == w.value {
			selected = i
		}
	}
	w.pointer.Children(ui.Select(labels, selected).Height(38).Radius(8).OnSelect(func(i int) { w.dispatch("change", w.options[i].value) }))
}
func (w *widget) setValue(s string) { w.value = s; w.content.SetText(s) }
func (w *widget) setPlaceholder(s string) {
	w.placeholder = s
	w.pointer.Placeholder(w.view.translate(s))
}
func (v *nativeView) refreshLanguage() {
	lang := v.ui.state.Config.Settings.Language
	if lang == v.language {
		return
	}
	v.language = lang
	if v.tray != nil {
		v.tray.relabel(v.translate("显示主窗口"), v.translate("退出"))
	}
	for _, w := range v.widgets {
		w.localize()
		if w.tooltip != "" && w.kind != "button" {
			w.setTooltip(w.tooltip)
		}
		if w.placeholder != "" {
			w.setPlaceholder(w.placeholder)
		}
		if w.options != nil {
			w.localizeOptions()
		}
		if hasClass(w.classes, "search-input") {
			w.content.Label(v.translate("搜索节点"))
		}
	}
}
func (w *widget) setLoading(b bool) {
	if w.loading == b {
		return
	}
	w.loading = b
	w.localize()
	if w.activityIcon != nil {
		if w.kind == "box" {
			w.activityIcon.Hidden(!b)
		}
		w.activityIcon.Refresh()
		w.scheduleActivityFrame()
	}
}

func (w *widget) scheduleActivityFrame() {
	v := w.view
	if !w.loading || v.window == nil || w.activityFramePending {
		return
	}
	w.activityFramePending = true
	time.AfterFunc(65*time.Millisecond, func() {
		fego.App.Post(func() {
			w.activityFramePending = false
			if active != v || !w.loading || v.widgets[w.id] != w {
				return
			}
			w.activityIcon.Refresh()
			w.scheduleActivityFrame()
		})
	})
}

func (w *widget) nativeText() string {
	if w.buttonCaption != nil {
		return w.buttonCaption.Text()
	}
	return w.content.Text()
}
func (w *widget) bounds() (int, int, int, int) {
	r := w.pointer.Bounds()
	return int(r.X), int(r.Y), int(r.W), int(r.H)
}
func openNativeDialog(w *widget, width, height int) {
	w.view.doc.ShowDialog(w.pointer.Width(float32(width)).MaxHeight(float32(height)))
}
func closeNativeDialog() {
	if active != nil {
		active.doc.CloseDialog()
	}
}
func fitNativeDialog() {
	if active != nil && active.popupRoot != nil {
		active.popupRoot.pointer.Refresh()
	}
}
func copyClipboard(s string) {
	if active != nil && active.window != nil {
		active.window.SetClipboard(s)
	}
}
func chooseLuaFile() string                   { return chooseFile("导入 Lua 文件", "*.lua") }
func chooseFile(title, pattern string) string { return choosePath(title, pattern, false) }
func chooseDirectory(title string) string     { return choosePath(title, "", true) }
func choosePath(title, pattern string, dir bool) string {
	title = active.translate(title)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		script := "Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.OpenFileDialog; if($d.ShowDialog() -eq 'OK'){[Console]::OutputEncoding=[Text.UTF8Encoding]::new();[Console]::Write($d.FileName)}"
		if dir {
			script = strings.ReplaceAll(strings.ReplaceAll(script, "OpenFileDialog", "FolderBrowserDialog"), "FileName", "SelectedPath")
		}
		cmd = exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script)
	case "darwin":
		s := "POSIX path of (choose file)"
		if dir {
			s = "POSIX path of (choose folder)"
		}
		cmd = exec.Command("osascript", "-e", s)
	default:
		args := []string{"--file-selection", "--title", title}
		if dir {
			args = append(args, "--directory")
		} else if pattern != "" {
			args = append(args, "--file-filter", pattern)
		}
		cmd = exec.Command("zenity", args...)
	}
	out, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			active.ui.err = err.Error()
			active.ui.notify()
		}
		return ""
	}
	return strings.TrimRight(string(out), "\r\n")
}

type display struct{ v *nativeView }
type eventView struct {
	fego.View
	v *nativeView
}

func (d display) Mount(h fego.Host) fego.View {
	view := d.v.doc.Mount(h)
	d.v.events = view
	return eventView{view, d.v}
}
func (v *nativeView) compactWindow() bool { return v.compact }

// Forward Fego’s optional retained repaint hook through the event adapter.
func (d eventView) PaintIfDirty() { d.View.(interface{ PaintIfDirty() }).PaintIfDirty() }

func (d eventView) Event(e fego.Event) {
	if e.Kind == fego.Resize && e.Width > 0 {
		v := d.v
		compact := float32(e.Width)/max(1, e.Scale) < 940
		if compact != v.compact {
			v.compact = compact
			if v.window != nil && v.ui.editor == "" && v.ui.detail == "" &&
				(v.ui.page == "home" || v.ui.page == "data") {
				v.render()
			}
		}
	}
	if e.Kind == fego.KeyDown && e.Key == "Escape" && d.v.ui.popup != nil {
		d.v.ui.closePopup()
		d.v.flush()
		return
	}
	d.View.Event(e)
}
func (v *nativeView) send(e fego.Event) {
	if v.tester != nil {
		v.tester.Send(e)
	} else if v.events != nil {
		v.events.Event(e)
	}
}
func (w *widget) focus() {
	r := w.pointer.Bounds()
	scale := float32(1)
	if w.view.tester != nil {
		scale = w.view.tester.Scale
	} else if w.view.window != nil {
		_, _, scale = w.view.window.Size()
	}
	x, y := (r.X+2)*scale, (r.Y+r.H/2)*scale
	w.view.send(fego.Event{Kind: fego.PointerDown, X: x, Y: y, Button: 1})
	w.view.send(fego.Event{Kind: fego.PointerUp, X: x, Y: y, Button: 1})
}
func blurFocus() {
	if active != nil {
		active.send(fego.Event{Kind: fego.Focus, Focused: false})
	}
}
func (w *widget) scrollPosition(pos int) int {
	r := w.pointer.Bounds()
	c := w.content.Bounds()
	old := max(0, int(r.Y-c.Y))
	if pos >= 0 && pos != old {
		w.view.send(fego.Event{Kind: fego.Scroll, X: r.X + r.W/2, Y: r.Y + r.H/2, DY: float32(pos - old)})
	}
	return old
}
func (v *nativeView) top() { v.scroll.scrollPosition(0) }
