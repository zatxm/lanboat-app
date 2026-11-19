package native

import (
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
	if v.inputs["nodeImportMethod"] == nil || len(v.inputs["nodeImportMethod"].options) != 2 || v.inputs["content"] == nil || !v.back.visible || !v.save.visible {
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
