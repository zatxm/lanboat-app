package profile

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRandomAmneziaOptions(t *testing.T) {
	previous := RandomAmneziaOptions()
	for range 128 {
		n := Node{Type: "wireguard", Name: "AWG", Server: "example.com", Port: 51820}
		n.SetOption("private-key", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
		n.SetOption("public-key", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32))))
		n.SetOption("obfs", "amneziawg")
		n.SetOption("heartbeat", "20-30")
		options := RandomAmneziaOptions()
		if reflect.DeepEqual(previous, options) {
			t.Fatal("randomization repeated the same configuration")
		}
		for key, value := range options {
			n.SetOption("amnezia-wg-option."+key, value)
		}
		if err := n.Validate(); err != nil {
			t.Fatal(err)
		}
		masked := n.Masked()
		if masked.TextOption("amnezia-wg-option.header-protection-key") != "" || masked.TextOption("amnezia-wg-option.h1") == "" {
			t.Fatal("masking leaked the protection key or lost headers")
		}
		previous = options
	}
}

func TestAmnezia31Validation(t *testing.T) {
	n := Node{Type: "wireguard"}
	n.SetOption("obfs", "amneziawg")
	for key, value := range RandomAmneziaOptions() {
		n.SetOption("amnezia-wg-option."+key, value)
	}
	for key, value := range map[string]string{
		"header-protection-key": "invalid", "s1": "11", "h1": n.TextOption("amnezia-wg-option.h2"),
		"content-padding-addition": "0-4294967295", "rekey-timeout": "7-5", "random-trailers": "true\nlisten_port=1",
	} {
		bad := n.Clone()
		bad.SetOption("amnezia-wg-option."+key, value)
		if validateWireguardOptions(bad) == nil {
			t.Fatal("invalid AWG 3.1 option accepted", key)
		}
	}
}

func TestMergedProtocolTypes(t *testing.T) {
	for _, kind := range []string{"amneziawg"} {
		if _, ok := ProtocolByID(kind); ok {
			t.Fatal("duplicate protocol is still selectable", kind)
		}
	}
	for _, raw := range []string{
		`{"type":"amneziawg","id":"keep-id","options":{"amnezia-wg-option":{"jc":4,"s3":16}}}`,
		`{"type":"shadowtls","id":"keep-id","password":"outer","options":{"ss-method":"aes-128-gcm","ss-password":"inner","sni":"example.com","version":3,"skip-cert-verify":true}}`,
	} {
		var n Node
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			t.Fatal(err)
		}
		if n.ID != "keep-id" {
			t.Fatal("migration changed node identity")
		}
		if n.Kind() == "wireguard" {
			if n.TextOption("obfs") != "amneziawg" || n.TextOption("amnezia-wg-option.s3") != "16" {
				t.Fatal("migration lost WireGuard settings")
			}
		} else if n.Kind() != "ss" || n.Plugin != "shadow-tls" || n.Password != "inner" || n.Method != "aes-128-gcm" || n.TextOption("plugin-opts.password") != "outer" || n.TextOption("plugin-opts.version") != "3" || n.TextOption("plugin-opts.host") != "example.com" {
			t.Fatal("migration lost ShadowTLS settings")
		}
		before := n.Clone()
		n.normalizeProtocol()
		if !reflect.DeepEqual(before, n) {
			t.Fatal("migration is not idempotent")
		}
	}
}

func TestLegacyProtocolImportsAndLoad(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	for _, node := range []Node{
		{Type: "amneziawg", ID: "legacy", Name: "AWG", Server: "example.com", Port: 51820, Options: map[string]any{"private-key": key, "public-key": key, "amnezia-wg-option": map[string]any{"jc": 4}}},
		{Type: "shadowtls", ID: "legacy", Name: "ShadowTLS", Server: "example.com", Port: 443, Password: "outer", Options: map[string]any{"ss-method": "aes-128-gcm", "ss-password": "inner", "sni": "example.com", "version": 3}},
	} {
		t.Run(node.Type, func(t *testing.T) {
			converted, err := ParseURI(protocolURI(node))
			if err != nil {
				t.Fatal("share import", err)
			}
			data, _ := json.Marshal(map[string]any{"nodes": []Node{node}})
			nodes, err := ParseImport(string(data))
			if err != nil || len(nodes) != 1 || nodes[0].Kind() != converted.Kind() {
				t.Fatal("subscription import", err)
			}
			c := Default()
			c.Nodes, c.Selected = []Node{node}, node.ID
			path := filepath.Join(t.TempDir(), "config.json")
			if err := Save(path, c); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(path)
			if err != nil || loaded.Selected != node.ID || loaded.Nodes[0].Kind() != converted.Kind() {
				t.Fatal("saved config migration", err)
			}
		})
	}
}
