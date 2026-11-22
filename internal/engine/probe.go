package engine

import (
	"context"
	"github.com/goccy/go-json"
	"net"

	allinone "github.com/zatxm/lanboat-allinone"
	"github.com/zatxm/lanboat-app/internal/profile"
)

type Probe struct{ runtime *allinone.Runtime }

func (p *Probe) DialContext(ctx context.Context, address string) (net.Conn, error) {
	return p.runtime.Dial(ctx, "tcp", address)
}
func (p *Probe) Close() error { return p.runtime.Close() }
func (e *Engine) ProbeClient(ctx context.Context, n profile.Node) (*Probe, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := n.Validate(); err != nil {
		return nil, err
	}
	out := coreOutbound(n)
	outbounds := []any{out, map[string]any{"type": "direct", "tag": "direct"}}
	outbounds = appendUpstream(outbounds, n.DialProxy)
	raw, err := json.Marshal(map[string]any{"outbounds": outbounds, "route": map[string]any{"final": "proxy"}, "log": map[string]any{"disabled": true}})
	if err != nil {
		return nil, err
	}
	rt, err := allinone.New(ctx, raw)
	if err != nil {
		return nil, err
	}
	if err = rt.Start(); err != nil {
		rt.Close()
		return nil, err
	}
	return &Probe{rt}, nil
}
