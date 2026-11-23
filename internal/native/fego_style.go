package native

import (
	"math"
	"strings"
	"time"

	"github.com/zatxm/fego/v26/ui"
)

func lanboatTheme(appearance string) ui.Theme {
	if appearance == "dark" {
		return ui.Dark
	}
	return ui.Light
}

const panelRadius = 12

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
		e.Width(200).Padding(24, 14).Background(t.Sidebar)
	case is("main"):
		e.Padding(20, 32, 28, 30)
	case is("header"):
		e.Padding(14, 32, 10, 30).MinHeight(62)
	case is("nav-item"):
		bg, fg := t.Sidebar, t.Text
		if is("active") {
			bg, fg = t.Raised, t.Accent
		}
		e.Height(46).Padding(0, 12).Gap(45).Radius(8).Background(bg)
		w.buttonCaption.FontSize(15).TextColor(fg)
		w.iconColor = fg
	case is("panel") || is("option-group") || is("dialog-surface"):
		padding, radius := float32(18), float32(panelRadius)
		if is("group") {
			padding = 1
		} else if is("option-group") {
			padding, radius = 14, 8
		}
		e.Background(t.Surface).Border(t.Border).Radius(radius).Padding(padding)
		if is("node-editor") {
			e.Padding(18, 6, 18, 18)
		}
	case is("about-key"):
		e.Width(132)
	case is("dns-control"):
		e.Width(228)
	case is("editor-content"):
		e.Padding(0, 14, 0, 0)
	case is("group-row"):
		e.MinHeight(62).Padding(9, 16).Clip()
	case is("routing-file-row") || is("home-node-row"):
		e.Refresh()
	case is("node-row"):
		e.MinHeight(listRowHeight).Padding(11, 16).Radius(0).Clip()
	case is("node-measurement"):
		e.Width(76).Justify(ui.End)
	case w.kind == "icon" && w.icon == "unreachable":
		w.iconColor = t.Danger
	case is("search-field"):
		e.Height(43).AlignItems(ui.Center).Padding(6, 9, 0, 10).Radius(8)
	case is("share-link"):
		e.Height(38).Padding(0, 10).Radius(8).Background(t.Raised).Clip()
	case is("share-link-preview"):
		e.FontSize(13).Clip()
	case is("alert-row"):
		e.Background(t.Raised).Radius(9).Padding(4, 12)
	case is("empty"):
		e.Padding(22, 18)
	}

	switch {
	case is("toolbar-title"):
		e.FontSize(21).Bold()
	case is("connection-title"):
		e.FontSize(18).Bold().TextColor(t.Text)
	case is("section-title"):
		e.FontSize(13).TextColor(t.Muted)
	case is("log-error"):
		e.FontSize(12).TextColor(t.Danger)
	case is("log-level"):
		color := t.Muted
		if is("level-error") || is("level-fatal") || is("level-panic") || is("level-dpanic") {
			color = t.Danger
		} else if is("level-warn") {
			color = t.Text
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
		e.Size(34, 34).Justify(ui.Center).Background(ui.Color{})
		switch {
		case is("danger"):
			w.iconColor = t.Danger
		case is("group-auto-select") && !is("auto-select-on"):
			w.iconColor = t.Muted
		}
		if !w.enabled && !w.loading && !is("rule-state") {
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
		if !w.enabled && !w.loading {
			fg = t.Muted
		}
		e.Height(38).Padding(0, 13).Radius(8).Background(bg).Border(border)
		if is("link") {
			e.Padding(0, 6)
		}
		if is("flush-left") {
			e.Padding(0)
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
		if !w.enabled && !w.loading {
			fg, w.iconColor = t.Muted, t.Muted
		}
		w.buttonCaption.FontSize(14).TextColor(fg)
		if w.buttonValue != nil {
			w.buttonValue.FontSize(13).TextColor(t.Muted)
		}
	case is("backup-tab"):
		bg, border, fg := t.Surface, t.Border, t.Text
		if is("active") {
			bg, border, fg = t.Raised, t.Accent, t.Accent
		}
		e.Radius(8).Background(bg).Border(border).TextColor(fg)
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

const listRowHeight = 64

func (w *widget) selectedRowColor() ui.Color {
	return lanboatTheme(w.view.theme).Raised
}

func (w *widget) paintRowSelection(p *ui.Painter) {
	if !hasClass(w.classes, "node-selected") {
		return
	}
	bounds := p.Bounds()
	first := hasClass(w.classes, "routing-file-first")
	last := hasClass(w.classes, "routing-file-last") || hasClass(w.classes, "home-node-last")
	fill := w.selectedRowColor()
	if !first && !last {
		p.Rect(bounds, fill)
		return
	}
	// Match the inside of the frame. Corners beyond the canvas clip stay square.
	const radius = panelRadius - 1
	if !first {
		bounds.Y -= radius
		bounds.H += radius
	}
	if !last {
		bounds.H += radius
	}
	p.RoundRect(bounds, radius, fill)
}

func (w *widget) lineIcon(size float32) *ui.Element {
	return ui.Canvas(size, size, func(p *ui.Painter) {
		icon := w.icon
		if w.loading {
			icon = "activity"
		}
		w.paintLineIcon(p, icon)
	})
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
	case "activity":
		start := float32(time.Now().UnixMilli()%780) * (2 * math.Pi / 780)
		arc(0, 0, 6, start, start+1.5*math.Pi)
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
	case "auto":
		// The filled branch, open branch and spark follow the supplied selection icon.
		var branches ui.Path
		branches.MoveTo(x-5.3, y-2.4)
		branches.CubicTo(x-5.3, y+0.5, x-1, y+0.5, x-1, y+3.7)
		branches.MoveTo(x+4.3, y-2)
		branches.CubicTo(x+4.3, y+0.5, x-1, y+0.7, x-1, y+3.7)
		p.StrokePath(&branches, 1.25, c)
		p.Circle(x-5.5, y-3, 2.3, c)
		ring(4.5, -3, 1.8)
		ring(-1, 5.7, 1.9)
		var star ui.Path
		star.MoveTo(x+6.5, y-9)
		star.QuadTo(x+6.8, y-7.4, x+8.4, y-7.1)
		star.QuadTo(x+6.8, y-6.8, x+6.5, y-5.2)
		star.QuadTo(x+6.2, y-6.8, x+4.6, y-7.1)
		star.QuadTo(x+6.2, y-7.4, x+6.5, y-9)
		star.Close()
		p.FillPath(&star, c)
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
	case "unreachable":
		ring(0, 0, 7)
		line(-2.5, -2.5, 2.5, 2.5)
		line(-2.5, 2.5, 2.5, -2.5)
	case "delete":
		c = lanboatTheme(w.view.theme).Danger
		if !w.enabled {
			c.A = 153
		}
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
	case "share":
		line(-4, -1.5, 3.5, -5.5)
		line(-4, 1.5, 3.5, 5.5)
		ring(-6, 0, 2.2)
		ring(6, -6, 2.2)
		ring(6, 6, 2.2)
	case "import", "export", "download", "upload", "up", "down":
		dir := float32(1)
		if icon == "upload" || icon == "up" {
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
	case "network-sharing":
		path(pair(-4, -8), pair(4, -8), pair(4, -3), pair(-4, -3), pair(-4, -8))
		line(0, -3, 0, 0)
		path(pair(-5, 3), pair(-5, 0), pair(5, 0), pair(5, 3))
		for _, cx := range []float32{-5, 5} {
			path(pair(cx-3, 3), pair(cx+3, 3), pair(cx+3, 8), pair(cx-3, 8), pair(cx-3, 3))
		}
	case "globe", "dns":
		ring(0, 0, 7)
		line(-6, 0, 6, 0)
		arc(0, 0, 6, -1.6, 1.6)
	case "language":
		path(pair(-8, -7), pair(4, -7), pair(4, 2), pair(-3, 2), pair(-7, 5), pair(-7, 2), pair(-8, 2), pair(-8, -7))
		path(pair(-6, 0), pair(-2, -5), pair(2, 0))
		line(-4, -1.5, 0, -1.5)
		path(pair(6, -3), pair(8, -3), pair(8, 7), pair(5, 7), pair(3, 5), pair(1, 5))
	case "shield":
		path(pair(0, -8), pair(7, -5), pair(6, 2), pair(4, 5), pair(0, 8), pair(-4, 5), pair(-6, 2), pair(-7, -5), pair(0, -8))
		path(pair(-3, 0), pair(-1, 2), pair(3, -2))
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
