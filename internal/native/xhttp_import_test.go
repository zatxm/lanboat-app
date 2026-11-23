package native

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func TestNativeWindowXHTTPAndImport(t *testing.T) {
	u := testUI(t)
	u.notify = requestIdle
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	active = v
	defer func() { active = nil; v.dispose() }()
	for _, kind := range []string{"ss", "vmess", "vless", "trojan"} {
		u.editNode(profile.Node{Type: kind})
		pump()
		key := "opt.network"
		if kind == "ss" {
			key = "opt.obfs"
		}
		v.inputs[key].edit("xhttp")
		pump()
		for _, name := range []string{"NoGRPCHeader", "XPaddingObfsMode", "XPaddingBytes", "XPaddingKey", "XPaddingHeader", "XPaddingPlacement", "XPaddingMethod", "UplinkHTTPMethod", "SessionPlacement", "SessionKey", "SeqPlacement", "SeqKey", "UplinkDataPlacement", "UplinkDataKey", "UplinkChunkSize", "ScMaxEachPostBytes", "MaxConcurrency", "MaxConnections", "CMaxReuseTimes", "HMaxReusableSecs", "HKeepAlivePeriod", "DownloadTargetHost", "DownloadTargetPort", "DownloadServerName", "DownloadHTTPHost"} {
			if v.inputs["opt.xhttp."+name] == nil {
				t.Fatalf("%s missing %s", kind, name)
			}
		}
		if v.inputs["opt.xhttp.XPaddingBytes"].placeholder != "100-1000" || u.field("opt.xhttp.XPaddingBytes").Text() != "" || u.field("opt.xhttp.XPaddingPlacement").Text() != "queryInHeader" {
			t.Fatal("incorrect defaults", kind)
		}
	}
	u.importNodes()
	pump()
	if v.inputs["nodeImportMethod"] == nil || len(v.inputs["nodeImportMethod"].options) != 3 || v.inputs["content"] == nil || !v.back.visible || !v.save.visible {
		t.Fatal("import editor missing controls")
	}
	for _, c := range v.controls {
		if c.widget.text == "取消" {
			t.Fatal("import still has cancel")
		}
	}
	v.inputs["content"].edit("draft")
	v.inputs["nodeImportMethod"].edit("file")
	pump()
	if u.field("content").Text() != "draft" {
		t.Fatal("switching mode lost draft")
	}
}

func TestSubscriptionImportSwitchAndRuleScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "proxies:\n  - {name: one, type: socks5, server: localhost, port: 1080}\nrules:\n  - DOMAIN,retained.example,DIRECT\n")
	}))
	defer server.Close()
	u := testUI(t)
	v, err := newView(u)
	if err != nil {
		t.Fatal(err)
	}
	defer v.dispose()

	for _, keep := range []bool{false, true} {
		u.importNodes()
		v.flush()
		method := v.inputs["nodeImportMethod"]
		for _, mode := range []string{"manual", "file", "subscription"} {
			method = v.inputs["nodeImportMethod"]
			method.edit(mode)
			v.flush()
			v.tester.Step()
			found := false
			for _, w := range v.widgets {
				if w.text == "导入内容" {
					t.Fatal("redundant input label")
				}
				if strings.HasPrefix(w.text, "兼容Lanboat") {
					hint := w.pointer.Bounds()
					found = hint.H > 0 && hint.Y+hint.H <= v.inputs["nodeImportMethod"].pointer.Bounds().Y
				}
			}
			if !found {
				t.Fatalf("compatibility hint missing above %s", mode)
			}
		}
		address, toggle := v.inputs["nodeImportURL"], v.inputs["nodeImportRules"]
		if address.parent != toggle.parent || toggle.pointer.Checked() || v.inputs["content"] != nil {
			t.Fatal("subscription mode should start with a URL and an unchecked switch in one row")
		}
		if keep {
			toggle.click()
		}
		address.edit(server.URL)
		v.save.click()
		waitJob(t, u)
		if u.err != "" || u.editor != "" {
			t.Fatalf("import failed: %s", u.err)
		}
	}
	c := u.backend.GetState().Config
	if len(c.Subscriptions) != 2 || len(c.Nodes) != 2 || len(c.RuleOverlays) != 1 {
		t.Fatalf("unexpected import counts: %d/%d/%d", len(c.Subscriptions), len(c.Nodes), len(c.RuleOverlays))
	}
	first, second := c.Subscriptions[0], c.Subscriptions[1]
	rule := c.RuleOverlays[0]
	if first.KeepRules || !second.KeepRules || c.Nodes[0].Source != first.ID || c.Nodes[1].Source != second.ID || rule.SubscriptionID != second.ID || !slices.Equal(rule.NodeIDs, []string{c.Nodes[1].ID}) || !strings.Contains(rule.Content, "retained.example") {
		t.Fatal("imported rules did not follow the per-subscription switch")
	}
	if _, err := u.backend.ImportSubscription("file:///invalid", true); err == nil || len(u.backend.GetState().Config.Subscriptions) != 2 {
		t.Fatal("invalid URL changed subscriptions")
	}
}
