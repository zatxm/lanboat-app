package engine

import (
	"context"
	"github.com/goccy/go-json"
	"strings"

	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/profile"
	"time"
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
	program, err := coreipc.Executable(FindCore())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err = coreipc.RunCommand(ctx, program, coreipc.Request{Op: "check", Config: raw})
	return err
}
func NodeOptionsIssue(n profile.Node) string {
	if err := nodeConnectionIssue(n); err != nil {
		return err.Error()
	}
	return ""
}
