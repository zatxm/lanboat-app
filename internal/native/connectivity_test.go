package native

import (
	"errors"
	"strings"
	"testing"
)

func TestNativeWindowConnectivityAndFeedback(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	u.navigate("settings")
	v.render()
	pump()
	u.openConnectivity()
	pump()
	if v.inputs["testTimeout"] != nil || v.inputs["speed.choice"] != nil {
		t.Fatal("connectivity summary contains editing controls")
	}
	var summary *widget
	for _, w := range v.widgets {
		if w.text == u.state.Config.Settings.SpeedTarget() && w.kind == "label" {
			summary = w
		}
	}
	if summary == nil {
		t.Fatal("applied URL missing")
	}
	u.editConnectivity(true)
	pump()
	choice := v.inputs["speed.choice"]
	choice.edit("custom")
	pump()
	v.inputs["speed.custom"].edit("https://example.com/speed")
	if u.state.Config.Settings.SpeedTarget() == "https://example.com/speed" {
		t.Fatal("draft applied")
	}
	initial := v.next
	u.saveConnectivity(true)
	waitJob(t, u)
	pump()
	if u.popup != nil || summary.text != "https://example.com/speed" || v.next != initial {
		t.Fatal("save rebuilt summary or left modal open")
	}
	if len(choice.options) != 5 || choice.options[4].value != "custom" {
		t.Fatal("custom URL appended to choices")
	}
	u.editConnectivity(false)
	pump()
	if v.inputs["testTimeout"].nativeText() != "15" || v.inputs["testConcurrency"].nativeText() != "20" {
		t.Fatal("defaults")
	}
	v.inputs["test.choice"].edit("custom")
	pump()
	v.inputs["test.custom"].edit("https://example.com/test")
	v.inputs["testTimeout"].edit("22")
	v.inputs["testConcurrency"].edit("7")
	u.saveConnectivity(false)
	waitJob(t, u)
	pump()
	s := u.backend.GetState().Config.Settings
	if s.TestURL != "https://example.com/test" || s.TestTimeout != 22 || s.TestConcurrency != 7 || s.TestMethod != "" || u.popup != nil {
		t.Fatal("latency save")
	}
	u.submit(func() error { return errors.New("failure fixture") }, nil)
	waitJob(t, u)
	pump()
	if u.popup == nil || u.popup.kind != "message" || u.popup.link != "failure fixture" {
		t.Fatal("failure modal")
	}
	u.closePopup()
	pump()
	if summary.text != "https://example.com/speed" {
		t.Fatal("feedback lost summary")
	}
	u.submit(func() error { return nil }, func() { u.back(); u.notice = "quiet navigation fixture" })
	waitJob(t, u)
	pump()
	if u.popup != nil || u.notice != "" {
		t.Fatal("navigation feedback")
	}
}

func TestNativeWindowLatencyUpdatesOnlyLabels(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	if _, err := u.backend.Import("ss://YWVzLTI1Ni1nY206cGFzcw@127.0.0.1:1#latency-fixture"); err != nil {
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
	id := u.state.Config.Nodes[0].ID
	list, search, label, next := v.nodeContainer, v.inputs["search"], v.delayLabels[id], v.next
	u.testVisibleNodes()
	waitJob(t, u)
	pump()
	if u.delays[id] != -1 || u.popup != nil || u.dirty || u.nodesDirty || v.next != next || v.nodeContainer != list || v.inputs["search"] != search || v.delayLabels[id] != label {
		t.Fatal("latency completion rebuilt controls or opened feedback")
	}
	u.openUDP()
	pump()
	if v.inputs["udp.ports"] != nil {
		t.Fatal("UDP summary contains inputs")
	}
	u.editUDP()
	pump()
	if u.field("udp.relay").Text() != "true" || u.field("udp.stun").Text() != "false" || u.field("udp.timeout").Text() != "30" || u.field("udp.ports").Text() != "" {
		t.Fatal("UDP defaults")
	}
	for _, w := range v.widgets {
		if w.text == "超时（秒）" {
			x, _, width, _ := w.bounds()
			inputX, _, _, _ := v.inputs["udp.timeout"].bounds()
			if inputX-x-width > 12 {
				t.Fatal("UDP timeout gap too large", inputX-x-width)
			}
		}
	}
	u.field("udp.ports").SetText("0")
	u.field("udp.stun").SetText("true")
	u.field("udp.timeout").SetText("12")
	next = v.next
	u.saveUDP()
	waitJob(t, u)
	pump()
	if u.backend.GetState().Config.Settings.UDP.Ports != "0" || !u.state.Config.Settings.UDP.BlockSTUN || v.next != next || u.popup != nil || v.inputs["udp.ports"] != nil {
		t.Fatal("UDP save rebuilt page or failed")
	}
	if err := u.backend.SaveConfigFile("", "switch.conf", "[Rule]\nFINAL,DIRECT"); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	u.openPage("config")
	pump()
	var row *widget
	var fileID string
	for _, f := range u.state.Config.Files {
		if f.Name == "switch.conf" {
			fileID = f.ID
		}
	}
	for _, w := range v.widgets {
		if w.text == "switch.conf" && hasClass(w.classes, "routing-file-row") {
			row = w
		}
	}
	if row == nil {
		t.Fatal("rule row missing")
	}
	next = v.next
	u.submit(func() error { return u.backend.ActivateConfigFile(fileID) }, nil)
	waitJob(t, u)
	pump()
	if v.next != next || !strings.Contains(row.classes, "node-selected") {
		t.Fatal("rule activation rebuilt the page")
	}

}
