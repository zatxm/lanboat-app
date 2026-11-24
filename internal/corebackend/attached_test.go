package corebackend

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/zatxm/lanboat-app/internal/coreipc"
)

const attachedConfig = `{
	"inbounds":[{"type":"tun","tag":"tun-in","address":["172.29.45.1/30"],"auto_route":false,"dns_mode":"disabled"}],
	"outbounds":[{"type":"direct","tag":"direct"}],
	"route":{"final":"direct"}
}`

func TestAttachedStartupCancellation(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		select {
		case <-entered:
			cancel()
		case <-ctx.Done():
		}
	}()
	_, err := coreipc.LaunchEmbedded(ctx, coreipc.Request{Op: "start", Config: []byte(attachedConfig), TUN: true}, nil,
		func(ctx context.Context, input io.Reader, output io.Writer) error {
			defer close(stopped)
			return RunAttached(ctx, input, output, func(ctx context.Context) (Tunnel, error) {
				close(entered)
				<-ctx.Done()
				return Tunnel{}, ctx.Err()
			})
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled VPN authorization: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("network backend didn't release pending authorization")
	}
}
