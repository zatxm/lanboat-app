package native

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNativeWindowConnectedEditorLocksOnlyActiveNode(t *testing.T) {
	if os.Getenv("LANBOAT_FLTK_TEST") != "1" {
		t.Skip("isolated display required")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	u.status.Connected = true
	u.status.ActiveNode = "active"
	n := profile.Node{ID: "active", Type: "hysteria2", Name: "fixture", Server: "127.0.0.1", Port: 443, Password: "fixture"}
	u.editNode(n)
	v.render()
	pump()
	if v.inputs["server"].enabled || v.save.enabled || !v.back.enabled {
		t.Fatal("active editor must be readonly with working Back")
	}
	n.ID = "other"
	u.editNode(n)
	v.render()
	pump()
	if !v.inputs["server"].enabled || !v.save.enabled || !v.inputs["opt.udp"].enabled {
		t.Fatal("unrelated editor locked while connected")
	}
	for _, w := range v.widgets {
		if strings.HasPrefix(w.text, "混淆") && w.text != "混淆" {
			t.Fatal("HY2 obfuscation label", w.text)
		}
	}
	u.navigate("settings")
	v.render()
	pump()
	u.openProxy()
	pump()
	u.editProxy("port")
	v.refresh()
	pump()
	if !v.inputs["proxy.port"].enabled {
		t.Fatal("connected settings remain locked")
	}
}
