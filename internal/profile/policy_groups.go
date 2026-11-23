package profile

import (
	"fmt"
	"slices"
	"strings"

	"github.com/zatxm/lanboat-app/internal/routing"
)

type PolicyGroup struct {
	ID         string `json:"id"`
	Source     string `json:"source,omitempty"`
	SourceName string `json:"sourceName,omitempty"`
	routing.Group
}

func (g PolicyGroup) Ref() string { return "group:" + g.ID }

func ClonePolicyGroups(groups []PolicyGroup) []PolicyGroup {
	result := slices.Clone(groups)
	for i := range result {
		result[i].Members = slices.Clone(result[i].Members)
	}
	return result
}

// ImportPolicyGroups lifts definitions out of an imported configuration.
// Reimporting the same source and name retains the existing reference.
func (c *Config) ImportPolicyGroups(content, source string, nodes []Node) (string, error) {
	p, err := routing.Parse(content)
	if err != nil {
		return "", err
	}
	targets := make(map[string]string, len(c.PolicyGroups)+len(p.Groups))
	scoped := nodes != nil
	if !scoped && len(p.Groups) > 0 {
		nodes = c.Nodes
	}
	nodeTargets := make(map[string]string, len(nodes))
	for _, n := range nodes {
		nodeTargets[n.Name] = "node:" + n.ID
	}
	used := make(map[string]bool, len(c.PolicyGroups)+len(p.Groups))
	prior := map[string]int{}
	for i, g := range c.PolicyGroups {
		used[g.Name] = true
		if !scoped {
			targets[g.Name] = g.Ref()
		}
		if g.Source == source {
			prior[g.SourceName] = i
		}
	}
	if len(p.Groups) != 0 {
		c.PolicyGroups = slices.Clone(c.PolicyGroups)
	}
	indexes := make([]int, len(p.Groups))
	for i, definition := range p.Groups {
		at, exists := prior[definition.Name]
		if !exists {
			at = len(c.PolicyGroups)
			c.PolicyGroups = append(c.PolicyGroups, PolicyGroup{
				ID: ID(), Source: source, SourceName: definition.Name,
				Group: routing.Group{Name: availablePolicyName(definition.Name, used)},
			})
		}
		g := c.PolicyGroups[at]
		used[g.Name] = true
		targets[definition.Name] = g.Ref()
		indexes[i] = at
	}
	resolve := func(value string) string {
		if target, ok := targets[value]; ok && value != "proxy" && value != "direct" && value != "reject" {
			return target
		}
		return value
	}
	resolveMember := func(value string) string {
		if resolved := resolve(value); resolved != value {
			return resolved
		}
		switch value {
		case "direct", "proxy", "reject":
			return value
		}
		if target := nodeTargets[value]; target != "" {
			return target
		}
		return value
	}
	for i, g := range p.Groups {
		group := &c.PolicyGroups[indexes[i]]
		g.Name, g.Line = group.Name, 0
		for j := range g.Members {
			g.Members[j] = resolveMember(g.Members[j])
		}
		g.Selected = resolveMember(g.Selected)
		group.Group = g
	}
	if len(targets) == 0 && len(p.Groups) == 0 {
		return content, nil
	}
	return routing.RewritePolicies(content, p, resolve), nil
}

func availablePolicyName(name string, used map[string]bool) string {
	if !used[name] && !strings.HasPrefix(name, "group:") {
		return name
	}
	if strings.HasPrefix(name, "group:") || len(name) > 185 {
		name = "Policy group"
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%d)", name, i)
		if !used[candidate] {
			return candidate
		}
	}
}

// MergePolicyGroups preserves existing IDs on node bundle import and resolves
// display-name conflicts without rewriting any reference.
func MergePolicyGroups(current, incoming []PolicyGroup) []PolicyGroup {
	out := slices.Clone(current)
	ids := make(map[string]bool, len(current))
	names := make(map[string]bool, len(current))
	for _, g := range current {
		ids[g.ID], names[g.Name] = true, true
	}
	for _, g := range incoming {
		if ids[g.ID] {
			continue
		}
		g.Name = availablePolicyName(g.Name, names)
		ids[g.ID], names[g.Name] = true, true
		out = append(out, g)
	}
	return out
}

func (c Config) ValidatePolicyGroups() error {
	groups := make(map[string]PolicyGroup, len(c.PolicyGroups))
	names := make(map[string]bool, len(c.PolicyGroups))
	for _, g := range c.PolicyGroups {
		if g.ID == "" || !routing.ValidPolicy(g.Ref()) || groups[g.Ref()].ID != "" || names[g.Name] ||
			strings.HasPrefix(g.Name, "group:") || !routing.ValidPolicy(g.Name) {
			return fmt.Errorf("策略组标识或名称无效或重复：%s", g.Name)
		}
		for _, member := range g.Members {
			if !routing.ValidPolicy(member) {
				return fmt.Errorf("策略组成员无效")
			}
		}
		if g.Selected != "" && !slices.Contains(g.Members, g.Selected) {
			return fmt.Errorf("所选策略不在分组内")
		}
		if strings.ContainsAny(g.URL, ",\r\n\x00") {
			return fmt.Errorf("策略组成员或测试URL无效")
		}
		if _, err := routing.Parse("[Proxy Group]\n" + g.Text()); err != nil {
			return err
		}
		groups[g.Ref()], names[g.Name] = g, true
	}

	seen := make(map[string]uint8, len(groups))
	var check func(string) error
	check = func(ref string) error {
		if !strings.HasPrefix(ref, "group:") {
			return nil
		}
		g, ok := groups[ref]
		if !ok {
			return fmt.Errorf("规则或策略组仍引用已删除或不存在的策略组")
		}
		switch seen[ref] {
		case 1:
			return fmt.Errorf("策略组循环引用：%s", g.Name)
		case 2:
			return nil
		}
		seen[ref] = 1
		for _, member := range g.Members {
			if err := check(member); err != nil {
				return err
			}
		}
		seen[ref] = 2
		return nil
	}
	for ref := range groups {
		if err := check(ref); err != nil {
			return err
		}
	}
	for _, r := range c.Rules {
		if err := check(r.Action); err != nil {
			return err
		}
	}
	validateContent := func(content string) error {
		if !strings.Contains(content, "group:") {
			return nil
		}
		p, err := routing.Parse(content)
		if err != nil {
			return err
		}
		routing.InspectPolicies(content, p, func(ref string) {
			if err == nil {
				err = check(ref)
			}
		})
		return err
	}
	for _, f := range c.Files {
		if err := validateContent(f.Content); err != nil {
			return err
		}
	}
	for _, o := range c.RuleOverlays {
		if err := validateContent(o.Content); err != nil {
			return err
		}
	}
	return nil
}

// ReferencedPolicyGroups returns the transitive set of shared groups required
// by the supplied configuration files or node overlays.
func (c Config) ReferencedPolicyGroups(contents []string) ([]PolicyGroup, error) {
	byRef := make(map[string]PolicyGroup, len(c.PolicyGroups))
	for _, g := range c.PolicyGroups {
		byRef[g.Ref()] = g
	}
	var result []PolicyGroup
	var visit func(string)
	visit = func(ref string) {
		if g, ok := byRef[ref]; ok {
			delete(byRef, ref)
			result = append(result, g)
			for _, child := range g.Members {
				visit(child)
			}
		}
	}
	for _, content := range contents {
		if !strings.Contains(content, "group:") {
			continue
		}
		p, err := routing.Parse(content)
		if err != nil {
			return nil, err
		}
		routing.InspectPolicies(content, p, visit)
	}
	return result, nil
}

func (c Config) ExportRoutingConfig(content string) (string, error) {
	groups, err := c.ReferencedPolicyGroups([]string{content})
	if err != nil || len(groups) == 0 {
		return content, err
	}
	p, err := routing.Parse(content)
	if err != nil {
		return "", err
	}
	used := map[string]bool{}
	for _, g := range p.Groups {
		used[g.Name] = true
	}
	names := make(map[string]string, len(groups))
	for _, g := range groups {
		name := availablePolicyName(g.Name, used)
		names[g.Ref()], used[name] = name, true
	}
	resolve := func(ref string) string {
		if name, ok := names[ref]; ok {
			return name
		}
		return ref
	}
	p.Groups = nil
	content = routing.RewritePolicies(content, p, resolve)
	definitions := make([]string, 0, len(groups))
	for _, shared := range groups {
		g := shared.Group
		g.Name = names[shared.Ref()]
		g.Members = slices.Clone(g.Members)
		for i := range g.Members {
			g.Members[i] = resolve(g.Members[i])
		}
		g.Selected = resolve(g.Selected)
		definitions = append(definitions, g.Text())
	}
	return routing.InsertSectionLine(content, "Proxy Group", strings.Join(definitions, "\n"))
}

// appendUsedPolicyGroups keeps an unrelated group edit from restarting a connection.
func (c Config) appendUsedPolicyGroups(p *routing.Parsed) {
	if len(c.PolicyGroups) == 0 {
		return
	}
	byRef := make(map[string]PolicyGroup, len(c.PolicyGroups))
	for _, g := range c.PolicyGroups {
		byRef[g.Ref()] = g
	}
	var add func(string)
	add = func(ref string) {
		g, ok := byRef[ref]
		if !ok {
			return
		}
		delete(byRef, ref)
		def := g.Group
		def.Name, def.Line = ref, 0
		p.Groups = append(p.Groups, def)
		for _, child := range def.Members {
			add(child)
		}
	}
	for _, g := range p.Groups {
		for _, member := range g.Members {
			add(member)
		}
	}
	add(p.Final)
	for _, r := range p.Rules {
		add(r.Action)
	}
	if !p.OverrideFinal {
		for _, r := range c.Rules {
			if !r.Disabled {
				add(r.Action)
			}
		}
	}
}
