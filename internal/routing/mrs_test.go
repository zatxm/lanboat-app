package routing

import (
	"net/netip"
	"os"
	"testing"
)

func TestMRSOfficialRuleSets(t *testing.T) {
	for _, tc := range []struct {
		file      string
		ip        bool
		hit, miss Request
	}{
		{"testdata/github.mrs", false, Request{Host: "github.com"}, Request{Host: "unrelated.invalid"}},
		{"testdata/cloudflare.mrs", true, Request{IP: netip.MustParseAddr("104.16.0.1")}, Request{IP: netip.MustParseAddr("127.0.0.1")}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			conditions, err := parseMRS(data, tc.ip)
			if err != nil {
				t.Fatal(err)
			}
			r := Rule{Type: "RULE-SET", SetRules: conditions}
			if !MatchRule(r, tc.hit) || MatchRule(r, tc.miss) {
				t.Fatal("MRS match differs from source")
			}
			if !tc.ip && (!MatchRule(r, Request{Host: "raw.githubusercontent.com"}) || !MatchRule(r, Request{Host: "gist.github.com"})) {
				t.Fatal("MRS domain suffix matching failed")
			}
			if _, err := parseMRS(data, !tc.ip); err == nil {
				t.Fatal("accepted incorrect MRS behavior")
			}
			if _, err := parseMRS(data[:len(data)/2], tc.ip); err == nil {
				t.Fatal("accepted truncated MRS")
			}
		})
	}
}
