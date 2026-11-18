package profile

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAdvancedProtocolFieldsAndPlaceholders(t *testing.T) {
	for _, kind := range []string{"anytls", "tuic", "juicity", "ssh", "wireguard", "openconnect", "trusttunnel", "masque"} {
		n := Node{Type: kind}
		fields := NodeFields(n)
		seen := map[string]Field{}
		for _, field := range fields {
			if _, ok := seen[field.Key]; ok {
				t.Fatal("duplicate field", kind, field.Key)
			}
			seen[field.Key] = field
		}
		if seen["udp"].Kind != "switch" {
			t.Fatal("missing UDP switch", kind)
		}
		if kind == "trusttunnel" {
			for _, key := range []string{"group", "authgroup"} {
				if _, ok := seen[key]; ok {
					t.Fatal("TrustTunnel has no group", key)
				}
			}
		}
		if kind == "openconnect" && seen["authgroup"].Placeholder != "all" {
			t.Fatal("OpenConnect authentication group was lost")
		}
		if kind == "ssh" {
			if _, ok := seen["private-key"]; ok {
				t.Fatal("disabled key is visible")
			}
			n.SetOption("private-key-enabled", true)
			for _, field := range NodeFields(n) {
				if field.Key == "private-key" && (!field.Secret || !field.Required || field.Group == "") {
					t.Fatal("private key boundary")
				}
			}
		}
		for _, key := range []string{"ip", "dns", "mtu", "heartbeat", "authgroup", "idle-timeout"} {
			if field, ok := seen[key]; ok && field.Placeholder != "" && field.Default != "" {
				t.Fatal("placeholder persisted as default", kind, key)
			}
		}
	}
	for _, obfs := range []string{"xt_wgobfs", "amneziawg"} {
		n := Node{Type: "wireguard"}
		n.SetOption("obfs", obfs)
		want := 1
		if obfs == "amneziawg" {
			want = 25
		}
		count := 0
		for _, field := range NodeFields(n) {
			if field.Group == "混淆设置" {
				count++
			}
		}
		if count != want {
			t.Fatalf("%s settings: %d want %d", obfs, count, want)
		}
	}
}

func TestAdvancedSecretsAndInactiveDrafts(t *testing.T) {
	n := Node{Type: "ssh", Password: "password"}
	n.SetOption("private-key-enabled", false)
	n.SetOption("private-key", "private-secret")
	n.SetOption("private-key-passphrase", "passphrase")
	n.SetOption("private-key-name", "key label")
	masked := n.Masked()
	if masked.Password != "" || masked.TextOption("private-key") != "" || masked.TextOption("private-key-passphrase") != "" || masked.TextOption("private-key-name") != "key label" {
		t.Fatal("SSH masked node leaks or loses drafts")
	}
	n = Node{Type: "wireguard"}
	n.SetOption("obfs", "none")
	n.SetOption("obfs-password", "obfs-secret")
	n.SetOption("amnezia-wg-option.s3", "38")
	masked = n.Masked()
	if masked.TextOption("obfs-password") != "" || masked.TextOption("amnezia-wg-option.s3") != "38" {
		t.Fatal("WireGuard hidden section masking")
	}
}

func TestWireguardPlaceholderValidationAndBounds(t *testing.T) {
	n := Node{Type: "wireguard", Name: "WG", Server: "example.com", Port: 51820}
	n.SetOption("private-key", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	n.SetOption("public-key", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32))))
	if err := n.Validate(); err != nil {
		t.Fatal("empty placeholders should be valid", err)
	}
	for key, value := range map[string]string{"mtu": "575", "dns": "not-an-ip", "ip": "invalid", "reserved": "0,0,256"} {
		invalid := n.Clone()
		invalid.SetOption(key, value)
		if invalid.Validate() == nil {
			t.Fatal("invalid option accepted", key)
		}
	}
}

func TestWireguardObfuscationRejectsInjectionAndOversizedPackets(t *testing.T) {
	n := Node{Type: "wireguard"}
	n.SetOption("obfs", "amneziawg")
	for key, value := range map[string]string{
		"amnezia-wg-option.jc": "1000000", "amnezia-wg-option.s4": "65500", "amnezia-wg-option.i1": "<r 999999999999>", "amnezia-wg-option.i2": "<r -1>", "amnezia-wg-option.i3": "<r 40000><r 40000>", "amnezia-wg-option.i4": "<r 10>\nlisten_port=4444", "amnezia-wg-option.h1": "0-4294967295", "amnezia-wg-option.h2": "10-1", "heartbeat": "65536",
	} {
		bad := n.Clone()
		bad.SetOption(key, value)
		if validateWireguardOptions(bad) == nil {
			t.Fatal("invalid WireGuard option", key)
		}
	}
	n.SetOption("amnezia-wg-option.i1", "<b 0xcafebabe><t><r 16><rc 4><rd 4><dz 2><d><ds>")
	n.SetOption("amnezia-wg-option.h1", "1001-1010")
	if err := validateWireguardOptions(n); err != nil {
		t.Fatal(err)
	}
}

func TestQUICOptionsCannotBeSilentlyIgnored(t *testing.T) {
	for _, kind := range []string{"http3", "tuic", "juicity", "hysteria", "hysteria2", "masque", "vmess", "vless", "trusttunnel"} {
		n := Node{Type: kind}
		n.SetOption("network", "quic")
		n.SetOption("quic", true)
		if err := ValidateQUICTLSOptions(n); err != nil {
			t.Fatal(kind, err)
		}
		for key, value := range map[string]string{"fragment": "true", "reality-opts.public-key": "invalid"} {
			invalid := n.Clone()
			invalid.SetOption(key, value)
			if ValidateQUICTLSOptions(invalid) == nil {
				t.Fatal("QUIC ignored option", kind, key)
			}
		}
	}
}
