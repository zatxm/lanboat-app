package desktop

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zatxm/lanboat-app/internal/engine"
	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
)

// connectionMu keeps node tests and engine changes separate from disconnect/reconnect.
func (a *App) SetAutoSelect(enabled bool) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	previous, ctx := a.config, a.connectionCtx
	c := previous
	c.Settings.AutoSelect = enabled
	if !enabled || ctx == nil || c.Settings.Mode == "direct" || !a.engine.Connected() {
		err := a.commit(c)
		a.mu.Unlock()
		return err
	}
	a.mu.Unlock()
	id, err := a.fastestNode(ctx, c)
	if err != nil {
		return err
	}
	c.Selected = id
	if id == previous.Selected {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.commit(c)
	}
	route, err := profile.EffectiveRouting(c)
	if err != nil {
		return err
	}
	c.Routing = &route
	a.mu.Lock()
	a.switching = true
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.switching = false; a.mu.Unlock() }()
	a.engine.Stop()
	err = a.engine.StartContext(ctx, c)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		a.mu.Lock()
		err = a.commit(c)
		if err == nil {
			a.activeNode = id
		}
		a.mu.Unlock()
	}
	if err == nil {
		a.log.Info("lowest latency node selected", zap.String("nodeID", id))
		return nil
	}
	a.engine.Stop()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// A failed candidate or save must not discard the connection that was working.
	previous.Routing = &route
	if restoreErr := a.engine.StartContext(ctx, previous); restoreErr != nil {
		a.engine.Stop()
		a.mu.Lock()
		a.connectionCancel()
		a.connectionCancel, a.connectionCtx = nil, nil
		a.activeNode = ""
		a.connectionError = "自动选择失败，原节点恢复失败，请重新连接"
		proxyErr := a.proxy.Restore()
		a.mu.Unlock()
		return errors.Join(err, restoreErr, proxyErr)
	}
	return fmt.Errorf("自动选择失败，已恢复原节点：%w", err)
}

func (a *App) fastestNode(parent context.Context, c profile.Config) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	results := make(map[string]int64, len(c.Nodes))
	jobs := make(chan profile.Node, len(c.Nodes))
	for _, n := range c.Nodes {
		jobs <- n
	}
	close(jobs)
	workers := c.Settings.TestWorkers()
	var wg sync.WaitGroup
	var mu sync.Mutex
	for range workers {
		wg.Go(func() {
			for n := range jobs {
				if ctx.Err() != nil {
					return
				}
				ms := int64(-1)
				protocol, ok := profile.ProtocolByID(n.Kind())
				if ok && protocol.Core != "external" && (n.Kind() != "ss" || engine.ShadowsocksIssue(n) == "") {
					if value, err := a.testNodeContext(ctx, n, c.Settings); err == nil {
						ms = value
					}

				}
				mu.Lock()
				results[n.ID] = ms
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	a.mu.Lock()
	a.nodeDelays = results
	a.mu.Unlock()
	if err := parent.Err(); err != nil {
		return "", err
	}
	best, lowest := "", int64(1<<63-1)
	if ms, ok := results[c.Selected]; ok && ms >= 0 {
		best, lowest = c.Selected, ms
	}
	for _, n := range c.Nodes {
		if ms, ok := results[n.ID]; ok && ms >= 0 && ms < lowest {
			best, lowest = n.ID, ms
		}
	}
	if best == "" {
		return "", errors.New("没有检测成功的可用节点，请检查节点或延迟测试设置")
	}
	return best, nil
}
