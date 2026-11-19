package ruledata

import (
	"net/netip"
	"testing"
)

func TestBundledCountries(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	dir, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	index, err := LoadCountries(dir)
	if err != nil || len(index.sets) < 200 {
		t.Fatal(len(index.sets), err)
	}
	for ip, want := range map[string]string{"8.8.8.8": "us", "114.114.114.114": "cn", "2001:4860:4860::8888": "us", "192.168.1.1": ""} {
		if got := index.Country(netip.MustParseAddr(ip)); got != want {
			t.Errorf("%s: %s, want %s", ip, got, want)
		}
	}
}
