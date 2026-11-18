package profile

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	random "math/rand/v2"
	"strconv"
)

var AmneziaPacketOptions = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4", "i1", "i2", "i3", "i4", "i5"}
var AmneziaRangeOptions = []string{"content-padding-addition", "rekey-after-time", "rekey-timeout", "reject-after-time", "keepalive-timeout", "max-handshake-attempts"}

// Signatures describe real protocols, so randomization leaves I1–I5 untouched.
func RandomAmneziaOptions() map[string]string {
	key := make([]byte, 32)
	rand.Read(key)
	options := map[string]string{
		"header-protection-key": base64.StdEncoding.EncodeToString(key),
		"jc":                    strconv.Itoa(4 + random.IntN(9)),
		"jmin":                  strconv.Itoa(40 + random.IntN(61)), "jmax": strconv.Itoa(200 + random.IntN(601)),
		"content-padding-addition": fmt.Sprintf("0-%d", 16+random.IntN(49)),
		"rekey-after-time":         "100-120", "rekey-timeout": "5-7", "reject-after-time": "160-180",
		"keepalive-timeout": "10-15", "max-handshake-attempts": "15-20",
		"random-trailers": "true", "disable-cookies": "false",
	}
	// Equal prefixes avoid ambiguous packet detection with random trailers.
	padding := strconv.Itoa(12 + random.IntN(21))
	for i := 1; i <= 4; i++ {
		options[fmt.Sprintf("s%d", i)] = padding
		lo := uint64(i-1)*(1<<29) + 5 + random.Uint64N((1<<29)-2048)
		options[fmt.Sprintf("h%d", i)] = fmt.Sprintf("%d-%d", lo, lo+16+random.Uint64N(1008))
	}
	return options
}
