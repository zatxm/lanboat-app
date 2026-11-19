package native

import (
	"strings"
	"testing"

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
	u.editProxy("port")
	pump()
	v.inputs["proxy.port"].edit("1089")
	if u.state.Config.Settings.Port == 1089 {
		t.Fatal("port draft applied")
	}
	u.saveProxySection("port")
	waitJob(t, u)
	pump()
	if u.popup != nil || v.body != summary || u.state.Config.Settings.Port != 1089 {
		t.Fatal("port save")
	}
	u.editProxy("sharing")
	pump()
	button("允许局域网设备连接").click()
	pump()
	button("访问认证").click()
	pump()
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
