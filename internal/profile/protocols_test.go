package profile

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const testUUID = "12345678-1234-4234-8234-123456789012"

func TestMixedSubscriptions(t *testing.T) {
	vmess, _ := json.Marshal(map[string]any{"v": "2", "ps": "VMess 本地", "add": "127.0.0.1", "port": "443", "id": testUUID, "aid": "0", "net": "grpc", "path": "svc", "tls": "tls"})
	plain := "ss://YWVzLTI1Ni1nY206cGFzcw@localhost:8388#SS\nvmess://" + base64.StdEncoding.EncodeToString(vmess) + "\nvless://" + testUUID + "@example.com:443?security=reality&type=ws&path=%2Fproxy&sni=example.com&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=abcd#REALITY\nhy2://secret@example.com:443?sni=example.com&obfs=salamander&obfs-password=mask#HY2\nsocks5://user:pass@localhost:1080#SOCKS\nhttps://user:pass@localhost:8443#HTTPS"
	for _, encode := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		for depth := 0; depth <= 2; depth++ {
			content := plain
			for i := 0; i < depth; i++ {
				content = encode.EncodeToString([]byte(content))
			}
			nodes, err := ParseImport(content)
			if err != nil {
				t.Fatal(err)
			}
			if len(nodes) != 6 || nodes[1].TextOption("service-name") != "svc" || nodes[2].TextOption("reality-opts.public-key") != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" || nodes[3].Password != "secret" || nodes[5].Kind() != "https" {
				t.Fatalf("wrong decoded nodes: %d", len(nodes))
			}
			for _, n := range nodes {
				got, err := ParseURI(URI(n))
				if err != nil {
					t.Fatal(err)
				}
				got.ID = n.ID
				if !reflect.DeepEqual(n, got) {
					t.Fatalf("sharing lost fields for %s", n.Kind())
				}
			}
		}
	}
	if nodes, err := ParseImport(plain + "\nunknown://bad@host:123"); err == nil || len(nodes) != 0 {
		t.Fatal("partially accepted invalid subscription")
	}
	if _, err := ParseImport(strings.Repeat("a", MaxImport+1)); err == nil {
		t.Fatal("oversized subscription accepted")
	}
}
func TestStructuredAndSSR(t *testing.T) {
	yaml := `proxies:
 - name: VLESS
   type: vless
   server: example.com
   port: 443
   uuid: ` + testUUID + `
   tls: true
   network: ws
   ws-opts:
     path: /proxy
     headers:
       Authorization: Bearer secret
 - name: HTTP TLS
   type: http
   server: example.com
   port: 8443
   tls: true
   username: user
   password: secret
`
	nodes, err := ParseImport(yaml)
	if err != nil || len(nodes) != 2 {
		t.Fatal(err)
	}
	if nodes[1].Kind() != "https" || nodes[0].TextOption("ws-opts.path") != "/proxy" {
		t.Fatal("structured options lost")
	}
	masked := nodes[0].Masked()
	data, _ := json.Marshal(masked)
	if strings.Contains(string(data), "Bearer secret") || strings.Contains(string(data), testUUID) {
		t.Fatal("private values exposed")
	}
	original := nodes[0].Clone()
	masked.SetOption("reality-opts.public-key", "changed")
	if !reflect.DeepEqual(original, nodes[0]) {
		t.Fatal("masking mutated source")
	}
	ssrBody := "2001:db8::1:443:origin:aes-256-cfb:plain:" + base64.RawURLEncoding.EncodeToString([]byte("pass")) + "/?remarks=" + base64.RawURLEncoding.EncodeToString([]byte("SSR 节点"))
	n, err := ParseURI("ssr://" + base64.RawURLEncoding.EncodeToString([]byte(ssrBody)))
	if err != nil || n.Server != "2001:db8::1" || n.Password != "pass" || n.Name != "SSR 节点" {
		t.Fatal(n, err)
	}
	for _, input := range []string{`{"nodes":[]}`, `proxies: []`, `{"proxies":[{"name":"bad","type":"unknown","server":"host","port":443}]}`} {
		if _, err := ParseImport(input); err == nil {
			t.Fatal("invalid structured subscription accepted")
		}
	}
}

func TestDirectCloneAndMaskOwnTheirMutableValues(t *testing.T) {
	node := Node{Type: "vmess", Password: "private", Options: map[string]any{
		"uuid": testUUID, "alpn": []any{"h2", map[string]any{"nested": []string{"original"}}},
		"unused": map[string]any{"secret": "hidden"}, "alterId": 7,
	}}
	clone := node.Clone()
	clone.Option("alpn").([]any)[1].(map[string]any)["nested"].([]string)[0] = "changed"
	clone.Option("unused").(map[string]any)["secret"] = "changed"
	if node.Option("alpn").([]any)[1].(map[string]any)["nested"].([]string)[0] != "original" || node.Option("unused").(map[string]any)["secret"] != "hidden" {
		t.Fatal("clone shares mutable options")
	}
	masked := node.Masked()
	masked.Option("alpn").([]any)[0] = "changed"
	if masked.Password != "" || masked.TextOption("uuid") != "" || masked.Option("unused") != nil || node.Option("alpn").([]any)[0] != "h2" || node.TextOption("uuid") != testUUID {
		t.Fatal("mask leaks or modifies source")
	}
	if clone.Option("alterId") != 7 {
		t.Fatal("clone changed a scalar's type")
	}
}
