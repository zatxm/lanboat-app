package profile

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecialFormFields(t *testing.T) {
	for _, kind := range []string{"sudoku", "naive", "tailscale"} {
		n := Node{Type: kind}
		seen := map[string]Field{}
		for _, field := range NodeFields(n) {
			if _, exists := seen[field.Key]; exists {
				t.Fatal("duplicate field", kind, field.Key)
			}
			seen[field.Key] = field
		}
		switch kind {
		case "sudoku":
			if !seen["key"].Required || seen["table-type"].Default != "prefer_entropy" || len(seen["table-type"].Choices) != 4 || seen["enable-pure-downlink"].Default != "true" || seen["http-mask"].Kind != "switch" || seen["http-mask"].Default != "false" || seen["multiplex"].Default != "off" || NodePasswordField(kind) {
				t.Fatal("Sudoku defaults or required fields")
			}
			if _, exists := seen["http-mask-mode"]; exists {
				t.Fatal("disabled HTTP mask has visible settings")
			}
		case "naive":
			if !seen["username"].Required || !NodePasswordField(kind) || seen["insecure-concurrency"].Default != "0" || seen["uot-version"].Default != "0" || !seen["extra-headers"].Secret {
				t.Fatal("Naive defaults or required fields")
			}
		case "tailscale":
			if len(seen) != 3 || !seen["auth-key"].Required || !seen["exit-node"].Required || NodePasswordField(kind) {
				t.Fatal("Tailscale fields")
			}
		}
	}
	for _, mode := range []string{"auto", "legacy", "stream", "poll", "ws"} {
		n := Node{Type: "sudoku"}
		n.SetOption("http-mask", true)
		n.SetOption("http-mask-mode", mode)
		seen := map[string]Field{}
		for _, field := range NodeFields(n) {
			seen[field.Key] = field
		}
		_, tls := seen["http-mask-tls"]
		_, host := seen["http-mask-host"]
		if tls != (mode != "legacy") || host != (mode != "legacy") || seen["path-root"].Group == "" {
			t.Fatal("HTTP mask field visibility", mode)
		}
	}
}

func TestNaiveCertificateChainInput(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	certificate := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	chain := certificate + certificate
	n := Node{Type: "naive"}
	n.SetOption("certificate", chain)
	if got, err := n.NaiveCertificatePEM(); err != nil || got != strings.TrimSpace(chain) {
		t.Fatal("PEM chain", err)
	}
	path := filepath.Join(t.TempDir(), "certificate chain.pem")
	if err := os.WriteFile(path, []byte(chain), 0600); err != nil {
		t.Fatal(err)
	}
	n.SetOption("certificate", path)
	if got, err := n.NaiveCertificatePEM(); err != nil || got != strings.TrimSpace(chain) {
		t.Fatal("certificate file", err)
	}
	for _, invalid := range []string{"", certificate + "trailing junk", "-----BEGIN PRIVATE KEY-----\ninvalid"} {
		os.WriteFile(path, []byte(invalid), 0600)
		if _, err := n.NaiveCertificatePEM(); err == nil {
			t.Fatal("invalid certificate file accepted")
		}
	}
}

func TestSpecialFormValidationAndMasking(t *testing.T) {
	n := Node{Type: "sudoku", Name: "Sudoku", Server: "example.com", Port: 443}
	n.SetOption("key", "test-key")
	for key, value := range map[string]string{"padding-min": "101", "padding-max": "4", "custom-table": "invalid", "custom-tables": "xpxvvpvv,invalid", "table-type": "invalid", "multiplex": "invalid"} {
		bad := n.Clone()
		bad.SetOption(key, value)
		if bad.Validate() == nil {
			t.Fatal("invalid Sudoku field accepted", key)
		}
	}
	n.SetOption("custom-tables", "xpxvvpvv,xxppvvvv")
	n.SetOption("http-mask", false)
	n.SetOption("http-mask-host", "hidden.example")
	if err := n.Validate(); err != nil {
		t.Fatal(err)
	}
	if masked := n.Masked(); masked.TextOption("key") != "" || masked.TextOption("http-mask-host") != "hidden.example" {
		t.Fatal("masking lost hidden fields or exposed key")
	}
	n = Node{Type: "naive", Name: "Naive", Server: "example.com", Port: 443, Password: "secret"}
	n.SetOption("username", "user")
	n.SetOption("extra-headers", "X-Example: value\nAuthorization: private-token")
	if err := n.Validate(); err != nil {
		t.Fatal(err)
	}
	if masked := n.Masked(); strings.Contains(masked.TextOption("extra-headers"), "private-token") {
		t.Fatal("headers leaked")
	}
	duplicate := n.Clone()
	duplicate.SetOption("extra-headers", "X-Example: first\nx-example: second")
	if duplicate.Validate() == nil {
		t.Fatal("duplicate headers accepted")
	}
	for key, value := range map[string]string{"extra-headers": "invalid", "certificate": "-----BEGIN CERTIFICATE-----\ninvalid", "sni": "invalid\nname", "insecure-concurrency": "-1", "uot-version": "3"} {
		bad := n.Clone()
		bad.SetOption(key, value)
		if bad.Validate() == nil {
			t.Fatal("invalid Naive field accepted", key)
		}
	}
}
