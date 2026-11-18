package profile

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestURIRoundTrip(t *testing.T) {
	for _, method := range []string{"aes-256-gcm", "2022-blake3-aes-128-gcm"} {
		n := Node{Name: "香港 / 测试 + 01", Server: "2001:db8::1", Port: 8388, Method: method, Password: "p@ss:%+/?word", Plugin: "obfs-local", PluginOpts: "obfs=http;obfs-host=example.com"}
		if strings.HasPrefix(method, "2022-") {
			n.Password = base64.StdEncoding.EncodeToString(make([]byte, 16))
		}
		parsed, err := ParseURI(URI(n))
		if err != nil {
			t.Fatal(err)
		}
		parsed.ID = ""
		if !reflect.DeepEqual(n, parsed) {
			t.Fatalf("round trip mismatch: %#v != %#v", n, parsed)
		}
	}
}

func TestImport(t *testing.T) {
	uri := "ss://" + base64.RawStdEncoding.EncodeToString([]byte("aes-256-gcm:secret@host.example:8388")) + "#legacy"
	for _, content := range []string{uri, base64.StdEncoding.EncodeToString([]byte(uri + "\n" + uri))} {
		nodes, err := ParseImport(content)
		if err != nil || len(nodes) == 0 || nodes[0].Password != "secret" {
			t.Fatalf("import: %v %v", nodes, err)
		}
	}
	for _, content := range []string{"", "vmess://whatever", uri + "\nss://invalid", "ss://YWVzLTI1Ni1nY206cA@host:0", strings.Repeat("x", MaxImport+1), "ss://dW5rbm93bjpw@host:80"} {
		if nodes, err := ParseImport(content); err == nil || len(nodes) != 0 {
			t.Fatalf("accepted invalid content of length %d", len(content))
		}
	}
}

func TestSavePermissionsAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles", "config.json")
	c := Default()
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted corrupt config")
	}
	content, _ := os.ReadFile(path)
	if string(content) != "broken" {
		t.Fatal("corrupt config was overwritten")
	}
}

func TestValidation(t *testing.T) {
	for _, rule := range []Rule{{Type: "ip_cidr", Value: "127.0.0.0/8", Action: "direct"}, {Type: "domain_suffix", Value: "example.com", Action: "reject"}} {
		if err := rule.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := (Rule{Type: "domain", Value: "https://example.com", Action: "proxy"}).Validate(); err == nil {
		t.Fatal("accepted URL as domain")
	}
	if err := (Settings{Port: 80, Mode: "global"}).Validate(); err == nil {
		t.Fatal("accepted privileged port")
	}
}
