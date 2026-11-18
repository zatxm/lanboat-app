package routing

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

type Request struct {
	Host           string
	IP             netip.Addr
	Port           int
	Network, Agent string
}

func MatchRule(r Rule, req Request) bool {
	switch r.Type {
	case "NATIVE":
		return len(r.SetRules) == 1 && matchCondition(r.SetRules[0], req)
	case "DOMAIN":
		return strings.EqualFold(strings.TrimSuffix(req.Host, "."), r.Value)
	case "DOMAIN-SUFFIX":
		return suffix(req.Host, r.Value)
	case "DOMAIN-KEYWORD":
		return strings.Contains(strings.ToLower(req.Host), strings.ToLower(r.Value))
	case "DOMAIN-REGEX":
		re, err := regexp.Compile(r.Value)
		return err == nil && re.MatchString(req.Host)
	case "IP-CIDR", "IP-CIDR6":
		prefix, _ := netip.ParsePrefix(r.Value)
		return prefix.Contains(req.IP)
	case "GEOIP", "IP-ASN":
		if r.SetPath != "" {
			for _, condition := range r.SetRules {
				if matchCondition(condition, req) {
					return true
				}
			}
			return false
		}
		for _, value := range Prefixes(r.Type, r.Value) {
			prefix, _ := netip.ParsePrefix(value)
			if prefix.Contains(req.IP) {
				return true
			}
		}
	case "DST-PORT":
		return strconv.Itoa(req.Port) == r.Value
	case "NETWORK":
		return strings.EqualFold(req.Network, r.Value)
	case "USER-AGENT":
		return matchUserAgent(r.Value, req.Agent)
	case "RULE-SET", "DOMAIN-SET", "IP-SET", "MRS-DOMAIN", "MRS-IP", "SRS-SET":
		for _, condition := range r.SetRules {
			if matchCondition(condition, req) {
				return true
			}
		}
	}
	return false
}
func suffix(host, value string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	value = strings.TrimPrefix(strings.ToLower(value), ".")
	return host == value || strings.HasSuffix(host, "."+value)
}
func matchCondition(raw map[string]any, req Request) bool {
	hit := true
	destinationPresent, destinationMatch := false, false
	portPresent, portMatch := false, false
	for key, value := range raw {
		if key == "type" || key == "invert" {
			continue
		}
		if key == "mode" {
			continue
		}
		if key == "rules" {
			children, _ := value.([]any)
			childHit := raw["mode"] == "and"
			for _, child := range children {
				m, _ := child.(map[string]any)
				ok := matchCondition(m, req)
				if raw["mode"] == "and" {
					childHit = childHit && ok
				} else {
					childHit = childHit || ok
				}
			}
			hit = hit && childHit
			continue
		}
		values := []any{}
		switch v := value.(type) {
		case []string:
			for _, s := range v {
				values = append(values, s)
			}
		case []int:
			for _, n := range v {
				values = append(values, n)
			}
		case []any:
			values = v
		case string:
			values = []any{v}
		default:
			hit = false
			continue
		}
		matched := false
		for _, v := range values {
			s, _ := v.(string)
			switch key {
			case "domain":
				matched = matched || strings.EqualFold(req.Host, s)
			case "domain_suffix":
				matched = matched || suffix(req.Host, s)
			case "domain_keyword":
				matched = matched || strings.Contains(strings.ToLower(req.Host), strings.ToLower(s))
			case "domain_regex":
				re, err := regexp.Compile(s)
				matched = matched || err == nil && re.MatchString(req.Host)
			case "ip_cidr":
				p, _ := netip.ParsePrefix(s)
				matched = matched || p.Contains(req.IP)
			case "network":
				matched = matched || strings.EqualFold(req.Network, s)
			case "port":
				n := 0
				switch x := v.(type) {
				case int:
					n = x
				case float64:
					n = int(x)
				}
				matched = matched || req.Port == n
			case "port_range":
				a, b, _ := strings.Cut(s, ":")
				lo, _ := strconv.Atoi(a)
				hi, _ := strconv.Atoi(b)
				matched = matched || req.Port >= lo && req.Port <= hi
			}
		}
		switch key {
		case "domain", "domain_suffix", "domain_keyword", "domain_regex", "ip_cidr":
			destinationPresent = true
			destinationMatch = destinationMatch || matched
		case "port", "port_range":
			portPresent = true
			portMatch = portMatch || matched
		default:
			hit = hit && matched
		}
	}
	if destinationPresent {
		hit = hit && destinationMatch
	}
	if portPresent {
		hit = hit && portMatch
	}
	if raw["invert"] == true {
		return !hit
	}
	return hit
}

func matchUserAgent(pattern, agent string) bool {
	pattern = "^" + strings.ReplaceAll(strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*"), `\?`, ".") + "$"
	re, err := regexp.Compile(pattern)
	return err == nil && re.MatchString(agent)
}
