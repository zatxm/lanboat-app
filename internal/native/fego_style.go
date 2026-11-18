//go:build !fltk

package native

import (
	"math"
	"strings"

	"github.com/zatxm/fego/ui"
)

func lanboatTheme(appearance string) ui.Theme {
	white := ui.RGB(255, 255, 255)
	if appearance == "dark" {
		return ui.Theme{
			Background: ui.RGB(20, 21, 24), Surface: ui.RGB(29, 30, 34), Raised: ui.RGB(42, 45, 51),
			Text: ui.RGB(229, 231, 235), Muted: ui.RGB(151, 156, 167), Border: ui.RGB(52, 55, 62),
			Accent: ui.RGB(184, 198, 218), AccentText: ui.RGB(20, 21, 24), Danger: ui.RGB(236, 134, 130), FontSize: 14,
		}
	}
	return ui.Theme{
		Background: white, Surface: white, Raised: ui.RGB(239, 240, 243),
		Text: ui.RGB(31, 35, 41), Muted: ui.RGB(109, 116, 130), Border: ui.RGB(224, 228, 235),
		Accent: ui.RGB(75, 102, 146), AccentText: white, Danger: ui.RGB(187, 56, 53), FontSize: 14,
	}
}

func hasClass(classes, name string) bool {
	return strings.Contains(" "+classes+" ", " "+name+" ")
}

func usesLineButton(classes string) bool {
	return hasClass(classes, "nav-item") || hasClass(classes, "row-button") ||
		hasClass(classes, "icon-only") || hasClass(classes, "home-action") || hasClass(classes, "header-back") || hasClass(classes, "header-save")
}

// lineButton reports buttons drawn as a line icon plus caption. Switches keep
// their control; buttons without an icon stay plain text buttons.
func lineButton(classes string) bool {
	return !hasClass(classes, "switch") && (usesLineButton(classes) || buttonIcon(classes) != "")
}

func buttonIcon(classes string) string {
	if hasClass(classes, "disclosure-open") {
		return "fold-down"
	}
	if hasClass(classes, "disclosure") {
		return "chevron"
	}
	for _, class := range strings.Fields(classes) {
		if icon, ok := strings.CutPrefix(class, "icon-"); ok && icon != "only" && icon != "" {
			return icon
		}
	}
	return ""
}

func (w *widget) style() {
	is := func(c string) bool { return hasClass(w.classes, c) }
	e := w.pointer
	t := lanboatTheme(w.view.theme)
	w.iconColor = t.Accent

	switch {
	case is("shell"):
		e.AlignItems(ui.Stretch).Background(t.Background)
	case is("stats-grid"):
		e.AlignItems(ui.Stretch)
	case is("sidebar"):
		e.Width(200).Padding(24, 14).Background(t.Background)
	case is("brand"):
		e.Padding(8, 12, 27, 12).MinHeight(81)
	case is("main"):
		e.Padding(20, 32, 28, 30)
	case is("header"):
		e.Padding(14, 32, 10, 30).MinHeight(62)
	case is("nav-item"):
		bg, fg := t.Background, t.Text
		if is("active") {
			bg, fg = t.Raised, t.Accent
		}
		e.Height(46).Padding(0, 12).Radius(8).Background(bg)
		w.buttonCaption.FontSize(15).TextColor(fg)
		w.iconColor = fg
	case is("panel") || is("option-group") || is("dialog-surface"):
		padding, radius := float32(18), float32(16)
		if is("group") {
			padding = 1
		} else if is("option-group") {
			padding, radius = 14, 11
		}
		e.Background(t.Surface).Border(t.Border).Radius(radius).Padding(padding)
		if is("node-editor") {
			e.Padding(18, 6, 18, 18)
		}
	case is("editor-content"):
		e.Padding(0, 14, 0, 0)
	case is("group-row"):
		e.MinHeight(62).Padding(9, 16).Clip()
	case is("node-row"):
		e.MinHeight(64).Padding(11, 16).Radius(0).Clip()
		bg := ui.Color{}
		if is("node-selected") {
			bg = ui.RGB(243, 247, 253)
			if w.view.theme == "dark" {
				bg = t.Raised
			}
			if is("node-last") {
				e.Radius(15)
			}
		}
		e.Background(bg)
	case is("search-field"):
		e.Height(43).Padding(3, 11).Radius(9).Background(t.Surface).Border(t.Border)
	case is("search-input"):
		e.Height(35).Radius(5).Border(t.Surface)
	case is("alert-row"):
		e.Background(t.Raised).Radius(9).Padding(4, 12)
	case is("empty"):
		e.Padding(22, 18)
	}

	switch {
	case is("toolbar-title") || is("brand-title"):
		e.FontSize(21).Bold()
	case is("connection-title"):
		e.FontSize(18).Bold().TextColor(t.Text)
		if is("green") {
			e.TextColor(ui.RGB(40, 135, 99))
		}
	case is("section-title"):
		e.FontSize(13).TextColor(t.Muted)
	case is("log-error"):
		e.FontSize(12).TextColor(t.Danger)
	case is("log-level"):
		color := t.Muted
		if is("level-error") || is("level-fatal") || is("level-panic") || is("level-dpanic") {
			color = t.Danger
		} else if is("level-warn") {
			color = ui.RGB(183, 121, 31)
		} else if is("level-info") {
			color = t.Accent
		}
		e.FontSize(12).Bold().TextColor(color)
	case is("log-message"):
		e.FontSize(13)
	case is("help") || is("subtitle") || is("field-label"):
		e.FontSize(12).TextColor(t.Muted)
	case is("node-name"):
		e.FontSize(15)
	case is("stat-value"):
		e.FontSize(25).Bold()
	case is("stat-traffic"):
		e.FontSize(16)
	case is("dialog-title"):
		e.FontSize(19).Bold()
	}
	if is("node-delay") {
		e.Width(76)
	}
	if is("compact") {
		e.Width(110)
	}
	if is("protocol-choice") {
		e.Width(228)
	}
	if is("switch") {
		e.Width(43).SetChecked(is("switch-on"))
	}
	if w.kind != "button" || is("nav-item") || is("switch") {
		return
	}
	switch {
	case is("icon-only"):
		e.Size(34, 34).Justify(ui.Center)
		if !w.enabled {
			w.iconColor = t.Muted
		}
	case lineButton(w.classes) && !usesLineButton(w.classes):
		// Former text buttons keep their filled or outlined look with a line icon.
		bg, border, fg := t.Surface, t.Border, t.Text
		switch {
		case is("primary"):
			bg, border, fg = t.Accent, t.Accent, t.AccentText
		case is("danger"):
			bg, border, fg = t.Danger, t.Danger, t.AccentText
		case is("link"):
			bg, border, fg = ui.Color{}, ui.Color{}, t.Accent
		case is("active"):
			bg, border, fg = t.Raised, t.Accent, t.Accent
		case is("secondary"):
			bg = t.Raised
		}
		if !w.enabled {
			fg = t.Muted
		}
		e.Height(38).Padding(0, 13).Radius(8).Background(bg).Border(border)
		if is("link") {
			e.Padding(0, 6)
		}
		w.iconColor = fg
		w.buttonCaption.FontSize(14).Bold().TextColor(fg)
	case usesLineButton(w.classes):
		e.Height(38).Padding(0, 7).Radius(8)
		if is("row-button") {
			e.Height(50).Padding(0, 16)
			if is("disclosure") {
				e.Height(44).Padding(0, 3)
			}
			w.iconColor = t.Muted
		}
		fg := t.Accent
		if is("primary") {
			e.Background(t.Accent).Padding(0, 10)
			fg = t.AccentText
			w.iconColor = fg
		}
		if is("row-button") {
			fg = t.Text
		}
		if !w.enabled {
			fg, w.iconColor = t.Muted, t.Muted
		}
		w.buttonCaption.FontSize(14).TextColor(fg)
		if w.buttonValue != nil {
			w.buttonValue.FontSize(13).TextColor(t.Muted)
		}
	default:
		e.Radius(8)
		if is("primary") {
			e.Background(t.Accent).TextColor(t.AccentText)
		}
		if is("danger") {
			e.Background(t.Danger).TextColor(t.AccentText)
		}
		if is("secondary") {
			e.Background(t.Raised)
		}
		if is("active") {
			e.Background(t.Raised).Border(t.Accent).TextColor(t.Accent)
		}
	}
}

func (w *widget) lineIcon(size float32) *ui.Element {
	return ui.Canvas(size, size, func(p *ui.Painter) { w.paintLineIcon(p, w.icon) })
}

func (w *widget) paintLineIcon(p *ui.Painter, icon string) {
	b := p.Bounds()
	x, y := b.W/2, b.H/2
	c := w.iconColor
	line := func(x1, y1, x2, y2 float32) { p.Line(x+x1, y+y1, x+x2, y+y2, 1.25, c) }
	path := func(points ...[2]float32) {
		for i := 1; i < len(points); i++ {
			a, b := points[i-1], points[i]
			line(a[0], a[1], b[0], b[1])
		}
	}
	pair := func(x, y float32) [2]float32 { return [2]float32{x, y} }
	arc := func(cx, cy, r, start, end float32) {
		ax, ay := cx+r*float32(math.Cos(float64(start))), cy+r*float32(math.Sin(float64(start)))
		for i := 1; i <= 20; i++ {
			a := start + (end-start)*float32(i)/20
			bx, by := cx+r*float32(math.Cos(float64(a))), cy+r*float32(math.Sin(float64(a)))
			line(ax, ay, bx, by)
			ax, ay = bx, by
		}
	}
	ring := func(cx, cy, r float32) { arc(cx, cy, r, 0, 2*math.Pi) }
	switch icon {
	case "sail":
		path(pair(-7, 6), pair(-4, 8), pair(5, 8), pair(8, 5), pair(-8, 5))
		path(pair(-1, -7), pair(-6, 2), pair(-1, 2), pair(-1, -7))
		path(pair(2, -5), pair(7, 2), pair(2, 2))
	case "home":
		path(pair(-8, -1), pair(0, -8), pair(8, -1))
		path(pair(-6, 0), pair(-6, 7), pair(-2, 7), pair(-2, 2), pair(2, 2), pair(2, 7), pair(6, 7), pair(6, 0))
	case "data", "test":
		path(pair(-8, 0), pair(-4, 0), pair(-2, -6), pair(1, 7), pair(4, -2), pair(6, 0), pair(8, 0))
	case "settings":
		ring(0, 0, 5)
		ring(0, 0, 2)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			s, c := math.Sincos(a)
			line(6*float32(c), 6*float32(s), 8*float32(c), 8*float32(s))
		}
	case "speed":
		arc(0, 2, 7, math.Pi, 2*math.Pi)
		path(pair(-7, 2), pair(-7, 5), pair(7, 5), pair(7, 2))
		line(0, 2, 4, -2)
	case "search":
		ring(-2, -2, 5)
		line(2, 2, 7, 7)
	case "info":
		ring(0, 0, 7)
		line(0, -1, 0, 4)
		p.Circle(x, y-4, 0.8, c)
	case "delete":
		line(-7, -4, 7, -4)
		path(pair(-5, -4), pair(-4, 7), pair(4, 7), pair(5, -4))
		path(pair(-3, -4), pair(-3, -7), pair(3, -7), pair(3, -4))
		line(-1.5, -1, -1.5, 4)
		line(1.5, -1, 1.5, 4)
	case "add":
		line(0, -6, 0, 6)
		line(-6, 0, 6, 0)
	case "check", "save":
		path(pair(-6, 0), pair(-2, 4), pair(7, -5))
	case "refresh":
		arc(0, 0, 6, -.5, 5.05)
		path(pair(6, -7), pair(6, -2), pair(1, -2))
	case "import", "export", "download", "upload", "share", "up", "down":
		dir := float32(1)
		if icon == "upload" || icon == "share" || icon == "up" {
			dir = -1
		}
		if icon != "up" && icon != "down" {
			path(pair(-7, 3), pair(-7, 7), pair(7, 7), pair(7, 3))
		}
		line(0, -7*dir, 0, 3*dir)
		path(pair(-3.5, -0.5*dir), pair(0, 3*dir), pair(3.5, -0.5*dir))
	case "back", "forward":
		dir := float32(1)
		if icon == "forward" {
			dir = -1
		}
		path(pair(-dir, -5), pair(-6*dir, 0), pair(-dir, 5))
		line(-6*dir, 0, 6*dir, 0)
	case "fold-down":
		path(pair(-4, -2), pair(0, 2), pair(4, -2))
	case "chevron":
		path(pair(-2, -4), pair(2, 0), pair(-2, 4))
	case "close":
		line(-5, -5, 5, 5)
		line(-5, 5, 5, -5)
	case "clock":
		ring(0, 0, 7)
		path(pair(0, -4), pair(0, 0), pair(3, 2))
	case "edit":
		path(pair(-6, 2), pair(3, -7), pair(7, -3), pair(-2, 6), pair(-7, 7), pair(-6, 2))
		line(1, -5, 5, -1)
	case "copy":
		path(pair(-3, -3), pair(7, -3), pair(7, 7), pair(-3, 7), pair(-3, -3))
		path(pair(4, -5), pair(4, -7), pair(-7, -7), pair(-7, 4), pair(-5, 4))
	case "folder":
		path(pair(-7, -6), pair(-1, -6), pair(1, -3), pair(8, -3), pair(6, 7), pair(-7, 7), pair(-7, -6))
	case "monitor":
		path(pair(-8, -6), pair(8, -6), pair(8, 4), pair(-8, 4), pair(-8, -6))
		line(0, 4, 0, 7)
		line(-4, 7, 4, 7)
	case "globe", "dns":
		ring(0, 0, 7)
		line(-6, 0, 6, 0)
		arc(0, 0, 6, -1.6, 1.6)
	case "pie":
		arc(0, 0, 7, 0, 1.5*math.Pi)
		path(pair(0, -7), pair(0, 0), pair(7, 0))
	case "route":
		ring(0, 0, 7)
		path(pair(-3, 3), pair(-1, -1), pair(4, -4), pair(1, 1), pair(-3, 3))
	case "server":
		path(pair(-7, -7), pair(7, -7), pair(7, -1), pair(-7, -1), pair(-7, -7))
		path(pair(-7, 1), pair(7, 1), pair(7, 7), pair(-7, 7), pair(-7, 1))
		p.Circle(x-4, y-4, 0.9, c)
		p.Circle(x-4, y+4, 0.9, c)
	case "layers":
		path(pair(0, -7), pair(8, -3), pair(0, 1), pair(-8, -3), pair(0, -7))
		path(pair(-8, 1), pair(0, 5), pair(8, 1))
	case "tcp":
		line(-7, -3, 7, -3)
		path(pair(4, -6), pair(7, -3), pair(4, 0))
		line(-7, 3, 7, 3)
		path(pair(-4, 0), pair(-7, 3), pair(-4, 6))
	case "udp":
		for _, dx := range []float32{-7, -2, 3} {
			line(dx, -3, dx+3, -3)
			line(dx+1, 3, dx+4, 3)
		}
		path(pair(4, -6), pair(7, -3), pair(4, 0))
		path(pair(-4, 0), pair(-7, 3), pair(-4, 6))
	case "moon":
		ring(0, 0, 7)
		line(0, -7, 0, 7)
		line(-2.5, -6.5, -2.5, 6.5)
		line(-5, -5, -5, 5)
	case "link":
		arc(-3, 0, 3.5, math.Pi/2, 3*math.Pi/2)
		arc(3, 0, 3.5, -math.Pi/2, math.Pi/2)
		line(-3, -3.5, 3, -3.5)
		line(-3, 3.5, 3, 3.5)
		line(-2, 0, 2, 0)
	case "logs":
		path(pair(-6, -7), pair(6, -7), pair(6, 7), pair(-6, 7), pair(-6, -7))
		line(-3, -3, 3, -3)
		line(-3, 0, 3, 0)
		line(-3, 3, 1, 3)
	case "file":
		path(pair(-6, -7), pair(2, -7), pair(6, -3), pair(6, 7), pair(-6, 7), pair(-6, -7))
		path(pair(2, -7), pair(2, -3), pair(6, -3))
	case "play":
		path(pair(-4, -6), pair(6, 0), pair(-4, 6), pair(-4, -6))
	case "":
	default:
		g := glyphs[icon]
		if g == "" {
			g = "›"
		}
		width, height := p.Measure(g, 15, false)
		p.Label(g, x-width/2, y-height/2, 15, c, false)
	}
}
