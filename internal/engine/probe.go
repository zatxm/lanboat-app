package engine

import (
	"context"
	"github.com/goccy/go-json"
	"github.com/zatxm/lanboat-app/internal/coreipc"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func probeConfig(n profile.Node) ([]byte, error) {
	if err := n.Validate(); err != nil {
		return nil, err
	}
	outbounds := appendUpstream([]any{coreOutbound(n), map[string]any{"type": "direct", "tag": "direct"}}, n.DialProxy)
	return json.Marshal(map[string]any{"outbounds": outbounds, "route": map[string]any{"final": "proxy"}, "log": map[string]any{"disabled": true}})
}

func (e *Engine) ProbeHTTP(ctx context.Context, node profile.Node, target string) (int64, error) {
	raw, err := probeConfig(node)
	if err != nil {
		return 0, err
	}
	request := coreipc.Request{Op: "probe", Target: target, Config: raw}
	e.mu.Lock()
	current := e.runtime
	e.mu.Unlock()
	if current != nil {
		response, err := current.Call(ctx, request)
		return response.Latency, err
	}
	program, err := coreipc.Executable(e.path)
	if err != nil {
		return 0, err
	}
	response, err := coreipc.RunCommand(ctx, program, request)
	return response.Latency, err
}
