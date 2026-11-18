package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"go.uber.org/zap"
)

func (a *App) ruleDataDir() string { return filepath.Join(filepath.Dir(a.path), "rules-data") }
func (a *App) RuleDataUpdatedAt() time.Time {
	b, err := os.ReadFile(filepath.Join(a.ruleDataDir(), "last-update"))
	if err != nil {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(b)))
	return t
}
func (a *App) UpdateRuleData(ctx context.Context) error {
	a.ruleUpdateMu.Lock()
	defer a.ruleUpdateMu.Unlock()
	root := a.ruleDataDir()
	if err := ruledata.Ensure(root); err != nil {
		return err
	}
	previous, err := ruledata.Current(root)
	if err != nil {
		return err
	}
	if err = ruledata.Fetch(ctx, root); err != nil {
		return fmt.Errorf("更新规则库失败：%w", err)
	}
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connectionCancel != nil && !a.switching && a.engine.Connected() && a.config.Settings.Mode == "rule" {
		restart := func() error {
			c := a.config
			route, e := profile.EffectiveRouting(c)
			if e != nil {
				return e
			}
			c.Routing = &route
			a.engine.Stop()
			return a.engine.StartContext(a.connectionCtx, c)
		}
		if err = restart(); err != nil {
			rollbackErr := ruledata.Activate(root, previous)
			if rollbackErr == nil {
				rollbackErr = restart()
			}
			if rollbackErr == nil {
				if compactErr := ruledata.CompactInactive(root); compactErr != nil {
					a.log.Warn("compress inactive rule data", zap.Error(compactErr))
				}
			}
			if rollbackErr != nil {
				a.connectionCancel()
				a.connectionCancel, a.connectionCtx = nil, nil
				a.activeNode = ""
				a.connectionError = "规则更新失败且原连接恢复失败，请重新连接"
			}
			return fmt.Errorf("应用规则更新失败：%v；恢复原连接：%v", err, rollbackErr)
		}
	}
	if err = os.WriteFile(filepath.Join(root, "last-update"), []byte(time.Now().Format(time.RFC3339Nano)), 0600); err != nil {
		return err
	}
	if err = ruledata.CompactInactive(root); err != nil {
		return fmt.Errorf("规则库已更新，但压缩旧数据失败：%w", err)
	}
	return nil
}

// DueUpdate schedules 10:10 in the system's local time, catching up after downtime.
func DueUpdate(now, last time.Time, days int) bool {
	local := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 10, 10, 0, 0, local)
	if last.IsZero() {
		return !now.Before(today)
	}
	last = last.In(local)
	due := time.Date(last.Year(), last.Month(), last.Day(), 10, 10, 0, 0, local).AddDate(0, 0, days)
	return !now.Before(due)
}
func (a *App) RunRuleUpdates(done <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var retry time.Time
	for {
		a.mu.Lock()
		settings := a.config.Settings
		a.mu.Unlock()
		if settings.RulesAutoUpdate() && time.Now().After(retry) && DueUpdate(time.Now(), a.RuleDataUpdatedAt(), settings.RulesUpdateDays()) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			err := a.UpdateRuleData(ctx)
			cancel()
			if err != nil {
				a.log.Warn("rule data update failed", zap.Error(err))
				retry = time.Now().Add(time.Hour)
			} else {
				retry = time.Time{}
			}
		}
		select {
		case <-done:
			return
		case <-ticker.C:
		}
	}
}
