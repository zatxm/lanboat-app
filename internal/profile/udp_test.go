package profile

import "testing"

func TestUDPSettingsPortsAndDefaults(t *testing.T) {
	s := UDPSettings{}
	if !s.RelayEnabled() || s.Seconds() != 30 || s.BlockSTUN {
		t.Fatal("defaults")
	}
	for _, ports := range []string{"", "0", "53", "53,443,1000-2000", "53,1000:2000"} {
		s.Ports = ports
		if err := s.Validate(); err != nil {
			t.Fatal(ports, err)
		}
	}
	for _, ports := range []string{"0,53", "65536", "-1", "20-10", "1,,2", "x", "1-70000"} {
		s.Ports = ports
		if s.Validate() == nil {
			t.Fatal("invalid allowed", ports)
		}
	}
	s.Ports = ""
	if s.DirectOnly("hysteria2") || !s.DirectOnly("http") {
		t.Fatal("relay protocols")
	}
	off := false
	s.Relay = &off
	if !s.DirectOnly("ss") {
		t.Fatal("relay disabled")
	}
}
