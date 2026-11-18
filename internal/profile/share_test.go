package profile

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestCommonShareSchemesAndCredentials(t *testing.T) {
	uuid := "12345678-1234-4234-8234-123456789012"
	for _, kind := range []string{"ss", "ssr", "vmess", "vless", "trojan", "hysteria", "hysteria2", "anytls", "tuic", "socks5", "socks5s", "http", "https", "ssh", "brook", "mieru"} {
		t.Run(kind, func(t *testing.T) {
			n := Node{ID: "private-id", Source: "private-sub", Type: kind, Name: "测试 # 节点", Server: "2001:db8::1", Port: 443, Method: "aes-256-gcm", Password: "p:@ /?#&中文"}
			if kind == "vmess" || kind == "vless" || kind == "tuic" {
				n.SetOption("uuid", uuid)
			}
			if kind == "vmess" || kind == "vless" {
				n.Password = ""
				n.SetOption("network", "ws")
				n.SetOption("transport-path", "/a?x=1")
				n.SetOption("transport-host", "cover.example")
				n.SetOption("tls", true)
			}
			if kind == "ssr" {
				n.SetOption("cipher", "aes-256-cfb")
				n.SetOption("protocol", "origin")
				n.SetOption("obfs", "plain")
			}
			if kind == "hysteria" {
				n.SetOption("obfs", "mask")
				n.SetOption("sni", "cover.example")
			}
			if kind == "hysteria2" {
				n.SetOption("obfs-password", "mask")
				n.SetOption("skip-cert-verify", true)
			}
			if kind == "mieru" {
				n.SetOption("transport", "udp")
			}
			if kind == "socks5" || kind == "socks5s" || kind == "http" || kind == "https" || kind == "ssh" || kind == "mieru" {
				n.SetOption("username", "user:@ /中文")
			}
			uri := ShareURI(n)
			if strings.Contains(uri, "lanboat://") || strings.Contains(uri, n.ID) || strings.Contains(uri, n.Source) {
				t.Fatal("private format or identity leaked")
			}
			parsed, err := ParseURI(uri)
			if err != nil {
				t.Fatalf("import shared %s: %v", kind, err)
			}
			if parsed.Kind() != kind || parsed.Server != n.Server || parsed.Port != n.Port || parsed.Password != n.Password || parsed.Name != n.Name {
				t.Fatal("share lost required data", kind)
			}
			if kind == "hysteria2" && parsed.TextOption("obfs-password") != "mask" {
				t.Fatal("missing HY2 obfuscation")
			}
			if kind == "vmess" {
				raw, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "vmess://"))
				var v map[string]any
				if e != nil || json.Unmarshal(raw, &v) != nil || v["v"] != "2" || v["id"] != uuid {
					t.Fatal("VMess format")
				}
			}
		})
	}
}
func TestShareSIP002PluginAnd2022(t *testing.T) {
	n := Node{Type: "ss", Name: "plugin", Server: "proxy.example", Port: 8388, Method: "aes-256-gcm", Password: "pass", Plugin: "v2ray-plugin"}
	n.SetOption("plugin-opts.mode", "websocket")
	n.SetOption("plugin-opts.host", "cover.example")
	n.SetOption("plugin-opts.tls", true)
	uri := ShareURI(n)
	u, e := url.Parse(uri)
	if e != nil || !strings.Contains(u.Query().Get("plugin"), "v2ray-plugin;") || !strings.Contains(u.Query().Get("plugin"), "host=cover.example") || !strings.Contains(u.Query().Get("plugin"), "tls") {
		t.Fatal("SIP002 plugin argument")
	}
	n.Plugin = ""
	n.Options = nil
	n.Method = "2022-blake3-aes-128-gcm"
	n.Password = base64.StdEncoding.EncodeToString(make([]byte, 16))
	u, _ = url.Parse(ShareURI(n))
	password, _ := u.User.Password()
	if u.User.Username() != n.Method || password != n.Password {
		t.Fatal("SS2022 userinfo must not be base64")
	}
}
func TestMalformedSimpleShareInput(t *testing.T) {
	for _, s := range []string{"mierus://host?port=443&protocol=TCP", "brook://unknown?unknown=host:443"} {
		if _, err := ParseURI(s); err == nil {
			t.Fatal("malformed share link accepted")
		}
	}
}
