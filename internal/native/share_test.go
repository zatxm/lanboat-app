package native

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowSharingPortAndNotifications(t *testing.T) {
	if os.Getenv("LANBOAT_FLTK_TEST") != "1" {
		t.Skip("isolated display required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u := testUI(t)
	u.notify = requestIdle
	if err := u.backend.SaveNode(profile.Node{Type: "ss", Name: "分享测试", Server: "example.invalid", Port: 443, Method: "aes-256-gcm", Password: "fixture-secret"}); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	pump()
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(4 * time.Second)
		for u.busy && time.Now().Before(deadline) {
			pump()
			time.Sleep(time.Millisecond)
		}
		pump()
		if u.busy || u.err != "" {
			t.Fatal("job failed", u.err)
		}
	}
	u.share("node", u.state.Config.Nodes[0].ID, "分享测试")
	wait()
	if u.popup == nil || v.popupRoot == nil || !strings.HasPrefix(u.popup.link, "ss://") || u.popup.bitmap == "" {
		t.Fatal("share modal or QR missing")
	}
	hasCopy := false
	for _, c := range v.controls {
		if c.widget.text == "复制链接" {
			hasCopy = true
		}
	}
	if !hasCopy {
		t.Fatal("copy action missing")
	}
	u.closePopup()
	pump()
	if v.popupRoot != nil {
		t.Fatal("modal did not close")
	}
	u.navigate("settings")
	pump()
	for _, w := range v.widgets {
		if strings.Contains(w.text, "GNOME") || w.text == "Tailscale" || w.text == "保存端口" {
			t.Fatal("removed connection control remains", w.text)
		}
	}
	if v.inputs["proxy.port"] != nil {
		t.Fatal("inline port input remains")
	}
	u.openProxy()
	pump()
	u.editProxy("port")
	pump()
	v.inputs["proxy.port"].edit("not-a-port")
	u.saveProxySection("port")
	v.refresh()
	if u.popup == nil || u.popup.kind != "message" || u.detail != "proxy-settings" {
		t.Fatal("invalid modal port accepted")
	}
	u.closePopup()
	pump()
	v.inputs["proxy.port"].edit("1088")
	u.saveProxySection("port")
	wait()
	if u.detail != "proxy-settings" || u.state.Config.Settings.Port != 1088 {
		t.Fatal("modal port save failed")
	}
	u.notice = "saved-fixture"
	v.refresh()
	pump()
	if u.popup == nil || u.popup.kind != "message" || v.notice.visible {
		t.Fatal("feedback modal missing")
	}
	u.closePopup()
	pump()
	if v.popupRoot != nil {
		t.Fatal("confirmation did not dismiss feedback")
	}
	u.openPage("config")
	pump()
	var first, second, add *widget
	for _, c := range v.controls {
		switch c.widget.text {
		case "测试规则":
			first = c.widget
		case "自定义规则":
			second = c.widget
		case "新增规则配置":
			add = c.widget
		}
	}
	if first == nil || second == nil || add == nil || first.expand || add.expand {
		t.Fatal("routing toolbar stretched")
	}
	x, _, w, _ := first.bounds()
	x2, _, _, _ := second.bounds()
	a, _, aw, _ := add.bounds()
	bx, _, bw, _ := v.body.bounds()
	if x != bx || x2 > x+w+20 || a+aw != bx+bw || a-x2 < 200 {
		t.Fatal("routing toolbar alignment", x, x2, a, aw, bx, bw)
	}
}
