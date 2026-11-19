package desktop

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/zatxm/lanboat-app/internal/profile"
)

const nodeFormat = "lanboat-nodes/v1"

type nodeBundle struct {
	Format        string                 `json:"format"`
	Nodes         []profile.Node         `json:"nodes"`
	Subscriptions []profile.Subscription `json:"subscriptions"`
	RuleOverlays  []profile.RuleOverlay  `json:"ruleOverlays,omitempty"`
	Selected      string                 `json:"selected,omitempty"`
}

// Importing a second time updates the same IDs. Unrelated local nodes stay in place.
func mergeIDs[T any](original, incoming []T, id func(T) string) []T {
	out := slices.Clone(original)
	index := make(map[string]int, len(out))
	for i, x := range out {
		index[id(x)] = i
	}
	for _, x := range incoming {
		if i, ok := index[id(x)]; ok {
			out[i] = x
		} else {
			index[id(x)] = len(out)
			out = append(out, x)
		}
	}
	return out
}
func (a *App) importNodeBundle(data string) (int, error) {
	var b nodeBundle
	if err := json.Unmarshal([]byte(data), &b); err != nil {
		return 0, err
	}
	if b.Format != nodeFormat {
		return 0, errors.New("节点文件版本不受支持")
	}
	sample := profile.Default()
	sample.Nodes, sample.Subscriptions, sample.RuleOverlays, sample.Selected = b.Nodes, b.Subscriptions, b.RuleOverlays, b.Selected
	if err := sample.ValidateSnapshot(); err != nil {
		return 0, err
	}
	subs := map[string]bool{}
	nodes := map[string]bool{}
	for _, s := range b.Subscriptions {
		subs[s.ID] = true
	}
	for _, n := range b.Nodes {
		nodes[n.ID] = true
	}
	for _, o := range b.RuleOverlays {
		if o.SubscriptionID != "" && !subs[o.SubscriptionID] {
			return 0, errors.New("规则订阅不存在")
		}
		for _, id := range o.NodeIDs {
			if !nodes[id] {
				return 0, errors.New("规则节点不存在")
			}
		}
	}
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Nodes = mergeIDs(c.Nodes, b.Nodes, func(n profile.Node) string { return n.ID })
	c.Subscriptions = mergeIDs(c.Subscriptions, b.Subscriptions, func(s profile.Subscription) string { return s.ID })
	c.RuleOverlays = mergeIDs(c.RuleOverlays, b.RuleOverlays, func(o profile.RuleOverlay) string { return o.ID })
	if c.Selected == "" {
		c.Selected = b.Selected
		if c.Selected == "" && len(c.Nodes) > 0 {
			c.Selected = c.Nodes[0].ID
		}
	}
	if err := c.ValidateSnapshot(); err != nil {
		return 0, err
	}
	return len(b.Nodes), a.commit(c)
}
