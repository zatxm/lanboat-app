package native

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestNativeWindowSettingsSummaryAndTextScroll(t *testing.T) {
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
	var content strings.Builder
	content.WriteString("[Rule]\n")
	for i := range 700 {
		fmt.Fprintf(&content, "DOMAIN,fixture%03d.test,DIRECT\n", i)
	}
	content.WriteString("FINAL,DIRECT")
	if err := u.backend.SaveConfigFile("", "long.conf", content.String()); err != nil {
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
	before := v.tester.Snapshot()
	text.scrollPosition(300)
	pump()
	if bytes.Equal(before.Pix, v.tester.Snapshot().Pix) || v.scroll.scrollPosition(-1) != 0 {
		t.Fatal("editable text must scroll internally")
	}
	text.edit("[Rule]\nFINAL,REJECT")
	if u.field("fileContent").Text() != text.nativeText() {
		t.Fatal("text buffer callback lost edits")
	}
}
