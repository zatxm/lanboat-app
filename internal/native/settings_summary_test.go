package native

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestNativeWindowSettingsSummaryAndTextScroll(t *testing.T) {
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
	u.navigate("settings")
	pump()
	u.openTCP()
	pump()
	if v.inputs["tcp.fingerprint"] != nil {
		t.Fatal("TCP summary contains inputs")
	}
	body := v.body
	u.editTCP()
	pump()
	v.inputs["tcp.fingerprint"].edit("Chrome150")
	v.inputs["tcp.maxRead"].edit("8192")
	if u.state.Config.Settings.TCP.DefaultFingerprint() == "Chrome150" {
		t.Fatal("TCP draft applied")
	}
	u.closePopup()
	pump()
	u.editTCP()
	pump()
	if v.inputs["tcp.fingerprint"].value == "Chrome150" {
		t.Fatal("cancel applied draft")
	}
	v.inputs["tcp.fingerprint"].edit("Chrome150")
	v.inputs["tcp.maxRead"].edit("8192")
	next := v.next
	u.saveTCP()
	waitJob(t, u)
	pump()
	if u.popup != nil || v.next != next || v.body != body || u.state.Config.Settings.TCP.ReadLimit() != 8192 || u.state.Config.Settings.TCP.DefaultFingerprint() != "Chrome150" {
		t.Fatal("TCP save rebuilt page or failed")
	}
	u.openPage("config")
	pump()
	readonly := u.state.Config.Files[0]
	readonly.Content = "[Rule]\n" + strings.Repeat("DOMAIN,example.com,DIRECT\n", 700) + "FINAL,DIRECT"
	u.editConfigFile(readonly)
	pump()
	text := v.inputs["fileContent"]
	original := text.nativeText()
	text.scrollPosition(200)
	pump()
	if text.scrollPosition(-1) < 1 || v.scroll.scrollPosition(-1) != 0 {
		t.Fatal("readonly config must scroll internally", text.scrollPosition(-1), v.scroll.scrollPosition(-1))
	}
	text.edit("wrong")
	if text.nativeText() != original {
		t.Fatal("readonly buffer changed")
	}
	if err := u.backend.SaveConfigFile("", "long.conf", "[Rule]\n"+strings.Repeat("DOMAIN,example.com,DIRECT\n", 700)+"FINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	for _, f := range u.state.Config.Files {
		if f.Name == "long.conf" {
			u.editConfigFile(f)
		}
	}
	pump()
	text = v.inputs["fileContent"]
	text.scrollPosition(300)
	pump()
	if text.scrollPosition(-1) < 1 {
		t.Fatal("editable text must scroll", text.scrollPosition(-1))
	}
	text.edit("[Rule]\nFINAL,REJECT")
	if u.field("fileContent").Text() != text.nativeText() {
		t.Fatal("text buffer callback lost edits")
	}
}
