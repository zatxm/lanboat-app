package engine

import (
	"context"
	allinone "github.com/zatxm/lanboat-allinone/v26"
	"github.com/zatxm/lanboat-app/internal/profile"
	"net"
)

type Probe struct{ runtime *allinone.Runtime }

func (p *Probe) DialContext(ctx context.Context, address string) (net.Conn, error) {
	return p.runtime.Dial(ctx, "tcp", address)
}
func (p *Probe) Close() error { return p.runtime.Close() }
func (e *Engine) ProbeClient(ctx context.Context, n profile.Node) (*Probe, error) {
	raw, err := probeConfig(n)
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
