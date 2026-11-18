package profile

import "testing"

func TestXHTTPExtraValidation(t *testing.T) {
	node := Node{Type: "ss"}
	node.SetOption("xhttp-extra", `{"headers":{"Authorization":"private"},"xPaddingBytes":"200-500","noGRPCHeader":true}`)
	extra, err := node.XHTTPExtraSettings()
	if err != nil || extra.PaddingMin != 200 || extra.PaddingMax != 500 || !extra.NoGRPCHeader || extra.Headers["Authorization"] != "private" {
		t.Fatal(extra, err)
	}
	for _, invalid := range []string{`{"xPaddingBytes":"500-100"}`, `{"xPaddingBytes":1000000}`, `{"downloadSettings":{}}`} {
		node.SetOption("xhttp-extra", invalid)
		if _, err = node.XHTTPExtraSettings(); err == nil {
			t.Fatal("unsupported extension accepted", invalid)
		}
	}
}

func TestXHTTPMutuallyExclusivePoolLimits(t *testing.T) {
	n := Node{Type: "ss"}
	n.SetOption("xhttp.MaxConcurrency", "16-32")
	n.SetOption("xhttp.MaxConnections", "1")
	if _, err := n.XHTTPExtraSettings(); err == nil {
		t.Fatal("both limits were accepted")
	}
	n.SetOption("xhttp.MaxConnections", "0")
	if _, err := n.XHTTPExtraSettings(); err != nil {
		t.Fatal(err)
	}
}

func TestXHTTPFormOptionsAndValidation(t *testing.T) {
	n := Node{Type: "ss"}
	n.SetOption("obfs", "xhttp")
	n.SetOption("xhttp.XPaddingBytes", "240-320")
	n.SetOption("xhttp.XPaddingObfsMode", true)
	n.SetOption("xhttp.XPaddingPlacement", "cookie")
	n.SetOption("xhttp.SessionPlacement", "query")
	n.SetOption("xhttp.SessionKey", "session")
	n.SetOption("xhttp.UplinkChunkSize", "128")
	e, err := n.XHTTPExtraSettings()
	if err != nil || e.PaddingMin != 240 || e.PaddingMax != 320 || !e.PaddingObfs || e.PaddingPlacement != "cookie" || e.SessionPlacement != "query" || e.SessionKey != "session" || e.ChunkSize != "128" {
		t.Fatal(e, err)
	}
	n.SetOption("xhttp.DownloadTargetPort", "1-2")
	if _, err = n.XHTTPExtraSettings(); err == nil {
		t.Fatal("accepted port range")
	}
}
