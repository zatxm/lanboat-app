package profile

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestConnectivityDefaultsAndValidation(t *testing.T) {
	s := Default().Settings
	if s.TestSeconds() != 15 || s.TestWorkers() != 20 || s.SpeedTarget() != SpeedURLs[0] || s.TestTarget() != TestURLs[0] {
		t.Fatal("defaults")
	}
	s.TestConcurrency = 20
	s.TestTimeout = 15
	s.SpeedURL = "https://example.com/speed"
	s.SpeedURLs = []string{s.SpeedURL}
	s.TestURLs = []string{"https://example.com/test"}
	s.TCP.Fingerprint = "Chrome150"
	data, _ := json.Marshal(s)
	var restored Settings
	if err := json.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(s, restored) {
		t.Fatal("roundtrip", err)
	}
	for _, method := range []string{"tcp", "icmp", "connect"} {
		s.TestMethod = method
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, change := range []func(*Settings){func(s *Settings) { s.TestConcurrency = 21 }, func(s *Settings) { s.TestConcurrency = -1 }, func(s *Settings) { s.TestTimeout = 301 }, func(s *Settings) { s.SpeedURL = "file:///tmp/test" }, func(s *Settings) { s.TestURLs = []string{"https://u:p@example.com"} }, func(s *Settings) { v := 123; s.TCP.MaxRead = &v }} {
		bad := s
		change(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid value accepted")
		}
	}
}

func TestTCPDefaultsDoNotLeakIntoNodeShares(t *testing.T) {
	n := Node{Type: "trojan", Server: "example.com", Port: 443, Password: "secret", TCP: &TCPSettings{Fingerprint: "Chrome150"}}
	if n.TCPFingerprint() != "Chrome150" {
		t.Fatal("global fingerprint ignored")
	}
	n.SetOption("client-fingerprint", "Chrome120")
	if n.TCPFingerprint() != "Chrome120" {
		t.Fatal("node fingerprint must take precedence")
	}
	if n.Masked().TCP != nil {
		t.Fatal("runtime settings leaked into masked node")
	}
	data, _ := json.Marshal(n)
	var restored Node
	if err := json.Unmarshal(data, &restored); err != nil || restored.TCP != nil {
		t.Fatal("runtime TCP settings persisted", err)
	}
}
