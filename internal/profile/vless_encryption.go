package profile

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

type VLESSEncryption struct {
	Mode    string
	ZeroRTT bool
	Keys    [][]byte
	Padding [][3]int
}

func ParseVLESSEncryption(text string) (VLESSEncryption, error) {
	var result VLESSEncryption
	if text == "" || text == "none" {
		return result, nil
	}
	parts := strings.Split(text, ".")
	invalid := func() (VLESSEncryption, error) { return VLESSEncryption{}, fmt.Errorf("VLESS 加密格式无效") }
	if len(parts) < 4 || len(text) > 32768 || parts[0] != "mlkem768x25519plus" {
		return invalid()
	}
	result.Mode = parts[1]
	switch result.Mode {
	case "native", "xorpub", "random":
	default:
		return invalid()
	}
	switch parts[2] {
	case "0rtt":
		result.ZeroRTT = true
	case "1rtt":
	default:
		return invalid()
	}
	maxPadding := 0
	for _, part := range parts[3:] {
		if len(part) < 20 {
			if len(result.Keys) > 0 {
				return invalid()
			}
			numbers := strings.Split(part, "-")
			if len(numbers) != 3 {
				return invalid()
			}
			var values [3]int
			for i, text := range numbers {
				n, err := strconv.Atoi(text)
				if err != nil || n < 0 || n > 65535 {
					return invalid()
				}
				values[i] = n
			}
			if values[0] > 100 || values[1] > values[2] {
				return invalid()
			}
			if len(result.Padding) == 0 && (values[0] != 100 || values[1] < 35) {
				return invalid()
			}
			if len(result.Padding)%2 == 0 {
				maxPadding += values[2]
			}
			result.Padding = append(result.Padding, values)
		} else {
			key, err := base64.RawURLEncoding.DecodeString(part)
			if err != nil || (len(key) != 32 && len(key) != 1184) || len(result.Keys) >= 8 {
				return invalid()
			}
			result.Keys = append(result.Keys, key)
		}
	}
	if len(result.Keys) == 0 || maxPadding > 65553 || len(result.Padding) > 32 {
		return invalid()
	}
	return result, nil
}
