package ruledata

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Country rules in the Lanboat bundle contain one sorted SRS destination IP set.
// Keep its fixed-width address pairs packed; converting them to individual CIDRs
// would expand the index substantially.
type countryRanges struct {
	code   string
	v4, v6 []byte
}
type CountryIndex struct{ sets []countryRanges }

func LoadCountries(dir string) (CountryIndex, error) {
	var index CountryIndex
	files, err := os.ReadDir(filepath.Join(dir, "srs", "geoip"))
	if err != nil {
		return index, err
	}
	for _, f := range files {
		code, ok := strings.CutSuffix(f.Name(), ".srs")
		if !ok || len(code) != 2 || !ValidRegion(code) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "srs", "geoip", f.Name()))
		if err != nil {
			return CountryIndex{}, err
		}
		if set, ok := readCountry(code, data); ok {
			index.sets = append(index.sets, set)
		}
	}
	return index, nil
}
func readCountry(code string, data []byte) (countryRanges, bool) {
	set := countryRanges{code: code}
	if len(data) < 5 || string(data[:3]) != "SRS" || data[3] < 1 || data[3] > 5 {
		return set, false
	}
	z, err := zlib.NewReader(bytes.NewReader(data[4:]))
	if err != nil {
		return set, false
	}
	defer z.Close()
	b, err := io.ReadAll(io.LimitReader(z, 16<<20))
	if err != nil || len(b) < 14 || !bytes.Equal(b[:4], []byte{1, 0, 6, 1}) || !bytes.Equal(b[len(b)-2:], []byte{255, 0}) {
		return set, false
	}
	count := binary.BigEndian.Uint64(b[4:12])
	b = b[12 : len(b)-2]
	start := 0
	for start < len(b) && b[start] == 4 {
		if len(b)-start < 10 || b[start+5] != 4 {
			return set, false
		}
		start += 10
	}
	set.v4, set.v6 = b[:start], b[start:]
	if len(set.v6)%34 != 0 || uint64(start/10+len(set.v6)/34) != count {
		return set, false
	}
	for i := 0; i < len(set.v6); i += 34 {
		if set.v6[i] != 16 || set.v6[i+17] != 16 {
			return set, false
		}
	}
	return set, true
}
func (index CountryIndex) Country(ip netip.Addr) string {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return ""
	}
	key := ip.AsSlice()
	n := len(key)
	stride := 2 * (n + 1)
	for _, set := range index.sets {
		data := set.v4
		if ip.Is6() {
			data = set.v6
		}
		i := sort.Search(len(data)/stride, func(i int) bool { return bytes.Compare(data[i*stride+n+2:(i+1)*stride], key) >= 0 }) * stride
		if i < len(data) && bytes.Compare(data[i+1:i+n+1], key) <= 0 {
			return set.code
		}
	}
	return ""
}
