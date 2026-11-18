package engine

import (
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
	"strconv"
	"strings"
)

func coreProfileRules(p *routing.Parsed) ([]any, []any) {
	var rules, sets []any
	seen := map[string]bool{}
	resolved := false
	for _, r := range p.Rules {
		if r.Type == "USER-AGENT" {
			continue
		}

		if !resolved && !r.NoResolve && (r.Type == "IP-CIDR" || r.Type == "IP-CIDR6" || r.Type == "GEOIP" || r.Type == "IP-ASN" || r.Type == "IP-SET" || r.Type == "MRS-IP" || (r.Type == "NATIVE" || r.Type == "SRS-SET") && routing.RuleSetNeedsIP(r.SetRules) || r.Type == "RULE-SET" && (routing.RuleSetNeedsIP(r.SetRules) || strings.HasPrefix(r.Value, "lanboat://geoip/"))) {
			rules = append(rules, map[string]any{"action": "resolve"})
			resolved = true
		}
		rule := map[string]any{"action": "route", "outbound": r.Action}
		if r.Action == "reject" {
			rule["action"] = "reject"
			delete(rule, "outbound")
		}
		if r.Type == "NATIVE" && len(r.SetRules) == 1 {
			for k, value := range r.SetRules[0] {
				rule[k] = value
			}
		} else if cond := routing.DomainCondition(r); cond != nil && r.Type != "RULE-SET" && r.Type != "DOMAIN-SET" && r.Type != "SRS-SET" {
			for k, value := range cond {
				rule[k] = value
			}
		} else if r.SetPath != "" {
			tag := routing.SetTag(r)
			rule["rule_set"] = []string{tag}
			if !seen[tag] {
				seen[tag] = true
				sets = append(sets, map[string]any{"type": "local", "tag": tag, "format": "binary", "path": r.SetPath})
			}
		} else if r.Type == "RULE-SET" || r.Type == "DOMAIN-SET" || r.Type == "IP-SET" || r.Type == "MRS-DOMAIN" || r.Type == "MRS-IP" || r.Type == "SRS-SET" {
			rule["type"] = "logical"
			rule["mode"] = "or"
			rule["rules"] = r.SetRules
		}
		switch r.Type {
		case "DOMAIN":
			rule["domain"] = []string{r.Value}
		case "DOMAIN-SUFFIX":
			rule["domain_suffix"] = []string{r.Value}
		case "DOMAIN-KEYWORD":
			rule["domain_keyword"] = []string{r.Value}
		case "IP-CIDR", "IP-CIDR6":
			rule["ip_cidr"] = []string{r.Value}
		case "DST-PORT":
			port, _ := strconv.Atoi(r.Value)
			rule["port"] = []int{port}
		case "NETWORK":
			rule["network"] = []string{r.Value}
		case "GEOIP", "IP-ASN":
			if r.SetPath != "" {
				break
			}
			tag := strings.ToLower(r.Type) + "-" + r.Value
			rule["rule_set"] = []string{tag}
			if !seen[tag] {
				seen[tag] = true
				sets = append(sets, map[string]any{"type": "inline", "tag": tag, "rules": []any{map[string]any{"ip_cidr": routing.Prefixes(r.Type, r.Value)}}})
			}
		}
		rules = append(rules, rule)
	}
	return rules, sets
}
func applyCoreRouting(config map[string]any, c profile.Config) {
	p := c.Routing
	if p == nil {
		return
	}
	route := config["route"].(map[string]any)
	if c.Settings.Mode == "rule" {
		rules, sets := coreProfileRules(p)
		existing := route["rules"].([]any)
		if len(existing) > 0 {
			existing = existing[:len(existing)-1]
		}
		route["rules"] = append(existing, rules...)
		route["final"] = p.Final
		if p.Final == "reject" {
			route["rules"] = append(route["rules"].([]any), map[string]any{"action": "reject"})
			route["final"] = "direct"
		}
		if len(sets) > 0 {
			route["rule_set"] = sets
		}
	}
	if c.ForceProxyPort > 0 {
		config["inbounds"] = append(config["inbounds"].([]any), map[string]any{"type": "mixed", "tag": "ua-proxy", "listen": "127.0.0.1", "listen_port": c.ForceProxyPort})
		route["rules"] = append([]any{map[string]any{"inbound": []string{"ua-proxy"}, "action": "route", "outbound": "proxy"}}, route["rules"].([]any)...)
	}
	if c.ForceDirectPort > 0 {
		config["inbounds"] = append(config["inbounds"].([]any), map[string]any{"type": "mixed", "tag": "ua-direct", "listen": "127.0.0.1", "listen_port": c.ForceDirectPort})
		route["rules"] = append([]any{map[string]any{"inbound": []string{"ua-direct"}, "action": "route", "outbound": "direct"}}, route["rules"].([]any)...)
	}
	for action, port := range c.PolicyPorts {
		tag := "policy-in-" + strconv.Itoa(port)
		config["inbounds"] = append(config["inbounds"].([]any), map[string]any{"type": "mixed", "tag": tag, "listen": "127.0.0.1", "listen_port": port})
		route["rules"] = append([]any{map[string]any{"inbound": []string{tag}, "action": "route", "outbound": action}}, route["rules"].([]any)...)
	}
	if len(p.Hosts) > 0 {
		hosts := make([]string, 0, len(p.Hosts))
		for host := range p.Hosts {
			hosts = append(hosts, host)
		}
		route["rules"] = append([]any{map[string]any{"domain": hosts, "action": "resolve", "server": "hosts"}}, route["rules"].([]any)...)
	}
}
