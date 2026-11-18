package desktop

import (
	"errors"
	"github.com/zatxm/lanboat-app/internal/profile"
)

func syncSubscriptionRules(c *profile.Config, sub profile.Subscription, content string, nodes []profile.Node) error {
	overlays := make([]profile.RuleOverlay, 0, len(c.RuleOverlays)+1)
	found := false
	for _, old := range c.RuleOverlays {
		if old.ID != sub.ID {
			overlays = append(overlays, old)
			continue
		}
		found = true
		if content != "" {
			old.Name = sub.Name
			old.Content = content
			old.NodeIDs = nil
			for _, n := range nodes {
				old.NodeIDs = append(old.NodeIDs, n.ID)
			}
			overlays = append(overlays, old)
		}
	}
	if content != "" && !found {
		if len(overlays) >= 1000 {
			return errors.New("附加规则数量超过限制")
		}
		overlay := profile.RuleOverlay{ID: sub.ID, Name: sub.Name, Content: content}
		for _, n := range nodes {
			overlay.NodeIDs = append(overlay.NodeIDs, n.ID)
		}
		for _, s := range c.Subscriptions {
			if s.ID == sub.ID {
				overlay.SubscriptionID = sub.ID
				break
			}
		}
		// New subscriptions are appended to the profile after their rules are prepared.
		if sub.URL != "" {
			overlay.SubscriptionID = sub.ID
		}
		overlays = append(overlays, overlay)
	}
	c.RuleOverlays = overlays
	return nil
}

func removeOverlayNodes(c *profile.Config, deleted map[string]bool) {
	overlays := make([]profile.RuleOverlay, 0, len(c.RuleOverlays))
	for _, o := range c.RuleOverlays {
		ids := make([]string, 0, len(o.NodeIDs))
		for _, id := range o.NodeIDs {
			if !deleted[id] {
				ids = append(ids, id)
			}
		}
		o.NodeIDs = ids
		if len(ids) > 0 || o.SubscriptionID != "" {
			overlays = append(overlays, o)
		}
	}
	c.RuleOverlays = overlays
}
