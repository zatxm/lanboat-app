package native

import (
	"strings"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowShadowsocks(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	u.editNode(profile.Node{Type: "ss"})
	pump()
	if u.draft.Method != "aes-256-cfb" || len(v.inputs["method"].options) != 38 || len(v.inputs["plugin"].options) != 6 || u.field("opt.udp").Text() != "true" || u.field("opt.ota").Text() != "false" || u.field("opt.tfo").Text() != "false" || u.field("opt.uot-version").Text() != "0" {
		t.Fatal("SS defaults or choices")
	}
	if v.inputs["server"].placeholder != "域名或IP" {
		t.Fatal("missing address placeholder")
	}
	v.inputs["password"].edit("typed-password")
	for _, mode := range profile.SSObfuscations {
		v.inputs["opt.obfs"].edit(mode)
		pump()
		if (mode != "none") != (v.inputs["opt.obfs-host"] != nil) {
			t.Fatal("conditional obfuscation fields", mode)
		}
		if u.field("password").Text() != "typed-password" {
			t.Fatal("obfuscation change lost password")
		}
	}
	v.inputs["opt.obfs"].edit("none")
	pump()
	for _, plugin := range profile.SSPlugins {
		v.inputs["plugin"].edit(plugin)
		pump()
		for _, field := range profile.SSPluginFields(u.draft.Plugin) {
			if v.inputs["opt."+field.Key] == nil {
				t.Fatal("plugin field missing", plugin, field.Key)
			}
		}
	}
	v.inputs["opt.plugin-opts.server"].edit("cover.example")
	v.inputs["opt.plugin-opts.port"].edit("443")
	v.inputs["opt.plugin-opts.host"].edit("cover.example")
	v.inputs["server"].edit("example.com")
	v.inputs["opt.tfo"].click()
	v.inputs["opt.uot-version"].edit("2")
	v.save.click()
	deadline := time.Now().Add(5 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if u.busy || u.err != "" || len(u.state.Config.Nodes) != 1 {
		t.Fatal("SS save", u.err)
	}
	n := u.state.Config.Nodes[0]
	if n.Name != "example.com:8388" || n.Plugin != "shadow-tls" || n.Password != "" || n.TextOption("plugin-opts.password") != "" || n.TextOption("tfo") != "true" {
		t.Fatal("SS settings or masking")
	}
	u.editNode(n)
	pump()
	u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
	deadline = time.Now().Add(5 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	assertEnglishWidgets(t, v)
	for _, control := range v.controls {
		if strings.Contains(control.widget.classes, "switch") && control.widget.kind != "button" {
			t.Fatal("switch kind")
		}
	}
	link, err := u.backend.ExportNode(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	full, err := profile.ParseURI(link)
	if err != nil || full.Password != "typed-password" {
		t.Fatal("SS credentials or options lost", err)
	}
}

func TestNativeWindowSSRAndPluginFields(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	u.editNode(profile.Node{Type: "ssr"})
	pump()
	if len(v.inputs["opt.protocol"].options) != 14 || len(v.inputs["opt.obfs"].options) != 6 {
		t.Fatal("SSR protocol or obfuscation choices")
	}
	v.inputs["password"].edit("saved-draft")
	for _, mode := range profile.SSRObfuscations {
		v.inputs["opt.obfs"].edit(mode)
		pump()
		want := mode != "plain" && mode != "random_head"
		if want != (v.inputs["opt.obfs-param"] != nil) {
			t.Fatal(mode, "SSR fields")
		}
	}
	v.inputs["plugin"].edit("cloak")
	pump()
	v.inputs["opt.plugin-opts.Transport"].edit("websocket")
	pump()
	if v.inputs["opt.plugin-opts.sni"] == nil || v.inputs["opt.plugin-opts.skip-cert-verify"] == nil {
		t.Fatal("Cloak WebSocket fields")
	}
	v.inputs["plugin"].edit("kcptun")
	pump()
	v.inputs["opt.plugin-opts.mode"].edit("custom")
	pump()
	if v.inputs["opt.plugin-opts.interval"] == nil || v.inputs["opt.plugin-opts.nc"] == nil || u.field("password").Text() != "saved-draft" {
		t.Fatal("custom KCP fields or draft preservation")
	}
	v.inputs["plugin"].edit("v2ray-plugin")
	pump()
	for _, option := range v.inputs["opt.plugin-opts.mode"].options {
		if option.value == "fast" {
			t.Fatal("unsupported fast mode")
		}
	}
}

func TestNativeWindowShadowTLS(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	choices := serverTypes()
	for i, choice := range choices {
		if choice.value == "ssr" && (i+1 == len(choices) || choices[i+1].value != "shadowtls") {
			t.Fatal("ShadowTLS must follow ShadowsocksR")
		}
	}
	u.editNode(profile.Node{Type: "shadowtls"})
	pump()
	if v.inputs["server"].placeholder != "域名或IP" || v.inputs["opt.tls"] != nil || u.field("opt.version").Text() != "1" || u.field("opt.skip-cert-verify").Text() != "false" || u.field("opt.client-fingerprint").Text() != "none" || len(v.inputs["opt.client-fingerprint"].options) != 12 {
		t.Fatal("ShadowTLS form defaults")
	}
	v.inputs["server"].edit("example.com")
	v.inputs["nodePort"].edit("443")
	v.inputs["opt.version"].edit("3")
	pump()
	v.inputs["opt.host"].edit("cover.example")
	v.inputs["opt.alpn"].edit("h2,http/1.1")
	v.inputs["opt.client-fingerprint"].edit("qq")
	v.save.click()
	waitJob(t, u)
	pump()
	if u.err != "" || len(u.state.Config.Nodes) != 1 || u.state.Config.Nodes[0].Kind() != "shadowtls" {
		t.Fatal("optional password save", u.err)
	}
	n := u.state.Config.Nodes[0]
	if n.TextOption("version") != "3" || n.TextOption("host") != "cover.example" || n.TextOption("client-fingerprint") != "qq" {
		t.Fatal("standalone settings lost")
	}
	u.editNode(n)
	pump()
	u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
	waitJob(t, u)
	pump()
	assertEnglishWidgets(t, v)
	u.changeSettings(func(s *profile.Settings) { s.Language = "zh" })
	waitJob(t, u)
	pump()
	u.navigate("home")
	u.editNode(profile.Node{Type: "ss"})
	pump()
	v.inputs["password"].edit("inner")
	v.inputs["server"].edit("ss.example")
	v.inputs["plugin"].edit("shadow-tls")
	pump()
	for _, field := range profile.SSPluginFields("shadow-tls") {
		if v.inputs["opt."+field.Key] == nil {
			t.Fatal("missing shared plugin field", field.Key)
		}
	}
	v.save.click()
	waitJob(t, u)
	pump()
	if u.popup == nil || u.popup.kind != "message" {
		t.Fatal("missing required plugin address accepted")
	}
	u.closePopup()
	v.inputs["opt.plugin-opts.server"].edit("shadow.example")
	v.inputs["opt.plugin-opts.port"].edit("443")
	v.inputs["opt.plugin-opts.password"].edit("outer")
	v.inputs["opt.plugin-opts.alpn"].edit("h2")
	v.save.click()
	waitJob(t, u)
	pump()
	if u.err != "" || len(u.state.Config.Nodes) != 2 {
		t.Fatal("plugin save", u.err)
	}
	n = u.state.Config.Nodes[1]
	u.editNode(n)
	pump()
	if u.field("opt.plugin-opts.password").Text() != "" || u.field("opt.plugin-opts.alpn").Text() != "h2" {
		t.Fatal("plugin reedit")
	}
	v.save.click()
	waitJob(t, u)
	pump()
	if u.err != "" {
		t.Fatal("plugin secret preservation", u.err)
	}
}
