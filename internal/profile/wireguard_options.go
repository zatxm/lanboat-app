package profile

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

func validateWireguardOptions(n Node) error {
	if value := n.TextOption("heartbeat"); value != "" {
		_, _, err := wireRange(value, 65535)
		if err != nil || strings.Contains(value, "-") && n.TextOption("obfs") != "amneziawg" {
			return fmt.Errorf("心跳间隔需为 0–65535")
		}
	}
	if n.TextOption("obfs") != "amneziawg" {
		return nil
	}
	values := map[string]int{}
	for _, key := range []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4"} {
		value := n.TextOption("amnezia-wg-option." + key)
		if value == "" {
			continue
		}
		number, err := strconv.Atoi(value)
		lo, hi := 0, 65507-148
		if key[0] == 'j' {
			hi = 65507
		}
		if key == "jc" {
			hi = 128
		}
		if err != nil || number < lo || number > hi {
			return fmt.Errorf("%s 需为 %d–%d", strings.ToUpper(key), lo, hi)
		}
		values[key] = number
	}
	if values["jmin"] > values["jmax"] {
		return fmt.Errorf("JMAX 不能小于 JMIN")
	}
	if values["s4"]+optionInteger(n, "mtu", 1420)+32 > 65507 {
		return fmt.Errorf("S4 与 MTU 超过 UDP 数据包长度")
	}
	headers := [4][2]uint64{{1, 1}, {2, 2}, {3, 3}, {4, 4}}
	for i, key := range []string{"h1", "h2", "h3", "h4"} {
		value := n.TextOption("amnezia-wg-option." + key)
		if value == "" {
			continue
		}
		a, b, err := wireRange(value, 1<<32-1)
		if err != nil {
			return fmt.Errorf("%s 需为有效的 32 位整数或范围", strings.ToUpper(key))
		}
		headers[i] = [2]uint64{a, b}
	}
	for i, left := range headers {
		for _, right := range headers[i+1:] {
			if left[0] <= right[1] && right[0] <= left[1] {
				return fmt.Errorf("H1–H4 范围不能重叠")
			}
		}
	}
	if value := n.TextOption("amnezia-wg-option.header-protection-key"); value != "" {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != 32 {
			return fmt.Errorf("头部保护密钥必须为 32 字节 Base64")
		}
		for _, name := range []string{"s1", "s2", "s3", "s4"} {
			if values[name] < 12 {
				return fmt.Errorf("启用头部保护时 S1–S4 必须至少为 12")
			}
		}
	}
	for _, key := range AmneziaRangeOptions {
		if value := n.TextOption("amnezia-wg-option." + key); value != "" {
			if _, _, err := wireRange(value, 65535); err != nil {
				return fmt.Errorf("%s 需为 0–65535 的整数或范围", key)
			}
		}
	}
	for _, key := range []string{"random-trailers", "disable-cookies"} {
		if value := n.TextOption("amnezia-wg-option." + key); value != "" && value != "true" && value != "false" {
			return fmt.Errorf("%s 需为 true 或 false", key)
		}
	}
	for _, key := range []string{"i1", "i2", "i3", "i4", "i5"} {
		if err := validateWireSignature(n.TextOption("amnezia-wg-option." + key)); err != nil {
			return fmt.Errorf("%s: %w", strings.ToUpper(key), err)
		}
	}
	return nil
}

func wireRange(value string, maximum uint64) (uint64, uint64, error) {
	left, right, hasRange := strings.Cut(value, "-")
	lo, err := strconv.ParseUint(left, 10, 32)
	if err != nil {
		return 0, 0, err
	}
	hi := lo
	if hasRange {
		hi, err = strconv.ParseUint(right, 10, 32)
	}
	if err != nil || hi < lo || hi > maximum || hi-lo == 1<<32-1 {
		return 0, 0, fmt.Errorf("无效范围")
	}
	return lo, hi, nil
}
func optionInteger(n Node, key string, fallback int) int {
	value := n.TextOption(key)
	if value == "" {
		return fallback
	}
	number, _ := strconv.Atoi(value)
	return number
}
func validateWireSignature(spec string) error {
	if spec == "" {
		return nil
	}
	if len(spec) > 131072 || strings.ContainsAny(spec, "\x00\r\n") {
		return fmt.Errorf("签名数据包格式无效")
	}
	total := 0
	for remaining := strings.TrimSpace(spec); remaining != ""; {
		if remaining[0] != '<' {
			return fmt.Errorf("签名需使用 <类型 参数> 格式")
		}
		tag, rest, ok := strings.Cut(remaining[1:], ">")
		if !ok {
			return fmt.Errorf("签名缺少 >")
		}
		fields := strings.Fields(tag)
		if len(fields) < 1 || len(fields) > 2 {
			return fmt.Errorf("签名参数无效")
		}
		value := ""
		if len(fields) == 2 {
			value = fields[1]
		}
		size := 0
		switch fields[0] {
		case "b":
			data, err := hex.DecodeString(strings.TrimPrefix(value, "0x"))
			if err != nil || len(data) == 0 {
				return fmt.Errorf("签名字节需为十六进制")
			}
			size = len(data)
		case "t":
			size = 4
		case "r", "rc", "rd", "dz":
			number, err := strconv.Atoi(value)
			if err != nil || number < 0 || number > 65507 {
				return fmt.Errorf("签名长度需为 0–65507")
			}
			size = number
		case "d", "ds": // No source payload is passed to I1–I5.
		default:
			return fmt.Errorf("未知签名类型 %s", fields[0])
		}
		total += size
		if total > 65507 {
			return fmt.Errorf("签名数据包不能超过 65507 字节")
		}
		remaining = strings.TrimSpace(rest)
	}
	return nil
}
