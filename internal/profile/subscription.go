package profile

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"regexp"
	"strings"

	"github.com/zatxm/lanboat-app/internal/routing"
	"github.com/zatxm/lanboat-app/internal/ruledata"
)

func (s Subscription) Validate() error {
	u, err := url.Parse(s.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || len(s.URL) > 4096 {
		return errors.New("订阅地址必须是有效的 HTTP 或 HTTPS URL")
	}
	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 160 {
		return errors.New("请填写订阅备注（最多 160 字节）")
	}
	for _, value := range []string{s.UDP, s.Insecure, s.FastOpen} {
		if value != "" && value != "true" && value != "false" {
			return errors.New("订阅开关选项无效")
		}
	}
	if s.MaxNodes < 0 || s.MaxNodes > MaxNodes {
		return errors.New("最大值需为 1–100000，留空表示不限")
	}
	if len(s.UserAgent) > 512 || strings.ContainsAny(s.UserAgent, "\r\n\x00") {
		return errors.New("用户代理无效")
	}
	if len(s.Password) > 4096 {
		return errors.New("订阅密码不能超过 4096 字节")
	}
	if len(s.Filter) > 1024 {
		return errors.New("过滤表达式不能超过 1024 字节")
	}
	if _, err := regexp.Compile("(?i)" + s.Filter); err != nil {
		return errors.New("过滤正则表达式无效")
	}
	return nil
}

// Subscription overrides leave unspecified node options intact.
func ParseSubscription(content string, s Subscription, defaultConfig string) ([]Node, string, error) {
	if err := s.Validate(); err != nil {
		return nil, "", err
	}
	limit := MaxNodes
	var rules []string
	var capture *[]string
	if s.KeepRules {
		capture = &rules
	}
	nodes, err := parseImport(content, limit, capture)
	if err != nil {
		return nil, "", err
	}
	if s.Filter != "" {
		match := regexp.MustCompile("(?i)" + s.Filter)
		filtered := nodes[:0]
		for _, n := range nodes {
			if match.MatchString(n.Name) {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
		if len(nodes) == 0 {
			return nil, "", errors.New("过滤后没有可用节点，已保留原订阅")
		}
	}
	if s.MaxNodes > 0 && len(nodes) > s.MaxNodes {
		// Sample without replacement, retaining the provider's order for the chosen nodes.
		remaining, keep := len(nodes), s.MaxNodes
		selected := nodes[:0]
		for _, n := range nodes {
			if rand.IntN(remaining) < keep {
				selected = append(selected, n)
				keep--
			}
			remaining--
		}
		nodes = selected
	}
	for i := range nodes {
		for key, value := range map[string]string{"udp": s.UDP, "skip-cert-verify": s.Insecure, "tfo": s.FastOpen} {
			if value != "" {
				nodes[i].SetOption(key, value == "true")
			}
		}
	}
	merged := ""
	if len(rules) > 0 {
		merged, err = mergeSubscriptionRulesKnown(defaultConfig, rules, nodes)
		if err != nil {
			return nil, "", err
		}
	}
	return nodes, merged, nil
}

func rulePolicyText(action string) string {
	if action == "direct" || action == "proxy" || action == "reject" {
		return strings.ToUpper(action)
	}
	return action
}

func mergeSubscriptionRulesKnown(defaultConfig string, lines []string, nodes []Node) (string, error) {
	allowed := map[string]bool{}
	for _, n := range nodes {
		allowed[n.Name] = true
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "@GROUP ") {
			name, _, _ := strings.Cut(strings.TrimPrefix(line, "@GROUP "), "=")
			allowed[strings.TrimSpace(name)] = true
		}
	}
	return mergeRules(defaultConfig, lines, allowed)
}

func mergeRules(defaultConfig string, lines []string, allowed map[string]bool) (string, error) {
	key := func(r routing.Rule) string {
		value := r.Value
		if r.Type != "USER-AGENT" && r.Type != "NATIVE" {
			value = strings.ToLower(value)
		}
		return r.Type + "," + value
	}
	overrides := make(map[string]bool, len(lines))
	seen := make(map[string]bool, len(lines))
	var extra strings.Builder
	final := ""
	var groupLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "@GROUP ") {
			groupLines = append(groupLines, strings.TrimPrefix(line, "@GROUP "))
			continue
		}
		parts := strings.Split(line, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		kind := strings.ToUpper(parts[0])
		terminal := kind == "MATCH" || kind == "FINAL"
		policyIndex := 2
		if terminal {
			policyIndex = 1
		}
		if len(parts) <= policyIndex {
			return "", errors.New("订阅分流规则格式无效")
		}
		parts[0] = kind
		if len(parts) > 2 && (kind == "GEOSITE" || kind == "GEOIP" && !ruledata.ValidRegion(strings.ToLower(parts[1]))) {
			category := strings.ToLower(parts[1])
			if category == "" || strings.ContainsAny(category, "/\\?# \t\r\n") {
				return "", fmt.Errorf("订阅规则集类别无效：%s", parts[1])
			}
			parts[0] = "RULE-SET"
			parts[1] = "lanboat://" + strings.ToLower(kind) + "/" + category
		}
		switch strings.ToUpper(parts[policyIndex]) {
		case "DIRECT", "REJECT", "PROXY":
			parts[policyIndex] = strings.ToUpper(parts[policyIndex])
		case "REJECT-DROP":
			parts[policyIndex] = "REJECT"
		default:
			if !allowed[parts[policyIndex]] {
				return "", fmt.Errorf("订阅规则包含无法保留的策略：%s", parts[policyIndex])
			}
		}
		parsed, err := routing.Parse("[Rule]\n" + strings.Join(parts, ","))
		if err != nil {
			return "", fmt.Errorf("订阅分流规则：%w", err)
		}
		if terminal {
			final = "FINAL," + rulePolicyText(parsed.Final) + "\n"
			continue
		}
		for _, r := range parsed.Rules {
			normalized := r.Text()
			if seen[normalized] {
				continue
			}
			seen[normalized], overrides[key(r)] = true, true
			extra.WriteString(normalized + "\n")
		}
	}
	if len(groupLines) > 0 {
		groups, err := routing.Parse("[Proxy Group]\n" + strings.Join(groupLines, "\n"))
		if err != nil {
			return "", fmt.Errorf("订阅代理分组：%w", err)
		}
		for _, group := range groups.Groups {
			for _, member := range group.Members {
				if member != "direct" && member != "proxy" && !allowed[member] {
					return "", fmt.Errorf("订阅分组包含无法保留的策略：%s", member)
				}
			}
		}
	}
	if _, err := routing.Parse(defaultConfig); err != nil {
		return "", err
	}
	var merged strings.Builder
	if len(groupLines) > 0 {
		merged.WriteString("[Proxy Group]\n" + strings.Join(groupLines, "\n") + "\n")
	}
	ruleSection, inserted, finalWritten := false, false, false
	writeFinal := func() {
		if final != "" && !finalWritten {
			merged.WriteString(final)
			finalWritten = true
		}
	}
	for _, line := range strings.SplitAfter(defaultConfig, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			if ruleSection {
				writeFinal()
			}
			ruleSection = strings.EqualFold(trim, "[Rule]")
			merged.WriteString(line)
			if ruleSection && !inserted {
				if !strings.HasSuffix(line, "\n") {
					merged.WriteByte('\n')
				}
				merged.WriteString(extra.String())
				inserted = true
			}
			continue
		}
		if ruleSection && trim != "" && !strings.HasPrefix(trim, "#") && !strings.HasPrefix(trim, "//") {
			kind, _, _ := strings.Cut(trim, ",")
			if (strings.EqualFold(kind, "FINAL") || strings.EqualFold(kind, "MATCH")) && final != "" {
				continue
			}
			parsed, err := routing.Parse("[Rule]\n" + trim)
			if err != nil {
				return "", err
			}
			if len(parsed.Rules) > 0 && overrides[key(parsed.Rules[0])] {
				continue
			}
		}
		merged.WriteString(line)
	}
	if !inserted {
		merged.WriteString("\n[Rule]\n" + extra.String())
	}
	if final != "" && !finalWritten {
		if !strings.HasSuffix(merged.String(), "\n") {
			merged.WriteByte('\n')
		}
		writeFinal()
	}
	content := merged.String()
	if _, err := routing.Parse(content); err != nil {
		return "", err
	}
	return content, nil
}
