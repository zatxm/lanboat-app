package engine

import (
	"context"
	"github.com/goccy/go-json"
	"strings"

	allinone "github.com/zatxm/lanboat-allinone"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func tlsFingerprint(value string) (string, error) {
	if value == "" || strings.EqualFold(value, "default") {
		return "", nil
	}
	if strings.EqualFold(value, "safari") {
		return "safari26_3", nil
	}
	return strings.ToLower(value), nil
}
func nodeConnectionIssue(n profile.Node) error {
	raw, err := json.Marshal(map[string]any{"outbounds": appendUpstream([]any{coreOutbound(n)}, n.DialProxy), "route": map[string]any{"final": "proxy"}})
	if err != nil {
		return err
	}
	return allinone.Check(context.Background(), raw)
}
func NodeOptionsIssue(n profile.Node) string {
	if err := nodeConnectionIssue(n); err != nil {
		return err.Error()
	}
	return ""
}
