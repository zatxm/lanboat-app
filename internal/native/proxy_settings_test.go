package native

import (
	"strings"
	"testing"

	"github.com/zatxm/fego"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowProxySettings(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	button := func(text string) *widget {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == text || strings.HasPrefix(c.widget.text, text+"\t") {
				return c.widget
			}
		}
		t.Fatal("missing", text)
		return nil
	}
	clickPointer := func(w *widget) {
		r := w.pointer.Bounds()
		x, y := r.X+r.W/2, r.Y+r.H/2
		v.send(fego.Event{Kind: fego.PointerMove, X: x, Y: y})
		v.send(fego.Event{Kind: fego.PointerDown, X: x, Y: y, Button: 1})
		v.send(fego.Event{Kind: fego.PointerUp, X: x, Y: y, Button: 1})
		pump()
	}
	checkRows := func(keys, labels []string) {
		t.Helper()
		var columnX, width, previousBottom float32
		for i, key := range keys {
			input := v.inputs["proxy."+key]
			if input == nil {
				t.Fatalf("missing proxy input %q", key)
			}
			r := input.pointer.Bounds()
			if r.W <= 0 || i > 0 && (r.X != columnX || r.W != width || r.Y < previousBottom) {
				t.Fatalf("proxy input %q is out of its column or row: %+v", key, r)
			}
			var label *widget
			for _, w := range v.widgets {
				if w.parent == input.parent && w.text == labels[i] {
					label = w
					break
				}
			}
			if label == nil {
				t.Fatalf("proxy label %q is not in the input row", labels[i])
			}
			l := label.pointer.Bounds()
			if dy := (l.Y + l.H/2) - (r.Y + r.H/2); dy < -1 || dy > 1 || l.X+l.W >= r.X {
				t.Fatalf("proxy label %q is not next to its input: %+v %+v", labels[i], l, r)
			}
			columnX, width, previousBottom = r.X, r.W, r.Y+r.H
		}
	}
	u.navigate("settings")
	v.render()
	pump()
	for _, w := range v.widgets {
		if w.text == "本地代理" || w.text == "修改端口" {
			t.Fatal("old inline control remains")
		}
	}
	button("代理").click()
	pump()
	if u.detail != "proxy-settings" || !v.back.visible || v.save.visible || v.inputs["proxy.port"] != nil {
		t.Fatal("proxy summary navigation")
	}
	summary := v.body
	edit := button("修改")
	x, y, width, _ := edit.bounds()
	before := v.tester.Snapshot().RGBAAt(x+width/2, y+3)
	clickPointer(edit)
	checkRows([]string{"port"}, []string{"端口"})
	v.inputs["proxy.port"].edit("1089")
	if u.state.Config.Settings.Port == 1089 {
		t.Fatal("port draft applied")
	}
	button("确定").click()
	waitJob(t, u)
	pump()
	if u.popup != nil || v.body != summary || u.state.Config.Settings.Port != 1089 {
		t.Fatal("port save")
	}
	if after := v.tester.Snapshot().RGBAAt(x+width/2, y+3); after != before {
		t.Fatal("click left a background on the edit icon", before, after)
	}
	u.editProxy("sharing")
	pump()
	clickPointer(button("允许局域网设备连接"))
	auth := button("访问认证")
	clickPointer(auth)
	x, y, width, _ = auth.bounds()
	frame := v.tester.Snapshot()
	if !auth.pointer.Checked() || frame.RGBAAt(x+width/2, y+1) != frame.RGBAAt(x-6, y+1) {
		t.Fatal("switch did not toggle or clicking left a focus background")
	}
	checkRows([]string{"username", "password"}, []string{"用户", "密码"})
	v.inputs["proxy.username"].edit("local-user")
	v.inputs["proxy.password"].edit("private-key")
	button("访问认证").click()
	pump()
	button("访问认证").click()
	pump()
	if v.inputs["proxy.password"].nativeText() != "private-key" {
		t.Fatal("toggle lost password")
	}
	u.saveProxySection("sharing")
	waitJob(t, u)
	pump()
	u.editProxy("upstream")
	pump()
	button("启用前置代理").click()
	pump()
	checkRows([]string{"type", "server", "upstreamPort", "upstreamUser", "upstreamPassword"}, []string{"类型", "地址", "端口", "用户", "密码"})
	if u.field("proxy.type").Text() != "socks5" || v.inputs["proxy.server"].placeholder != "IP" {
		t.Fatal("upstream defaults")
	}
	v.inputs["proxy.server"].edit("127.0.0.1")
	v.inputs["proxy.upstreamPort"].edit("0")
	u.saveProxySection("upstream")
	pump()
	if u.popup == nil || u.popup.kind != "message" {
		t.Fatal("invalid upstream accepted")
	}
	u.closePopup()
	pump()
	v.inputs["proxy.upstreamPort"].edit("65535")
	v.inputs["proxy.upstreamUser"].edit("up-user")
	v.inputs["proxy.upstreamPassword"].edit("up-key")
	v.inputs["proxy.type"].edit("http")
	pump()
	if v.inputs["proxy.upstreamPassword"].nativeText() != "up-key" {
		t.Fatal("type change lost draft")
	}
	u.saveProxySection("upstream")
	waitJob(t, u)
	pump()
	s := u.backend.GetState().Config.Settings
	if u.err != "" || u.popup != nil || v.body != summary || s.Port != 1089 || !s.Proxy.AllowLAN || !s.Proxy.Authentication || s.Proxy.Password != "private-key" || s.Proxy.Upstream.Type != "http" || s.Proxy.Upstream.Password != "up-key" {
		t.Fatal("proxy save", u.err)
	}
	u.editProxy("sharing")
	pump()
	u.changeSettings(func(s *profile.Settings) { s.Language = "en"; s.Appearance = "dark" })
	waitJob(t, u)
	pump()
	assertEnglishWidgets(t, v)
	if v.inputs["proxy.password"].nativeText() != "private-key" {
		t.Fatal("language update lost draft")
	}
}
