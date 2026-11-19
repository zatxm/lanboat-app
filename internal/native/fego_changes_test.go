//go:build !fltk

package native

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/traffic"
)

func TestLogTabSelectionAndEmptyPadding(t *testing.T) {
	u := testUI(t)
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()
	u.page, u.detail = "data", "network-logs"
	v.render()
	for _, kind := range []string{"proxy", "dns", "app", "proxy"} {
		u.logs = nil
		u.networkQuery.Store(logQuery{Kind: kind})
		u.networkPage = traffic.Page{Kind: kind}
		v.refresh()
		v.tester.Step()
		active := 0
		empty := false
		for _, w := range v.widgets {
			if hasClass(w.classes, "backup-tab") && hasClass(w.classes, "active") {
				active++
				if w.text != map[string]string{"proxy": "代理日志", "dns": "DNS日志", "app": "应用日志"}[kind] {
					t.Fatal(w.text)
				}
			}
			if w.text == "暂无日志" {
				empty = true
				if w.pointer.Bounds().X < v.logContainer.pointer.Bounds().X+15 {
					t.Fatal("missing inset")
				}
			}
		}
		if active != 1 || !empty {
			t.Fatal(kind, active, empty)
		}
	}
}
func TestRulesFrameFitsRows(t *testing.T) {
	u := testUI(t)
	v, e := newView(u)
	if e != nil {
		t.Fatal(e)
	}
	defer v.dispose()
	u.page = "config"
	v.render()
	v.tester.Step()
	for _, w := range v.widgets {
		if hasClass(w.classes, "list-frame") {
			b := w.pointer.Bounds()
			if b.H < 60 || b.H > 90 {
				t.Fatal(b)
			}
			return
		}
	}
	t.Fatal("rules frame missing")
}

func TestRoutingConfigNameRowAndPreviewActivity(t *testing.T) {
	u := testUI(t)
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()

	checkName := func() {
		t.Helper()
		v.render()
		v.tester.Step()
		input := v.inputs["fileName"]
		b := input.pointer.Bounds()
		for _, w := range v.widgets {
			if w.parent == input.parent && w.text == "名称" {
				label := w.pointer.Bounds()
				if b.X <= label.X+label.W || label.Y < b.Y || label.Y+label.H > b.Y+b.H {
					t.Fatalf("name label %v and input %v should share a row", label, b)
				}
				return
			}
		}
		t.Fatal("name label missing")
	}
	u.openPage("config")
	u.editConfigFile(profile.ConfigFile{ID: "custom", Name: "test.conf"})
	checkName()
	u.importConfig()
	checkName()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte("[Rule]\nFINAL,DIRECT"))
	}))
	defer server.Close()
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	v.inputs["importMethod"].edit("url")
	v.inputs["importURL"].edit(server.URL + "/test.conf")
	for _, c := range v.controls {
		button := c.widget
		if button.text != "解析" {
			continue
		}
		button.click()
		if !u.busy || !button.loading || button.enabled || button.activityIcon == nil {
			t.Fatal("preview button did not show activity")
		}
		v.tester.Step()
		iconBounds := button.activityIcon.Bounds()
		iconPixels := func() []byte {
			rect := image.Rect(0, 0, int(iconBounds.W), int(iconBounds.H))
			frame := image.NewRGBA(rect)
			position := image.Pt(int(iconBounds.X), int(iconBounds.Y))
			draw.Draw(frame, rect, v.tester.Snapshot(), position, draw.Src)
			return frame.Pix
		}
		before := iconPixels()
		time.Sleep(130 * time.Millisecond)
		button.activityIcon.Refresh()
		v.tester.Step()
		if bytes.Equal(before, iconPixels()) {
			t.Fatal("activity icon did not rotate")
		}
		close(release)
		released = true
		waitJob(t, u)
		v.flush()
		for _, next := range v.controls {
			if next.widget.text == "解析" && next.widget.enabled && !next.widget.loading {
				return
			}
		}
		t.Fatal("preview button did not reset")
	}
	t.Fatal("preview button missing")
}

func TestHomeSearchRespondsToIconAndExportNoticeWraps(t *testing.T) {
	u := testUI(t)
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()
	v.render()
	v.tester.Step()

	field := v.inputs["search"].parent
	bounds := field.pointer.Bounds()
	v.tester.Click(bounds.X+18, bounds.Y+bounds.H/2)
	v.tester.Type("fixture")
	if got := u.field("search").Text(); got != "fixture" {
		t.Fatalf("clicking the search icon did not focus the search box, query %q", got)
	}
	search := v.inputs["search"].pointer.Bounds()
	photo := v.tester.Snapshot()
	if top := photo.RGBAAt(int(search.X+40), int(search.Y)); top != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("focused input draws a second border inside the search field: %v", top)
	}
	top, bottom := photo.Rect.Max.Y, 0
	for y := int(search.Y); y < int(search.Y+search.H); y++ {
		for x := int(search.X + 10); x < int(search.X+52); x++ {
			if pixel := photo.RGBAAt(x, y); pixel.R < 130 && pixel.G < 130 && pixel.B < 130 {
				top, bottom = min(top, y), max(bottom, y+1)
			}
		}
	}
	textCenter := float32(top+bottom) / 2
	if center := bounds.Y + bounds.H/2; textCenter < center-1.5 || textCenter > center+1.5 {
		t.Fatalf("search text center %v does not match field center %v", textCenter, center)
	}

	path := "/tmp/" + strings.Repeat("very-long-directory/", 15) + "nodes.json"
	u.notice = "已导出到：\n" + path
	v.refresh()
	v.tester.Step()
	var lines int
	for _, w := range v.widgets {
		if w.kind != "label" || w.classes != "node-name" || !w.raw {
			continue
		}
		lines++
		if w.pointer.Bounds().W > v.popupRoot.pointer.Bounds().W-30 {
			t.Fatal("line extends beyond the dialog", w.text)
		}
	}
	if lines < 3 {
		t.Fatal("long export path was not wrapped")
	}
}

func TestConnectionTimerDisplayAndBackupLimit(t *testing.T) {
	u := testUI(t)
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()

	u.navigate("data")
	u.status.Connected = true
	u.status.StartedAt = time.Now().Add(-4*time.Minute - 10500*time.Millisecond).UnixMilli()
	v.render()
	if !strings.HasPrefix(v.uptime.text, "0:04:") {
		t.Fatal("connection time was not displayed", v.uptime.text)
	}
	u.status.Connected = false
	v.refresh()
	if v.uptime.text != "0:00:00" {
		t.Fatal("timer did not reset", v.uptime.text)
	}

	u.navigate("settings")
	u.openBackups()
	u.state.Backups = make([]desktop.BackupInfo, desktop.MaxBackups)
	v.render()
	for _, c := range v.controls {
		if c.widget.text == "立即备份" {
			if c.widget.enabled || c.widget.tooltip != backupLimitHint {
				t.Fatal("backup limit did not disable the button or show the hint")
			}
			return
		}
	}
	t.Fatal("new backup button missing")
}

func TestRoutingSelectionFollowsRoundedFrame(t *testing.T) {
	u := testUI(t)
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()
	u.page = "config"
	const rules = "[Rule]\nFINAL,DIRECT"
	u.state.Config.Files = append(u.state.Config.Files,
		profile.ConfigFile{ID: "middle", Name: "middle.conf", Content: rules},
		profile.ConfigFile{ID: "last", Name: "last.conf", Content: rules})
	for i, f := range u.state.Config.Files {
		u.state.Config.ActiveFile = f.ID
		v.render()
		v.tester.Step()
		photo := v.tester.Snapshot()
		found := false
		for _, w := range v.widgets {
			if w.text != f.Name || !hasClass(w.classes, "node-selected") {
				continue
			}
			found = true
			b := w.pointer.Bounds()
			fill := w.selectedRowColor()
			want := color.RGBA{fill.R, fill.G, fill.B, fill.A}
			for _, x := range []int{int(b.X + 2), int(b.X + b.W - 3)} {
				top := photo.RGBAAt(x, int(b.Y+2)) == want
				bottom := photo.RGBAAt(x, int(b.Y+b.H-3)) == want
				if top != (i != 0) || bottom != (i != len(u.state.Config.Files)-1) {
					t.Fatalf("wrong corners for selected row %d at x=%d, top=%t bottom=%t", i, x, top, bottom)
				}
			}
		}
		if !found {
			t.Fatal("selected row missing", f.ID)
		}
	}
}
