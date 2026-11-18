package engine

import (
	"context"
	"fmt"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
	"net"
	"strings"
)

func (e *Engine) closePolicies() {
	for _, l := range e.fallbackReservations {
		l.Close()
	}
	e.fallbackReservations = nil
	e.fallbackGroups = nil
}
func (e *Engine) preparePolicies(ctx context.Context, c profile.Config, config map[string]any) (err error) {
	if c.Settings.Mode != "rule" || c.Routing == nil {
		return nil
	}
	defer func() {
		if err != nil {
			e.closePolicies()
		}
	}()
	groups := map[string]routing.Group{}
	for _, g := range c.Routing.Groups {
		groups[g.Name] = g
	}
	resolved := map[string]string{"direct": "direct", "proxy": "proxy"}
	visiting := map[string]bool{}
	outbounds := config["outbounds"].([]any)
	var resolve func(string) (string, error)
	resolve = func(policy string) (string, error) {
		if tag, ok := resolved[policy]; ok {
			return tag, nil
		}
		if visiting[policy] {
			return "", fmt.Errorf("代理分组循环引用：%s", policy)
		}
		visiting[policy] = true
		defer delete(visiting, policy)
		if g, ok := groups[policy]; ok {
			members := make([]string, 0, len(g.Members))
			for _, member := range g.Members {
				tag, err := resolve(member)
				if err != nil {
					return "", err
				}
				members = append(members, tag)
			}
			if len(members) == 0 {
				return "", fmt.Errorf("代理分组不能为空：%s", g.Name)
			}
			tag := "policy-group-" + g.Name
			out := map[string]any{"type": "selector", "tag": tag, "outbounds": members, "default": members[0]}
			if g.Selected != "" {
				selected, err := resolve(g.Selected)
				if err != nil {
					return "", err
				}
				out["default"] = selected
			}
			if g.Type == "url-test" {
				out = map[string]any{"type": "urltest", "tag": tag, "outbounds": members, "url": g.URL, "interval": fmt.Sprintf("%ds", g.Interval), "tolerance": g.Tolerance}
			}
			if g.Type == "fallback" {
				group := fallbackGroup{tag: tag, url: g.URL, interval: g.Interval}
				for _, member := range members {
					group.members = append(group.members, fallbackMember{tag: member})
				}
				e.fallbackGroups = append(e.fallbackGroups, group)
			}
			outbounds = append(outbounds, out)
			resolved[policy] = tag
			return tag, nil
		}
		var target profile.Node
		for _, n := range c.Nodes {
			if policy == "node:"+n.ID || policy == n.Name {
				if target.ID != "" && target.ID != n.ID {
					return "", fmt.Errorf("节点名称重复，请使用node:ID：%s", policy)
				}
				target = n
			}
		}
		if target.ID == "" {
			return "", fmt.Errorf("规则策略对应的节点或分组不存在：%s", policy)
		}
		key := "node:" + target.ID
		if tag, ok := resolved[key]; ok {
			resolved[policy] = tag
			return tag, nil
		}
		if err := target.Validate(); err != nil {
			return "", fmt.Errorf("分流节点%s：%w", target.Name, err)
		}
		target = target.Clone()
		target.DialProxy = &c.Settings.Proxy.Upstream
		target.TCP = &c.Settings.TCP
		tag := "policy-node-" + target.ID
		out := coreOutbound(target)
		if c.Settings.Proxy.Upstream.Enabled {
			out["detour"] = "upstream"
		}
		out["tag"] = tag
		outbounds = append(outbounds, out)
		resolved[key] = tag
		resolved[policy] = tag
		return tag, nil
	}
	// Resolve only targets used by effective rules; unused groups consume no protocol clients.
	var rewrite func(any, []string) error
	rewrite = func(raw any, keys []string) error {
		switch v := raw.(type) {
		case map[string]any:
			for _, key := range keys {
				if action, ok := v[key].(string); ok && action != "" && action != "direct" && action != "proxy" {
					target := action
					if key == "detour" {
						if !strings.HasPrefix(target, "policy-target:") {
							continue
						}
						target = strings.TrimPrefix(target, "policy-target:")
					}
					tag, err := resolve(target)
					if err != nil {
						return err
					}
					v[key] = tag
				}
			}
			for _, child := range v {
				if err := rewrite(child, keys); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := rewrite(child, keys); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err = rewrite(config["route"], []string{"outbound", "final"}); err != nil {
		return err
	}
	if err = rewrite(config["dns"], []string{"detour"}); err != nil {
		return err
	}
	ports := map[string]int{}
	for i := range e.fallbackGroups {
		for j := range e.fallbackGroups[i].members {
			member := &e.fallbackGroups[i].members[j]
			port := ports[member.tag]
			if port == 0 {
				var listener net.Listener
				listener, err = net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					return err
				}
				e.fallbackReservations = append(e.fallbackReservations, listener)
				port = listener.Addr().(*net.TCPAddr).Port
				ports[member.tag] = port
				tag := fmt.Sprintf("fallback-probe-%d", port)
				config["inbounds"] = append(config["inbounds"].([]any), map[string]any{"type": "mixed", "tag": tag, "listen": "127.0.0.1", "listen_port": port})
				route := config["route"].(map[string]any)
				route["rules"] = append([]any{map[string]any{"inbound": []string{tag}, "action": "route", "outbound": member.tag}}, route["rules"].([]any)...)
			}
			member.port = port
		}
	}
	config["outbounds"] = outbounds
	return ctx.Err()
}
