package desktop

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"testing"
)

func TestUDPSettingsDoNotAliasCaller(t *testing.T) {
	a := testApp(t)
	s := a.GetState().Config.Settings
	relay := false
	s.UDP = profile.UDPSettings{Relay: &relay, Timeout: 42}
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	relay = true
	if a.GetState().Config.Settings.UDP.RelayEnabled() {
		t.Fatal("saved relay aliases caller")
	}
	snapshot := a.GetState()
	*snapshot.Config.Settings.UDP.Relay = true
	if a.GetState().Config.Settings.UDP.RelayEnabled() {
		t.Fatal("snapshot aliases live relay")
	}
	c, err := profile.Load(a.path)
	if err != nil || c.Settings.UDP.RelayEnabled() || c.Settings.UDP.Seconds() != 42 {
		t.Fatal("saved UDP settings", err)
	}
}
