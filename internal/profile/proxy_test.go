package profile

import (
	"github.com/goccy/go-json"
	"reflect"
	"strings"
	"testing"
)

func TestProxySettingsValidationAndStorage(t *testing.T) {
	s := Default().Settings
	if s.Proxy.ListenHost() != "127.0.0.1" {
		t.Fatal("LAN enabled by default")
	}
	s.Proxy = ProxySettings{AllowLAN: true, Authentication: true, Username: "fixture", Password: "secret", Upstream: UpstreamProxy{Enabled: true, Type: "socks5", Server: "proxy.example", Port: 1081, Username: "upstream", Password: "key"}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(s)
	var restored Settings
	if err := json.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(restored, s) {
		t.Fatal("proxy settings lost", err)
	}
	if restored.Proxy.ListenHost() != "0.0.0.0" {
		t.Fatal("LAN not enabled")
	}
	for _, change := range []func(*Settings){func(s *Settings) { s.Proxy.Password = "" }, func(s *Settings) { s.Proxy.Username = "bad:name" }, func(s *Settings) { s.Proxy.Upstream.Server = "http://bad" }, func(s *Settings) { s.Proxy.Upstream.Port = 0 }, func(s *Settings) { s.Proxy.Upstream.Port = 65536 }, func(s *Settings) { s.Proxy.Upstream.Type = "unknown" }, func(s *Settings) { s.Proxy.Upstream.Username = strings.Repeat("x", 256) }, func(s *Settings) { s.Proxy.Upstream.Server = "::1"; s.Proxy.Upstream.Port = s.Port }} {
		bad := s
		change(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid proxy settings accepted")
		}
	}
	n := Node{DialProxy: &s.Proxy.Upstream}
	data, _ = json.Marshal(n)
	if strings.Contains(string(data), "proxy.example") || n.Masked().DialProxy != nil {
		t.Fatal("runtime dialer leaked into saved or masked node")
	}
}
