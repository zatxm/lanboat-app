package profile

import (
	"fmt"
	"github.com/zatxm/lanboat-app/internal/routing"
	"strings"
)

var RuleTypeNames = map[string]string{"domain": "DOMAIN", "domain_suffix": "DOMAIN-SUFFIX", "domain_keyword": "DOMAIN-KEYWORD", "domain_regex": "DOMAIN-REGEX", "ip_cidr": "IP-CIDR", "ip_cidr6": "IP-CIDR6", "dst_port": "DST-PORT", "network": "NETWORK", "rule_set": "RULE-SET", "domain_set": "DOMAIN-SET", "geoip": "GEOIP", "ip_asn": "IP-ASN", "user_agent": "USER-AGENT"}

func (r Rule) Text() string {
	kind := RuleTypeNames[r.Type]
	if kind == "" {
		kind = r.Type
	}
	action := r.Action
	if action == "direct" || action == "proxy" || action == "reject" {
		action = strings.ToUpper(action)
	}
	text := kind + "," + r.Value + "," + action
	if r.NoResolve {
		text += ",no-resolve"
	}
	return text
}
func CustomRouting(rules []Rule) (routing.Parsed, error) {
	var lines []string
	for _, r := range rules {
		if strings.TrimSpace(r.Value) == "" || len(r.Value) > 2048 || strings.ContainsAny(r.Value, "\r\n\x00") || !routing.ValidPolicy(r.Action) {
			return routing.Parsed{}, fmt.Errorf("规则内容或策略无效")
		}
		if !r.Disabled {
			lines = append(lines, r.Text())
		}
	}
	return routing.Parse("[Rule]\n" + strings.Join(lines, "\n"))
}

func HasPolicyTargets(c Config) bool {
	p, err := EffectiveRouting(c)
	if err != nil {
		return false
	}
	builtin := func(action string) bool {
		return action == "" || action == "direct" || action == "proxy" || action == "reject"
	}
	if !builtin(p.Final) {
		return true
	}
	for _, r := range p.Rules {
		if !builtin(r.Action) {
			return true
		}
	}
	for _, r := range c.Rules {
		if !r.Disabled && !builtin(r.Action) {
			return true
		}
	}
	return false
}
