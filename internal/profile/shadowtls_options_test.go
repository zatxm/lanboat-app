package profile

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestShadowTLSFieldsAndRoundTrip(t *testing.T) {
	standalone := Node{Type: "shadowtls", Name: "ShadowTLS", Server: "example.com", Port: 443}
	if err := standalone.Validate(); err != nil {
		t.Fatal("optional password", err)
	}
	if !NodeTLSEnabled(standalone) {
		t.Fatal("TLS must always be enabled")
	}
	fields := NodeFields(standalone)
	plugin := SSPluginFields("shadow-tls")
	if len(plugin) != len(fields)+3 || len(ShadowTLSFingerprints) != 12 {
		t.Fatal("incomplete shared fields")
	}
	for i, field := range fields {
		other := plugin[i+3]
		other.Key = field.Key
		if !reflect.DeepEqual(field, other) {
			t.Fatal("plugin and standalone differ", field.Key)
		}
	}
	if !plugin[0].Required || plugin[0].Placeholder != "域名或IP" || !plugin[1].Required || plugin[2].Required || fields[0].Default != "1" {
		t.Fatal("defaults or required fields")
	}
	standalone.Password = "private"
	standalone.SetOption("host", "cover.example")
	standalone.SetOption("client-fingerprint", "qq")
	standalone.SetOption("version", "3")
	raw, _ := json.Marshal(standalone)
	var decoded Node
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Kind() != "shadowtls" {
		t.Fatal("new protocol migrated", err)
	}
	imported, err := ParseURI(protocolURI(standalone))
	if err != nil || imported.Kind() != "shadowtls" || imported.Password != "private" || imported.TextOption("host") != "cover.example" {
		t.Fatal("share roundtrip", err)
	}
	if standalone.Masked().Password != "" || standalone.Masked().TextOption("client-fingerprint") != "qq" {
		t.Fatal("masking")
	}
	for _, fingerprint := range ShadowTLSFingerprints {
		n := standalone.Clone()
		n.SetOption("version", "1")
		n.SetOption("client-fingerprint", fingerprint)
		if err := n.Validate(); err != nil {
			t.Fatal(fingerprint, err)
		}
	}
	for _, hostKey := range []string{"host", "sni", "peer"} {
		node, err := ParseURI("shadowtls://secret@example.com:443?version=3&fp=qq&" + hostKey + "=cover.example")
		if err != nil || node.Kind() != "shadowtls" || node.TextOption("host") != "cover.example" {
			t.Fatal("standard URI", hostKey, err)
		}
	}
	for key, value := range map[string]string{"version": "4", "host": "bad/path", "alpn": "h2,", "certificate": "-----BEGIN CERTIFICATE-----\ninvalid", "skip-cert-verify": "invalid", "client-fingerprint": "unknown"} {
		n := standalone.Clone()
		n.SetOption(key, value)
		if n.Validate() == nil {
			t.Fatal("invalid option accepted", key)
		}
	}
}

func TestShadowTLSPluginEndpointsAndLegacyDefaults(t *testing.T) {
	n := Node{Type: "ss", Name: "SS", Server: "ss.example", Port: 8388, Method: DefaultSSMethod, Password: "inner", Plugin: "shadow-tls"}
	if err := n.Validate(); err != nil {
		t.Fatal("legacy implicit endpoints", err)
	}
	NormalizeSS(&n)
	if n.TextOption("plugin-opts.server") != n.Server || n.TextOption("plugin-opts.port") != "8388" || n.TextOption("plugin-opts.password") != "inner" {
		t.Fatal("legacy defaults lost")
	}
	n.SetOption("plugin-opts.password", "")
	NormalizeSS(&n)
	if n.TextOption("plugin-opts.password") != "" {
		t.Fatal("explicit empty password inherited SS password")
	}
	for key, value := range map[string]string{"server": "", "port": "0", "host": "bad host", "alpn": "h2\r\n", "client-fingerprint": "unknown"} {
		bad := n.Clone()
		bad.SetOption("plugin-opts."+key, value)
		if bad.Validate() == nil {
			t.Fatal("invalid plugin option accepted", key)
		}
	}
}
