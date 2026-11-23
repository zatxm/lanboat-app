package desktop

import (
	"errors"
	"reflect"

	"github.com/zatxm/lanboat-app/internal/profile"
)

// Callers hold connectionMu before mu, including connection startup and auto selection.
func (a *App) checkActiveNode(c profile.Config) error {
	if a.connectionCancel == nil || a.switching || a.activeNode == "" {
		return nil
	}
	if d := a.config.Settings.DNS; d != nil && d.ProxyRoute == "node" && c.Settings.DNS != nil && c.Settings.DNS.ProxyRoute == "node" && c.Settings.DNS.ProxyNode == d.ProxyNode {
		var old, next profile.Node
		for _, n := range a.config.Nodes {
			if n.ID == d.ProxyNode {
				old = n
				break
			}
		}
		for _, n := range c.Nodes {
			if n.ID == d.ProxyNode {
				next = n
				break
			}
		}
		if !reflect.DeepEqual(old, next) {
			return errors.New("正在使用的DNS出站节点不能修改或删除")
		}
	}
	var before, after profile.Node
	for _, n := range a.config.Nodes {
		if n.ID == a.activeNode {
			before = n
			break
		}
	}
	for _, n := range c.Nodes {
		if n.ID == a.activeNode {
			after = n
			break
		}
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("正在连接的节点不能修改或删除")
	}
	return nil
}

func (a *App) applyConnectedConfig(c profile.Config, force bool, restoreData func() error) (func() error, error) {
	noop := func() error {
		if restoreData != nil {
			return restoreData()
		}
		return nil
	}
	if a.connectionCancel == nil || a.switching || !a.engine.Connected() {
		return noop, nil
	}
	previous := a.config
	nodeTargetsChanged := !reflect.DeepEqual(c.Nodes, previous.Nodes) && (profile.HasPolicyTargets(c) || profile.HasPolicyTargets(previous))
	restart := force && c.Settings.Mode == "rule" || nodeTargetsChanged || !reflect.DeepEqual(c.Settings.UDP, previous.Settings.UDP) || !reflect.DeepEqual(c.Settings.TCP, previous.Settings.TCP) || !reflect.DeepEqual(c.Settings.DNS, previous.Settings.DNS) || c.Selected != previous.Selected || c.Settings.Port != previous.Settings.Port || c.Settings.Mode != previous.Settings.Mode || c.Settings.Proxy != previous.Settings.Proxy
	if !restart && (c.ActiveFile != previous.ActiveFile || !reflect.DeepEqual(c.Files, previous.Files) || !reflect.DeepEqual(c.Rules, previous.Rules) || !reflect.DeepEqual(c.RuleOverlays, previous.RuleOverlays) || !reflect.DeepEqual(c.PolicyGroups, previous.PolicyGroups) || c.Settings.Mode == "rule" && c.Settings.RuleRegion != previous.Settings.RuleRegion) {
		old, err := profile.EffectiveRouting(previous)
		if err != nil {
			return noop, err
		}
		next, err := profile.EffectiveRouting(c)
		if err != nil {
			return noop, err
		}
		restart = !reflect.DeepEqual(old, next)
	}
	proxyChanged := c.Settings.SystemProxy != previous.Settings.SystemProxy || c.Settings.Port != previous.Settings.Port || c.Settings.Mode != previous.Settings.Mode
	if !restart && !proxyChanged {
		return noop, nil
	}
	apply := func(config profile.Config) error {
		if restart {
			route, err := profile.EffectiveRouting(config)
			if err != nil {
				return err
			}
			config.Routing = &route
			a.engine.Stop()
			if err = a.engine.StartContext(a.connectionCtx, config); err != nil {
				return err
			}
		}
		if proxyChanged {
			if err := a.proxy.Restore(); err != nil {
				return err
			}
			if config.Settings.SystemProxy && config.Settings.Mode != "tun" {
				return a.proxy.Enable(config.Settings.Port)
			}
		}
		return nil
	}
	rollback := func() error {
		if restoreData != nil {
			if err := restoreData(); err != nil {
				a.engine.Stop()
				a.connectionCancel()
				a.connectionCancel, a.connectionCtx = nil, nil
				a.activeNode = ""
				a.connectionError = "设置应用失败，原连接恢复失败，请重新连接"
				return errors.Join(err, a.proxy.Restore())
			}
		}
		if err := apply(previous); err != nil {
			a.engine.Stop()
			a.connectionCancel()
			a.connectionCancel, a.connectionCtx = nil, nil
			a.activeNode = ""
			a.connectionError = "设置应用失败，原连接恢复失败，请重新连接"
			return errors.Join(err, a.proxy.Restore())
		}
		return nil
	}
	if err := apply(c); err != nil {
		return noop, errors.Join(err, rollback())
	}
	return rollback, nil
}
