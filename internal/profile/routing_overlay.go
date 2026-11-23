package profile

import (
	"fmt"
	"slices"
	"strings"

	"github.com/zatxm/lanboat-app/internal/routing"
)

func overlayHasFinal(content string) bool {
	ruleSection := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			ruleSection = strings.EqualFold(line, "[Rule]")
			continue
		}
		if ruleSection {
			kind, _, _ := strings.Cut(line, ",")
			if strings.EqualFold(kind, "FINAL") || strings.EqualFold(kind, "MATCH") {
				return true
			}
		}
	}
	return false
}

func applyNodeRuleOverlay(p *routing.Parsed, c Config) error {
	if c.Selected == "" {
		return nil
	}
	var chosen *RuleOverlay
	for i := range c.RuleOverlays {
		if slices.Contains(c.RuleOverlays[i].NodeIDs, c.Selected) {
			if chosen != nil {
				return fmt.Errorf("节点关联了多个附加规则来源")
			}
			chosen = &c.RuleOverlays[i]
		}
	}
	if chosen == nil {
		return nil
	}
	extra, err := routing.Parse(chosen.Content)
	if err != nil {
		return fmt.Errorf("附加规则 %s：%w", chosen.Name, err)
	}
	groups := map[string]string{}
	for i, g := range extra.Groups {
		groups[g.Name] = fmt.Sprintf("overlay:%s:%d", chosen.ID, i)
	}
	nodes := map[string]string{}
	wanted := make(map[string]bool, len(chosen.NodeIDs))
	for _, id := range chosen.NodeIDs {
		wanted[id] = true
	}
	for _, n := range c.Nodes {
		if wanted[n.ID] {
			if previous := nodes[n.Name]; previous != "" && previous != n.ID {
				return fmt.Errorf("附加规则中的节点名称重复：%s", n.Name)
			}
			nodes[n.Name] = n.ID
		}
	}
	for _, g := range c.PolicyGroups {
		groups[g.Ref()] = g.Ref()
	}
	resolve := func(action string) (string, error) {
		if name := groups[action]; name != "" {
			return name, nil
		}
		if action == "direct" || action == "proxy" || action == "reject" {
			return action, nil
		}
		if id := nodes[action]; id != "" {
			return "node:" + id, nil
		}
		if strings.HasPrefix(action, "node:") && wanted[strings.TrimPrefix(action, "node:")] {
			return action, nil
		}
		return "", fmt.Errorf("附加规则的策略不存在：%s", action)
	}
	for i := range extra.Groups {
		g := &extra.Groups[i]
		g.Name = groups[g.Name]
		for j := range g.Members {
			g.Members[j], err = resolve(g.Members[j])
			if err != nil {
				return err
			}
		}
		if g.Selected != "" {
			g.Selected, err = resolve(g.Selected)
			if err != nil {
				return err
			}
		}
	}
	for i := range extra.Rules {
		extra.Rules[i].Action, err = resolve(extra.Rules[i].Action)
		if err != nil {
			return err
		}
	}
	p.Groups = append(extra.Groups, p.Groups...)
	p.PriorityRules = len(extra.Rules)
	if overlayHasFinal(chosen.Content) {
		p.Final, err = resolve(extra.Final)
		if err != nil {
			return err
		}
		p.Rules = extra.Rules
		p.OverrideFinal = true
	} else {
		p.Rules = append(extra.Rules, p.Rules...)
	}
	return nil
}

// InsertCustomRouting places application rules after the selected node's rules.
func InsertCustomRouting(p *routing.Parsed, rules []Rule) error {
	if p.OverrideFinal {
		return nil
	}
	manual, err := CustomRouting(rules)
	if err != nil {
		return err
	}
	at := p.PriorityRules
	if at < 0 || at > len(p.Rules) {
		return fmt.Errorf("附加规则顺序无效")
	}
	combined := make([]routing.Rule, 0, len(p.Rules)+len(manual.Rules))
	combined = append(combined, p.Rules[:at]...)
	combined = append(combined, manual.Rules...)
	combined = append(combined, p.Rules[at:]...)
	p.Rules = combined
	return nil
}

func ValidateRuleOverlays(overlays []RuleOverlay) error {
	if len(overlays) > 1000 {
		return fmt.Errorf("附加规则数量超过限制")
	}
	ids := map[string]bool{}
	for _, overlay := range overlays {
		if overlay.ID == "" || len(overlay.ID) > 80 || ids[overlay.ID] || strings.ContainsAny(overlay.ID, ",=\r\n\x00[]") {
			return fmt.Errorf("附加规则标识无效或重复")
		}
		ids[overlay.ID] = true
		if _, err := routing.Parse(overlay.Content); err != nil {
			return fmt.Errorf("附加规则 %s：%w", overlay.Name, err)
		}
		if len(overlay.NodeIDs) > MaxNodes {
			return fmt.Errorf("附加规则节点数量超过限制")
		}
	}
	return nil
}

// Preserve generated legacy subscription files while moving their distinct rules to the node source.
func MigrateSubscriptionRules(c *Config) (bool, error) {
	var base routing.Parsed
	for _, f := range c.Files {
		if f.ID == "default" {
			var err error
			base, err = routing.Parse(f.Content)
			if err != nil {
				return false, err
			}
			break
		}
	}
	baseline := map[string]bool{}
	key := func(r routing.Rule) string {
		return r.Text()
	}
	for _, r := range base.Rules {
		baseline[key(r)] = true
	}
	existing := map[string]bool{}
	for _, o := range c.RuleOverlays {
		existing[o.ID] = true
	}
	changed := false
	for _, sub := range c.Subscriptions {
		if existing[sub.ID] {
			continue
		}
		var old *ConfigFile
		for i := range c.Files {
			if c.Files[i].ID == "subscription-"+sub.ID {
				old = &c.Files[i]
				break
			}
		}
		if old == nil {
			continue
		}
		parsed, err := routing.Parse(old.Content)
		if err != nil {
			return false, err
		}
		var text strings.Builder
		if len(parsed.Groups) > 0 {
			text.WriteString("[Proxy Group]\n")
			for _, group := range parsed.Groups {
				text.WriteString(group.Text() + "\n")
			}
		}
		text.WriteString("[Rule]\n")
		for _, r := range parsed.Rules {
			if !baseline[key(r)] {
				text.WriteString(r.Text() + "\n")
			}
		}
		if parsed.Final != base.Final {
			text.WriteString("FINAL," + rulePolicyText(parsed.Final) + "\n")
		}
		var ids []string
		for _, n := range c.Nodes {
			if n.Source == sub.ID {
				ids = append(ids, n.ID)
			}
		}
		if sub.KeepRules && (len(parsed.Groups) > 0 || strings.Count(text.String(), "\n") > 1) {
			c.RuleOverlays = append(c.RuleOverlays, RuleOverlay{ID: sub.ID, SubscriptionID: sub.ID, Name: sub.Name, Content: text.String(), NodeIDs: ids})
		}
		if c.ActiveFile == old.ID {
			c.ActiveFile = "default"
		}
		// Keep the old file as an ordinary configuration for review or manual recovery.
		old.ID = ID()
		changed = true
	}
	return changed, ValidateRuleOverlays(c.RuleOverlays)
}
