package profile

import (
	"fmt"
	"slices"
	"strings"
)

type TCPSettings struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	MaxRead     *int   `json:"maxRead,omitempty"`
}

var TCPFingerprints = []string{"Safari15_5", "Safari16_0", "Safari26_3", "Chrome100", "Chrome110", "Chrome120", "Chrome140", "Chrome150", "Firefox109", "Firefox117", "Firefox120", "Firefox148"}

func (s TCPSettings) ReadLimit() int {
	if s.MaxRead == nil {
		return 4096
	}
	return *s.MaxRead
}
func (s TCPSettings) DefaultFingerprint() string {
	if s.Fingerprint == "" {
		return "Chrome140"
	}
	return s.Fingerprint
}
func (s TCPSettings) Validate() error {
	if s.Fingerprint != "" && !slices.Contains(TCPFingerprints, s.Fingerprint) {
		return fmt.Errorf("TCP指纹无效")
	}
	if !slices.Contains([]int{0, 4096, 8192, 16384}, s.ReadLimit()) {
		return fmt.Errorf("最大读取长度无效")
	}
	return nil
}
func (n Node) TCPFingerprint() string {
	value := n.TextOption("client-fingerprint")
	if n.TCP != nil && (value == "" || strings.EqualFold(value, "default")) {
		return n.TCP.DefaultFingerprint()
	}
	return value
}
