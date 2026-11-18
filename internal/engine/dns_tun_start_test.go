package engine

import (
	"context"
	"encoding/json"
	allinone "github.com/zatxm/lanboat-allinone"
	"github.com/zatxm/lanboat-app/internal/profile"
	"testing"
)

func TestTUNMainCoreDNSProbeInboundsStart(t *testing.T) {
	c := profile.Default()
	c.Selected = "ss"
	c.Nodes = []profile.Node{{ID: "ss", Type: "ss", Name: "fixture", Server: "127.0.0.1", Port: 9, Method: "aes-256-gcm", Password: "fixture"}}
	c.Settings.Mode = "tun"
	c.Settings.Port = freePort(t)
	d := profile.DefaultDNS()
	d.Bootstrap = "tcp://1.1.1.1"
	d.Direct = "tcp://1.1.1.1"
	d.Proxy = "tcp://1.1.1.1"
	d.Final = "proxy"
	d.FakeIP = true
	d.Persist = true
	c.Settings.DNS = &d
	config := BuildConfig(c, "127.0.0.1:0", "fixture-secret")
	_, release, err := configureDNSChecks(config)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	raw, _ := json.Marshal(config)
	rt, err := allinone.New(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	release()
	if err = rt.Start(); err != nil {
		t.Fatal(err)
	}
}
