package profile

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestQuantumultXNodeTextAndRules(t *testing.T) {
	text := `[general]
server_check_url = https://probe.example
[server_local]
shadowsocks=[2001:db8::1]:8388, method=aes-128-gcm, password="secret,example", tag=West
vless=edge.example:443, password=4f817a10-ff8f-4ca2-912a-f4885d721642, method=none, obfs=wss, obfs-uri=/test, obfs-host=tls.example, tls-verification=true, tag=TLS
[filter_local]
host-suffix,example.net,West
final,proxy`
	for _, content := range []string{text, base64.StdEncoding.EncodeToString([]byte(text))} {
		nodes, rules, err := ParseImportWithRules(content, "[Rule]\n")
		if err != nil || len(nodes) != 2 {
			t.Fatalf("Quantumult X import: %d nodes, %v", len(nodes), err)
		}
		if nodes[0].Server != "2001:db8::1" || nodes[0].Password != "secret,example" ||
			nodes[1].TextOption("network") != "ws" || !NodeTLSEnabled(nodes[1]) || nodes[1].TextOption("sni") != "tls.example" {
			t.Fatal("Quantumult X node options were lost")
		}
		if !strings.Contains(rules, "DOMAIN-SUFFIX,example.net,West") || !strings.Contains(rules, "FINAL,PROXY") {
			t.Fatal("Quantumult X routing rules were lost")
		}
	}
}

func TestLoonNodeTextAndRules(t *testing.T) {
	text := `[Proxy]
https://label.example/ = Trojan,server.example,443,"password,one",transport=ws,host=edge.example,path=/one,skip-cert-verify=false
Hop = Hysteria2,hop.example,443,"secret",server-ports="40000:40002,443",salamander-password=mask,download-bandwidth=0,hop-interval=15
[Rule]
DOMAIN,match.example,Hop
FINAL,DIRECT`
	nodes, rules, err := ParseImportWithRules(text, "[Rule]\n")
	if err != nil || len(nodes) != 2 {
		t.Fatalf("Loon import: %d nodes, %v", len(nodes), err)
	}
	if nodes[0].Password != "password,one" || nodes[0].TextOption("network") != "ws" ||
		nodes[1].TextOption("ports") != "40000:40002,443" || nodes[1].TextOption("obfs") != "salamander" ||
		nodes[1].TextOption("down") != "auto" || nodes[1].TextOption("hop-interval") != "15s" {
		t.Fatal("Loon transport, obfuscation or port hopping options were lost")
	}
	if !strings.Contains(rules, "DOMAIN,match.example,Hop") {
		t.Fatal("Loon routing rule was lost")
	}
}

func TestSubscriptionLinkCompatibilityMetadata(t *testing.T) {
	text := `STATUS=Subscription status text
trojan://fixture@trojan.example:443?pcs=#Trojan
vless://4f817a10-ff8f-4ca2-912a-f4885d721642@vless.example:443?type=tcp&headerType=none&quicSecurity=none&pcs=&security=tls#Vless
hysteria2://fixture@hysteria.example:443?mport=443,34000-34010,#Hop`
	nodes, err := ParseImport(base64.StdEncoding.EncodeToString([]byte(text)))
	if err != nil || len(nodes) != 3 {
		t.Fatalf("subscription compatibility: %d nodes, %v", len(nodes), err)
	}
	if nodes[2].TextOption("ports") != "443,34000-34010" {
		t.Fatal("Hysteria2 port range was lost")
	}
}
