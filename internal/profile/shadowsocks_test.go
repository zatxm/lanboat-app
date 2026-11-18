package profile

import (
	"reflect"
	"testing"
)

func TestShadowsocksMethodsAndAdvancedRoundTrip(t *testing.T) {
	if len(Methods) != 38 || len(SSObfuscations) != 10 || len(SSPlugins) != 6 {
		t.Fatal("incomplete Shadowsocks choices")
	}
	for _, method := range Methods {
		n := Node{Type: "ss", Name: "test", Server: "example.com", Port: 443, Method: method, Password: "secret"}
		if len(method) > 5 && method[:5] == "2022-" {
			n.Password = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
			if method == "2022-blake3-aes-128-gcm" {
				n.Password = "AAAAAAAAAAAAAAAAAAAAAA=="
			}
		}
		if err := n.Validate(); err != nil {
			t.Fatal(method, err)
		}
	}
	n := Node{Type: "ss", Name: "advanced", Server: "example.com", Port: 443, Method: DefaultSSMethod, Password: "secret", Plugin: "shadow-tls"}
	n.SetOption("obfs", "none")
	n.SetOption("udp", true)
	n.SetOption("tfo", true)
	n.SetOption("uot-version", "2")
	n.SetOption("plugin-opts.host", "cover.example")
	n.SetOption("plugin-opts.password", "plugin-secret")
	n.SetOption("plugin-opts.version", "3")
	parsed, err := ParseURI(URI(n))
	if err != nil {
		t.Fatal(err)
	}
	parsed.ID = ""
	if !reflect.DeepEqual(n, parsed) {
		t.Fatal("advanced SS options lost in share link")
	}
	masked := n.Masked()
	if masked.Password != "" || masked.TextOption("plugin-opts.password") != "" || masked.TextOption("uot-version") != "2" {
		t.Fatal("mask lost settings or leaked credentials")
	}
	n.SetOption("obfs", "wss")
	n.SetOption("obfs-host", "front.example")
	n.SetOption("obfs-path", "/transport")
	if n.Masked().TextOption("obfs-path") != "/transport" {
		t.Fatal("mask discarded conditional obfuscation settings")
	}
}

func TestLegacyPluginOptionsDoNotOverrideEditedFields(t *testing.T) {
	n := Node{Type: "ss", Plugin: "v2ray-plugin", PluginOpts: "host=old.example;path=/old;tls"}
	NormalizeSS(&n)
	n.SetOption("plugin-opts.host", "edited.example")
	n.SetOption("plugin-opts.tls", false)
	NormalizeSS(&n)
	if n.TextOption("plugin-opts.host") != "edited.example" || n.TextOption("plugin-opts.tls") != "false" {
		t.Fatal("legacy options overwrote edited form values")
	}
}
