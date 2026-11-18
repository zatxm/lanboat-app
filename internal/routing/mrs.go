package routing

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strings"

	"github.com/klauspost/compress/zstd"
	"go4.org/netipx"
)

// parseMRS decodes the public MRS v1 wire format into native sing-box predicates.
func parseMRS(data []byte, ipSet bool) ([]map[string]any, error) {
	decoder, err := zstd.NewReader(bytes.NewReader(data), zstd.WithDecoderMaxMemory(64<<20))
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	decoded, err := io.ReadAll(io.LimitReader(decoder, 64<<20+1))
	if err != nil || len(decoded) > 64<<20 {
		return nil, fmt.Errorf("MRS 解压数据过大或无效")
	}
	r := bytes.NewReader(decoded)
	var header [5]byte
	if _, err = io.ReadFull(r, header[:]); err != nil || string(header[:4]) != "MRS\x01" || (header[4] == 1) != ipSet || header[4] > 1 {
		return nil, fmt.Errorf("MRS 标头或规则类型无效")
	}
	var count, extra int64
	if binary.Read(r, binary.BigEndian, &count) != nil || binary.Read(r, binary.BigEndian, &extra) != nil || count < 0 || count > 2000000 || extra < 0 || extra > int64(r.Len()) {
		return nil, fmt.Errorf("MRS 标头无效")
	}
	if _, err = r.Seek(extra, io.SeekCurrent); err != nil {
		return nil, err
	}
	version, err := r.ReadByte()
	if err != nil || version != 1 {
		return nil, fmt.Errorf("MRS 数据版本无效")
	}
	if ipSet {
		var n int64
		if binary.Read(r, binary.BigEndian, &n) != nil || n < 1 || n > int64(r.Len()/32) {
			return nil, fmt.Errorf("MRS IP 数据无效")
		}
		var prefixes []string
		for i := int64(0); i < n; i++ {
			var a, b [16]byte
			if _, err = io.ReadFull(r, a[:]); err != nil {
				return nil, err
			}
			if _, err = io.ReadFull(r, b[:]); err != nil {
				return nil, err
			}
			from, to := netip.AddrFrom16(a).Unmap(), netip.AddrFrom16(b).Unmap()
			if !from.IsValid() || from.BitLen() != to.BitLen() || from.Compare(to) > 0 {
				return nil, fmt.Errorf("MRS IP 范围无效")
			}
			for _, prefix := range netipx.IPRangeFrom(from, to).Prefixes() {
				prefixes = append(prefixes, prefix.String())
			}
		}
		return []map[string]any{{"ip_cidr": prefixes}}, nil
	}
	readBits := func() ([]uint64, error) {
		var n int64
		if binary.Read(r, binary.BigEndian, &n) != nil || n < 1 || n > int64(r.Len()/8) {
			return nil, fmt.Errorf("MRS 域名位图无效")
		}
		bits := make([]uint64, n)
		if err := binary.Read(r, binary.BigEndian, bits); err != nil {
			return nil, err
		}
		return bits, nil
	}
	leaves, err := readBits()
	if err != nil {
		return nil, err
	}
	bitmap, err := readBits()
	if err != nil {
		return nil, err
	}
	var n int64
	if binary.Read(r, binary.BigEndian, &n) != nil || n < 1 || n > int64(r.Len()) || n > 4000000 {
		return nil, fmt.Errorf("MRS 域名标签无效")
	}
	labels := make([]byte, n)
	if _, err = io.ReadFull(r, labels); err != nil {
		return nil, err
	}
	bit := func(bits []uint64, i int) bool {
		return i >= 0 && i/64 < len(bits) && bits[i/64]&(uint64(1)<<uint(i%64)) != 0
	}
	starts := make([]int, 1, len(labels)+1)
	for i := 0; i < len(bitmap)*64 && len(starts) <= len(labels); i++ {
		if bit(bitmap, i) {
			starts = append(starts, i+1)
		}
	}
	if len(starts) != len(labels)+1 || len(leaves)*64 < len(starts) {
		return nil, fmt.Errorf("MRS 域名树无效")
	}
	var exact, wildcard []string
	var visit func(int, []byte) error
	visit = func(node int, path []byte) error {
		if len(path) > 1024 {
			return fmt.Errorf("MRS 域名过长")
		}
		if bit(leaves, node) {
			key := strings.TrimSuffix(string(path), "+")
			runes := []rune(key)
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			domain := string(runes)
			if strings.Contains(domain, "*") || strings.HasPrefix(domain, ".") {
				parts := strings.Split(domain, ".")
				for i, part := range parts {
					if part == "*" {
						parts[i] = `[^.]+`
					} else {
						parts[i] = regexp.QuoteMeta(part)
					}
				}
				pattern := strings.Join(parts, `\.`)
				if strings.HasPrefix(domain, ".") {
					pattern = `.+` + pattern
				}
				wildcard = append(wildcard, "^"+pattern+"$")
			} else if domain != "" {
				exact = append(exact, domain)
			}
		}
		for pos := starts[node]; pos < len(bitmap)*64 && !bit(bitmap, pos); pos++ {
			index := pos - node
			if index < 0 || index >= len(labels) || index+1 <= node {
				return fmt.Errorf("MRS 域名树索引无效")
			}
			if err := visit(index+1, append(path, labels[index])); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(0, nil); err != nil {
		return nil, err
	}
	condition := map[string]any{}
	if len(exact) > 0 {
		condition["domain"] = exact
	}
	if len(wildcard) > 0 {
		condition["domain_regex"] = wildcard
	}
	if len(condition) == 0 {
		return nil, fmt.Errorf("MRS 域名规则为空")
	}
	return []map[string]any{condition}, nil
}
