package engine

import (
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func ShadowsocksIssue(n profile.Node) string {
	if n.TextOption("ota") == "true" && (strings.Contains(n.Method, "gcm") || strings.Contains(n.Method, "poly1305")) {
		return "一次性认证只适用于流加密算法，AEAD 算法已包含认证"
	}
	return ""
}
