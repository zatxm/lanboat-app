package native

import (
	"encoding/base64"
	"github.com/zatxm/lanboat-app/internal/profile"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeWindowAmneziaRandomize(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	for _, choice := range serverTypes() {
		if choice.value == "amneziawg" {
			t.Fatal("merged protocol remains selectable")
		}
	}
	u.editNode(profile.Node{Type: "wireguard"})
	pump()
	v.inputs["server"].edit("example.com")
	v.inputs["opt.obfs"].edit("amneziawg")
	pump()
	v.inputs["opt.amnezia-wg-option.i1"].edit("<r 40>")
	randomize := func() {
		t.Helper()
		for _, c := range v.controls {
			if c.widget.text == "一键随机" {
				c.widget.click()
				pump()
				return
			}
		}
		t.Fatal("randomize button is missing")
	}
	randomize()
	first := u.field("opt.amnezia-wg-option.header-protection-key").Text()
	if first == "" || u.field("opt.amnezia-wg-option.h1").Text() == "" || u.field("opt.amnezia-wg-option.i1").Text() != "<r 40>" || u.field("server").Text() != "example.com" {
		t.Fatal("randomize lost draft values or did not fill parameters")
	}
	randomize()
	if u.field("opt.amnezia-wg-option.header-protection-key").Text() == first {
		t.Fatal("randomize did not refresh the key")
	}
	v.inputs["opt.obfs"].edit("none")
	pump()
	v.inputs["opt.obfs"].edit("amneziawg")
	pump()
	if u.field("opt.amnezia-wg-option.header-protection-key").Text() == "" {
		t.Fatal("switching obfuscation lost generated parameters")
	}
	u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
	waitJob(t, u)
	pump()
	assertEnglishWidgets(t, v)
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	v.inputs["opt.private-key"].edit(key)
	v.inputs["opt.public-key"].edit(key)
	v.inputs["opt.heartbeat"].edit("20-30")
	wantKey := u.field("opt.amnezia-wg-option.header-protection-key").Text()
	v.save.click()
	waitJob(t, u)
	pump()
	if u.err != "" || len(u.state.Config.Nodes) != 1 {
		t.Fatal("generated configuration did not save", u.err)
	}
	saved := u.state.Config.Nodes[0]
	if saved.TextOption("amnezia-wg-option.header-protection-key") != "" {
		t.Fatal("saved protection key leaked into UI state")
	}
	u.editNode(saved)
	pump()
	v.inputs["name"].edit("renamed AWG")
	v.save.click()
	waitJob(t, u)
	uri, err := u.backend.ExportNode(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := profile.ParseURI(uri)
	if err != nil || exported.TextOption("amnezia-wg-option.header-protection-key") != wantKey || exported.TextOption("heartbeat") != "20-30" {
		t.Fatal("editing lost generated configuration", err)
	}
}

func TestNativeWindowSpecialForms(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	for _, kind := range []string{"sudoku", "naive", "tailscale"} {
		u.navigate("home")
		u.editNode(profile.Node{Type: kind})
		pump()
		if kind == "tailscale" {
			if v.inputs["server"] != nil || v.inputs["nodePort"] != nil || v.inputs["password"] != nil {
				t.Fatal("Tailscale has unrelated fields")
			}
			v.inputs["opt.auth-key"].edit("tskey-auth-fixture")
			v.inputs["opt.exit-node"].edit("100.64.0.1")
		} else {
			if v.inputs["server"].placeholder != "域名或IP" || v.inputs["nodePort"].placeholder != "1–65535" {
				t.Fatal("shared required address/port fields", kind)
			}
			v.inputs["server"].edit("example.com")
		}
		if kind == "sudoku" {
			if v.inputs["password"] != nil || u.field("opt.table-type").Text() != "prefer_entropy" || u.field("opt.enable-pure-downlink").Text() != "true" || u.field("opt.http-mask").Text() != "false" {
				t.Fatal("Sudoku defaults")
			}
			v.inputs["opt.key"].edit("fixture-key")
			v.inputs["opt.http-mask"].click()
			pump()
			v.inputs["opt.http-mask-host"].edit("front.example")
			v.inputs["opt.path-root"].edit("prefix")
			v.inputs["opt.http-mask-mode"].edit("legacy")
			pump()
			if v.inputs["opt.http-mask-tls"] != nil || v.inputs["opt.http-mask-host"] != nil || v.inputs["opt.path-root"] == nil {
				t.Fatal("Sudoku legacy field visibility")
			}
			v.inputs["opt.http-mask-mode"].edit("ws")
			pump()
			if u.field("opt.http-mask-host").Text() != "front.example" || u.field("opt.path-root").Text() != "prefix" {
				t.Fatal("HTTP mask mode switch lost draft")
			}
			v.inputs["opt.http-mask"].click()
			pump()
			if v.inputs["opt.http-mask-mode"] != nil {
				t.Fatal("disabled HTTP mask settings remain visible")
			}
		}
		if kind == "naive" {
			if u.field("opt.insecure-concurrency").Text() != "0" || len(v.inputs["opt.uot-version"].options) != 3 {
				t.Fatal("Naive defaults")
			}
			v.inputs["opt.username"].edit("fixture-user")
			v.inputs["password"].edit("fixture-password")
			v.inputs["opt.extra-headers"].edit("X-Fixture: value")
		}
		u.saveEditor()
		waitJob(t, u)
		pump()
		if u.err != "" || u.editor != "" {
			t.Fatal("special form save", kind, u.err)
		}
		saved := u.state.Config.Nodes[len(u.state.Config.Nodes)-1]
		u.editNode(saved)
		pump()
		v.inputs["name"].edit("renamed " + kind)
		u.saveEditor()
		waitJob(t, u)
		uri, err := u.backend.ExportNode(saved.ID)
		if err != nil {
			t.Fatal(err)
		}
		node, err := profile.ParseURI(uri)
		if err != nil || node.Kind() != kind {
			t.Fatal("edited form export", kind, err)
		}
		if kind == "sudoku" && (node.TextOption("key") != "fixture-key" || node.TextOption("http-mask-host") != "front.example") {
			t.Fatal("saved Sudoku credentials or hidden values lost")
		}
		if kind == "naive" && node.TextOption("extra-headers") != "X-Fixture: value" {
			t.Fatal("saved headers lost")
		}
		u.editNode(node.Masked())
		u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
		waitJob(t, u)
		pump()
		assertEnglishWidgets(t, v)
	}
}

func TestLatencyJobDoesNotBlockOrReplacePageState(t *testing.T) {
	u := testUI(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = u.backend.SaveNode(profile.Node{Name: "test", Server: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Method: "aes-256-gcm", Password: "fixture"}); err != nil {
		t.Fatal(err)
	}
	u.changeSettings(func(s *profile.Settings) { s.TestTimeout = 1 })
	waitJob(t, u)
	u.state = u.backend.GetState()
	u.testVisibleNodes()
	if u.busy || !u.testing {
		t.Fatal("latency job took exclusive UI lock")
	}
	u.navigate("settings")
	u.changeSettings(func(s *profile.Settings) { s.Mode = "direct" })
	waitJob(t, u)
	if u.page != "settings" || u.state.Config.Settings.Mode != "direct" || len(u.delays) != 1 {
		t.Fatal("latency job replaced current page or settings", u.err)
	}
}
func TestNativeWindowDetailedProtocolForms(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	for _, kind := range []string{"vmess", "vless", "relay", "socks5", "socks5s", "http", "https", "http2", "http3", "trojan", "hysteria", "hysteria2", "anytls", "tuic", "juicity", "ssh", "wireguard", "openconnect", "trusttunnel", "masque", "snell", "mieru", "brook", "lua"} {
		u.navigate("home")
		u.editNode(profile.Node{Type: kind})
		pump()
		if v.inputs["server"].placeholder != "域名或IP" || v.inputs["name"] == nil {
			t.Fatal("missing shared fields", kind)
		}
		if kind == "trusttunnel" && v.inputs["opt.authgroup"] != nil {
			t.Fatal("TrustTunnel has no group")
		}
		if kind == "vmess" || kind == "vless" {
			if u.field("opt.tls").Text() != "false" || u.field("opt.udp").Text() != "false" {
				t.Fatal("defaults", kind)
			}
			if kind == "vmess" && (u.field("opt.alterId").Text() != "" || v.inputs["opt.alterId"].placeholder != "如果为0,AEAD启用") {
				t.Fatal("Alter ID hint must be a placeholder")
			}
			v.inputs["opt.uuid"].edit("12345678-1234-4234-8234-123456789012")
			for _, network := range []string{"http", "h2", "ws", "httpupgrade", "mkcp", "grpc", "quic", "xhttp", "tcp"} {
				v.inputs["opt.network"].edit(network)
				pump()
				if network != "tcp" {
					groups := 0
					for _, w := range v.widgets {
						if strings.Contains(w.classes, "option-group") {
							groups++
						}
					}
					if groups == 0 {
						t.Fatal("dynamic transport has no group", network)
					}
				}
			}
			v.inputs["opt.tls"].click()
			pump()
		}
		if profile.NodeTLSEnabled(u.draft) {
			if v.inputs["opt.alpn"] == nil {
				t.Fatal("missing ALPN", kind)
			}
			if kind == "hysteria" || kind == "hysteria2" || kind == "tuic" || kind == "juicity" {
				if kind == "hysteria" || kind == "hysteria2" {
					want := "100"
					if kind == "hysteria2" {
						want = "auto"
					}
					if v.inputs["opt.up"].placeholder != want || u.field("opt.up").Text() != "" || v.inputs["opt.heartbeat"].placeholder != "0" {
						t.Fatal("Hysteria hints must be placeholders", kind)
					}
				}
			} else {
				if len(v.inputs["opt.client-fingerprint"].options) != 18 {
					t.Fatal("fingerprint list", kind)
				}
				v.inputs["opt.fragment"].click()
				pump()
				if u.field("opt.fragment-opts.length-min").Text() != "40" || v.inputs["opt.fragment-opts.packets"] == nil {
					t.Fatal("fragment defaults", kind)
				}
				if kind != "http2" && kind != "http3" {
					v.inputs["opt.alpn"].edit("h2")
					pump()
					if u.field("opt.http2").Text() != "true" {
						t.Fatal("ALPN did not enable HTTP2")
					}
				}
				if kind == "http2" || kind == "http3" {
					if v.inputs["opt.http2"] != nil {
						t.Fatal("HTTP2 toggle should be hidden")
					}
				}
				if w := v.inputs["opt.mutual-auth"]; w != nil {
					w.click()
					pump()
					if v.inputs["opt.pkcs12"] == nil || v.inputs["opt.pkcs12-password"] == nil {
						t.Fatal("mutual authentication fields", kind)
					}
				}
			}
		}
		if w := v.inputs["opt.plugin"]; w != nil {
			w.edit("kcptun")
			pump()
			if v.inputs["opt.plugin-opts.mtu"] == nil {
				t.Fatal("missing plugin fields", kind)
			}
		}

		if kind == "snell" {
			for _, mode := range []string{"http", "tls", "none"} {
				v.inputs["opt.obfs-opts.mode"].edit(mode)
				pump()
				if (v.inputs["opt.obfs-opts.path"] != nil) != (mode == "http") {
					t.Fatal("Snell conditional path")
				}
			}
		}
		if kind == "brook" {
			v.inputs["opt.network"].edit("wss")
			pump()
			if u.field("opt.path").Text() != "/" {
				t.Fatal("Brook path default")
			}
		}
		if kind == "lua" {
			script := filepath.Join(t.TempDir(), "custom.lua")
			if err := os.WriteFile(script, []byte(profile.LuaHTTPExample), 0600); err != nil {
				t.Fatal(err)
			}
			u.importLuaFile(script)
			pump()
			if u.draft.LuaFile() != "custom" || u.field("opt.script").Text() != profile.LuaHTTPExample {
				t.Fatal("Lua local import lost script")
			}
			v.inputs["opt.tls"].click()
			pump()
			if v.inputs["opt.reality-opts.public-key"] == nil {
				t.Fatal("Lua TLS fields")
			}
		}

		u.changeSettings(func(s *profile.Settings) { s.Language = "en" })
		waitJob(t, u)
		pump()
		assertEnglishWidgets(t, v)
	}
	// Save a complete VMess draft after several expanding/collapsing sections.
	u.navigate("home")
	u.editNode(profile.Node{Type: "vmess"})
	pump()
	v.inputs["server"].edit("example.com")
	v.inputs["opt.uuid"].edit("12345678-1234-4234-8234-123456789012")
	v.inputs["opt.network"].edit("mkcp")
	pump()
	v.inputs["opt.kcp.seed"].edit("saved-seed")
	v.inputs["opt.network"].edit("ws")
	pump()
	v.inputs["opt.network"].edit("mkcp")
	pump()
	if u.field("opt.kcp.seed").Text() != "saved-seed" {
		t.Fatal("transport toggle lost draft")
	}
	v.save.click()
	deadline := time.Now().Add(3 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if u.err != "" || len(u.state.Config.Nodes) != 1 {
		t.Fatal("VMess save", u.err)
	}
}

func TestNativeWindowHomeDeleteIconsAndLatencyScope(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	if _, err := u.backend.Import("ss://YWVzLTI1Ni1nY206cGFzcw@127.0.0.1:8388#delete-fixture"); err != nil {
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
	var action *widget
	for _, c := range v.controls {
		if c.widget.text == "删除节点" {
			action = c.widget
		}
	}
	if action == nil || !strings.Contains(action.classes, "icon-only") {
		t.Fatal("missing delete icon")
	}
	action.click()
	pump()
	if u.confirm == nil || u.confirm.kind != "node" {
		t.Fatal("delete did not confirm")
	}
	u.deleteConfirmed()
	deadline := time.Now().Add(3 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if len(u.state.Config.Nodes) != 0 {
		t.Fatal("delete did not remove node", u.err)
	}
	subServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ss://YWVzLTI1Ni1nY206cGFzcw@127.0.0.1:8388#one\nss://YWVzLTI1Ni1nY206cGFzcw@127.0.0.1:8389#two")
	}))
	defer subServer.Close()
	if _, err := u.backend.SaveSubscriptionOptions(profile.Subscription{Name: "Test subscription", URL: subServer.URL, Insecure: "true"}); err != nil {
		t.Fatal(err)
	}
	u.state = u.backend.GetState()
	v.render()
	pump()
	action = nil
	for _, c := range v.controls {
		if c.widget.text == "删除订阅" {
			action = c.widget
		}
	}
	if action == nil {
		t.Fatal("missing subscription delete icon")
	}
	action.click()
	pump()
	if u.confirm == nil || u.confirm.kind != "subscription" {
		t.Fatal("subscription deletion did not confirm")
	}
	u.deleteConfirmed()
	deadline = time.Now().Add(3 * time.Second)
	for u.busy && time.Now().Before(deadline) {
		pump()
		time.Sleep(time.Millisecond)
	}
	pump()
	if len(u.state.Config.Nodes) != 0 || len(u.state.Config.Subscriptions) != 0 {
		t.Fatal("subscription deletion left nodes", u.err)
	}
	u.notice = ""
	u.closePopup()
	pump()
	u.testNotice = "latency-result"
	v.refresh()
	if u.popup != nil {
		t.Fatal("latency completion must stay inline")
	}
	u.closePopup()
	u.navigate("settings")
	pump()
	if v.notice.visible {
		t.Fatal("latency result leaked into settings")
	}
}

func TestNativeWindowFormScreenshots(t *testing.T) {
	dir := os.Getenv("LANBOAT_UI_SHOTS")
	if dir == "" {
		t.Skip("optional isolated visual capture")
	}
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	v.render()
	pump()
	for _, kind := range []string{"shadowtls", "ss", "vmess", "socks5s", "hysteria2", "snell", "mieru", "brook", "lua", "sudoku", "naive", "tailscale"} {
		u.navigate("home")
		u.editNode(profile.Node{Type: kind})
		pump()
		if kind == "ss" {
			v.inputs["plugin"].edit("shadow-tls")
			pump()
		}
		if kind == "vmess" {
			v.inputs["opt.network"].edit("mkcp")
			pump()
			v.inputs["opt.tls"].click()
			pump()
		}
		if kind == "sudoku" {
			v.inputs["opt.http-mask"].click()
			pump()
		}
		v.tester.Step()
		path := filepath.Join(dir, kind+".png")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(file, v.tester.Snapshot())
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
