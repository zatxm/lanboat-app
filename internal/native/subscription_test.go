package native

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowSubscriptionForm(t *testing.T) {
	u := testUI(t)
	fixture, err := os.ReadFile("../profile/testdata/karing-subscription.txt")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.UserAgent() != "CustomAgent" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("subscription-encryption", "true")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	u.editNode(profile.Node{Type: "subscribe"})
	pump()
	for _, key := range []string{"protocol", "subURL", "subUDP", "subInsecure", "subFastOpen", "subMax", "subUA.choice", "subPassword", "subFilter", "subKeepRules", "name"} {
		if v.inputs[key] == nil {
			t.Fatal("missing subscription field", key)
		}
	}
	selector := v.inputs["subUA.choice"]
	if selector.value != "" || len(selector.options) != len(profile.SubscriptionUserAgents)+2 ||
		!strings.Contains(selector.options[0].label, "Lanboat/") {
		t.Fatal("UserAgent should automatically show the default and offer all presets")
	}
	v.inputs["subURL"].edit(server.URL + "?flag=loon")
	pump()
	if !strings.Contains(selector.options[0].label, "Loon/1.0") {
		t.Fatal("Loon link did not update the automatic user agent")
	}
	for _, w := range v.widgets {
		if w.text == "取消" || w.text == "连接信息" || w.text == "名称" || w.text == "名称 / 备注" {
			t.Fatal("obsolete form element", w.text)
		}
	}
	checkProtocolLayout := func() {
		t.Helper()
		w := v.inputs["protocol"]
		if w.fieldTitle != "协议类型" || w.pointer.Bounds().W == 0 {
			t.Fatal("protocol selector is missing its floating label or width")
		}
	}
	checkProtocolLayout()
	edge := v.inputs["protocol"].pointer.Bounds()
	inputHeight := v.inputs["subURL"].pointer.Bounds().H
	for _, key := range []string{"protocol", "subUA.choice"} {
		if height := v.inputs[key].content.Bounds().H; height != inputHeight {
			t.Errorf("%s height %v should match the input height %v", key, height, inputHeight)
		}
	}
	remark := v.inputs["name"]
	if remark.kind != "text" || remark.pointer.Bounds().H < inputHeight+20 {
		t.Fatal("remark must provide at least two lines of input")
	}
	for _, key := range []string{"subUA.choice", "subFilter", "subURL", "name"} {
		w := v.inputs[key]
		r := w.pointer.Bounds()
		if r.X != edge.X || r.W != edge.W || w.fieldTitle == "" {
			t.Errorf("misaligned floating field %s: %v, expected edges of %v", key, r, edge)
		}
	}
	for _, key := range []string{"subUDP", "subKeepRules"} {
		toggle := v.inputs[key]
		r := toggle.pointer.Bounds()
		if toggle.parent.pointer.Bounds().X != edge.X || r.X <= edge.X {
			t.Errorf("switch %s should have a left-aligned caption", key)
		}
	}
	for _, key := range []string{"subUDP", "subInsecure", "subFastOpen"} {
		w := v.inputs[key]
		if len(w.options) != 0 || !strings.Contains(w.classes, "switch") || u.field(key).Text() != "false" {
			t.Fatal("subscription switch is not disabled by default", key)
		}
		w.click()
		pump()
		if u.field(key).Text() != "true" {
			t.Fatal("subscription switch did not enable", key)
		}
		w.click()
		pump()
	}
	if !strings.Contains(v.inputs["subMax"].tooltip, "随机丢弃") {
		t.Fatal("missing options or limit tooltip")
	}
	next := v.next
	v.inputs["subKeepRules"].click()
	pump()
	if u.field("subKeepRules").Text() != "true" || !strings.Contains(v.inputs["subKeepRules"].classes, "switch-on") || next != v.next {
		t.Fatal("keep rules switch rebuilt form or failed")
	}
	v.inputs["subUA.choice"].edit("custom")
	pump()
	checkAgentLayout := func() {
		t.Helper()
		input := v.inputs["subUA"]
		got := input.pointer.Bounds()
		want := v.inputs["subUA.choice"].pointer.Bounds()
		if !input.visible || got.X != want.X || got.W != want.W || got.Y < want.Y+want.H {
			t.Fatalf("custom UserAgent input should align below its dropdown: got %v, dropdown %v", got, want)
		}
	}
	checkAgentLayout()
	const notes = "Subscription notes\nSecond line"
	for key, value := range map[string]string{"name": notes, "subURL": server.URL, "subUDP": "false", "subInsecure": "true", "subFastOpen": "true", "subMax": "5", "subUA": "CustomAgent", "subPassword": "decrypt-secret", "subFilter": "HK|SG"} {
		if key == "subUDP" || key == "subInsecure" || key == "subFastOpen" {
			if u.field(key).Text() != value {
				v.inputs[key].click()
			}
		} else {
			v.inputs[key].edit(value)
		}
	}
	u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
	deadline := time.Now().Add(5 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if u.busy || u.err != "" {
		t.Fatal("language update", u.err)
	}
	assertEnglishWidgets(t, v)
	checkProtocolLayout()
	checkAgentLayout()
	if u.field("name").Text() != notes || v.inputs["subPassword"].nativeText() != "decrypt-secret" || next != v.next {
		t.Fatal("translation lost subscription fields")
	}
	v.save.click()
	deadline = time.Now().Add(5 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if u.busy || u.err != "" || u.editor != "" || len(u.state.Config.Nodes) != 1 || len(u.state.Config.Files) != 1 || len(u.state.Config.RuleOverlays) != 1 {
		t.Fatal("native encrypted subscription save", u.err)
	}
	sub := u.state.Config.Subscriptions[0]
	overlay := u.state.Config.RuleOverlays[0]
	if overlay.SubscriptionID != sub.ID || len(overlay.NodeIDs) != 1 || overlay.NodeIDs[0] != u.state.Config.Nodes[0].ID || overlay.Content == "" {
		t.Fatal("subscription rules are not scoped to its node")
	}
	if sub.Password != "" || sub.Name != notes || sub.UDP != "false" || !sub.KeepRules {
		t.Fatal("form options not saved or password exposed")
	}
	u.editSubscription(sub)
	pump()
	if u.field("subUDP").Text() != "false" || u.field("subMax").Text() != "5" || u.field("subPassword").Text() != "" {
		t.Fatal("subscription editor lost options")
	}
	v.save.click()
	deadline = time.Now().Add(5 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if u.busy || u.err != "" || len(u.state.Config.Subscriptions) != 1 || len(u.state.Config.RuleOverlays) != 1 {
		t.Fatal("editing lost decryption password or duplicated subscription", u.err)
	}
	u.navigate("config")
	u.editConfigFile(u.state.Config.Files[0])
	pump()
	before := u.field("fileContent").Text()
	v.inputs["fileContent"].edit("[Rule]\nFINAL,DIRECT")
	if u.editor != "config-view" || v.save.visible || u.field("fileContent").Text() != before {
		t.Fatal("default config is writable")
	}
}
