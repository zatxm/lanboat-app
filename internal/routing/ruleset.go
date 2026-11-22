package routing

import (
	"context"
	"crypto/sha256"
	"github.com/goccy/go-json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	allinone "github.com/zatxm/lanboat-allinone"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/zatxm/lanboat-app/internal/ruledata"
)

// DomainCondition is shared by traffic routing and DNS projection. IP/port/process predicates never become DNS matches.
func DomainCondition(r Rule) map[string]any {
	if r.Type == "NATIVE" && len(r.SetRules) == 1 {
		return domainOnly(r.SetRules[0])
	}
	if r.SetPath != "" && strings.HasPrefix(r.Value, "lanboat://geosite/") {
		return map[string]any{"rule_set": []string{SetTag(r)}}
	}
	key := map[string]string{"DOMAIN": "domain", "DOMAIN-SUFFIX": "domain_suffix", "DOMAIN-KEYWORD": "domain_keyword", "DOMAIN-REGEX": "domain_regex"}[r.Type]
	if key != "" {
		return map[string]any{key: []string{r.Value}}
	}
	if r.Type != "RULE-SET" && r.Type != "DOMAIN-SET" && r.Type != "MRS-DOMAIN" && r.Type != "SRS-SET" {
		return nil
	}
	var rules []any
	for _, raw := range r.SetRules {
		if p := domainOnly(raw); p != nil {
			rules = append(rules, p)
		}
	}
	if len(rules) == 0 {
		return nil
	}
	return map[string]any{"type": "logical", "mode": "or", "rules": rules}
}
func domainOnly(raw map[string]any) map[string]any {
	if raw["type"] == "logical" {
		items, _ := raw["rules"].([]any)
		mode, _ := raw["mode"].(string)
		var projected []any
		for _, item := range items {
			child, ok := item.(map[string]any)
			if !ok {
				return nil
			}
			p := domainOnly(child)
			if p == nil {
				if mode == "and" || raw["invert"] == true {
					return nil
				}
				continue
			}
			projected = append(projected, p)
		}
		if len(projected) == 0 {
			return nil
		}
		return map[string]any{"type": "logical", "mode": mode, "rules": projected, "invert": raw["invert"] == true}
	}
	out := map[string]any{}
	for k, v := range raw {
		switch k {
		case "domain", "domain_suffix", "domain_keyword", "domain_regex":
			out[k] = v
		case "invert":
			out[k] = v
		case "type":
			if v != "default" {
				return nil
			}
		default:
			return nil
		}
	}
	if len(out) == 0 || len(out) == 1 && out["invert"] != nil {
		return nil
	}
	return out
}
func SetTag(r Rule) string {
	return fmt.Sprintf("lanboat-%x", sha256.Sum256([]byte(r.Type+":"+r.Value)))
}

func SetReference(value string) bool {
	if strings.HasPrefix(value, "lanboat://") {
		parts := strings.Split(strings.TrimPrefix(value, "lanboat://"), "/")
		return len(parts) == 2 && (parts[0] == "geoip" || parts[0] == "geosite" || parts[0] == "asn") && parts[1] != "" && parts[1] != "." && parts[1] != ".." && regexp.MustCompile(`^[a-zA-Z0-9!_@.-]+$`).MatchString(parts[1])
	}
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.Fragment == "" || filepath.IsAbs(value)
}

// ResolveSets runs at connection startup, caches downloads, and leaves parsing/validation free of network access.
func ResolveSets(ctx context.Context, p *Parsed, core, dir string) error {
	var current string
	for i, r := range p.Rules {
		reference := r.Value
		if r.Type == "GEOIP" {
			reference = "lanboat://geoip/" + strings.ToLower(r.Value)
		}
		if r.Type == "IP-ASN" {
			reference = "lanboat://asn/AS" + r.Value
		}
		if r.Type != "RULE-SET" && r.Type != "DOMAIN-SET" && r.Type != "IP-SET" && r.Type != "MRS-DOMAIN" && r.Type != "MRS-IP" && r.Type != "SRS-SET" && r.Type != "GEOIP" && r.Type != "IP-ASN" {
			continue
		}
		if strings.HasPrefix(reference, "lanboat://") {
			if current == "" {
				root := filepath.Join(filepath.Dir(dir), "rules-data")
				if err := ruledata.Ensure(root); err != nil {
					return err
				}
				var err error
				current, err = ruledata.Current(root)
				if err != nil {
					return err
				}
			}
			name := strings.TrimPrefix(reference, "lanboat://")
			setPath := filepath.Join(current, "srs", filepath.FromSlash(name)+".srs")
			if _, err := os.Stat(setPath); err != nil {
				return fmt.Errorf("规则集%s不存在：%w", r.Value, err)
			}
			p.Rules[i].SetPath = setPath
			continue
		}
		data, err := readSet(ctx, r.Value, dir, time.Duration(r.SetInterval)*time.Second, r.SetHeaders)
		if err != nil {
			return fmt.Errorf("规则集%s：%w", r.Value, err)
		}
		if r.Type == "SRS-SET" || strings.HasSuffix(strings.Split(r.Value, "?")[0], ".srs") {
            data,err=allinone.DecodeRuleSet(data)
            if err!=nil{return fmt.Errorf("解码规则集失败：%w",err)}
		}
		if r.Type == "MRS-DOMAIN" || r.Type == "MRS-IP" {
			p.Rules[i].SetRules, err = parseMRS(data, r.Type == "MRS-IP")
		} else if r.Type == "DOMAIN-SET" {
			p.Rules[i].SetRules, err = parseDomainSet(data)
		} else if r.Type == "IP-SET" {
			p.Rules[i].SetRules, err = parseIPSet(data)
		} else {
			p.Rules[i].SetRules, err = parseSet(data)
		}
		if err != nil {
			return fmt.Errorf("规则集%s：%w", r.Value, err)
		}
	}
	return nil
}
func readSet(ctx context.Context, source, dir string, maxAge time.Duration, headers ...map[string][]string) ([]byte, error) {
	if filepath.IsAbs(source) {
		f, err := os.Open(source)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return limitedSet(f)
	}
	var requestHeaders map[string][]string
	if len(headers) > 0 {
		requestHeaders = headers[0]
	}
	cacheKey, err := json.Marshal(struct {
		Source  string              `json:"source"`
		Headers map[string][]string `json:"headers,omitempty"`
	}{source, requestHeaders})
	if err != nil {
		return nil, err
	}
	cache := filepath.Join(dir, fmt.Sprintf("ruleset-%x.json", sha256.Sum256(cacheKey)))
	if maxAge <= 0 {
		maxAge = time.Hour
	}
	if info, err := os.Stat(cache); err == nil && time.Since(info.ModTime()) < maxAge {
		data, err := os.ReadFile(cache)
		if err == nil && len(data) <= MaxConfig {
			return data, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range requestHeaders {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" && req.URL.Scheme != "http" || via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return fmt.Errorf("规则集重定向无效")
		}
		return nil
	}}
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	data, err := limitedSet(res.Body)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, ".ruleset-download-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	defer os.Remove(name)
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, cache)
	}
	return data, err
}
func limitedSet(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxConfig+1))
	if err == nil && len(data) > MaxConfig {
		err = fmt.Errorf("规则集不能超过1MiB")
	}
	return data, err
}
func parseSet(data []byte) ([]map[string]any, error) {
	var source struct {
		Version int              `json:"version"`
		Rules   []map[string]any `json:"rules"`
	}
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		if err := json.Unmarshal(data, &source); err != nil {
			return nil, err
		}
		if len(source.Rules) == 0 {
			return nil, fmt.Errorf("规则集内容为空")
		}
		for _, r := range source.Rules {
			if err := validateSetRule(r); err != nil {
				return nil, err
			}
		}
		return source.Rules, nil
	}
	var clash struct {
		Payload []string `yaml:"payload"`
	}
	_ = yaml.Unmarshal(data, &clash)
	lines := clash.Payload
	if len(lines) == 0 {
		lines = strings.Split(string(data), "\n")
	}
	var rules []map[string]any
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.Contains(line, ",") {
			if strings.HasPrefix(line, "+.") || strings.HasPrefix(line, "*.") {
				line = "DOMAIN-SUFFIX," + line[2:]
			} else {
				line = "DOMAIN," + line
			}
		}
		suffix := ",DIRECT"
		if strings.HasSuffix(line, ",no-resolve") {
			line = strings.TrimSuffix(line, ",no-resolve")
			suffix += ",no-resolve"
		}
		p, err := Parse("[Rule]\n" + line + suffix)
		if err != nil {
			return nil, err
		}
		for _, r := range p.Rules {
			cond := DomainCondition(r)
			if cond == nil {
				if r.Type == "IP-CIDR" || r.Type == "IP-CIDR6" {
					cond = map[string]any{"ip_cidr": []string{r.Value}}
				} else {
					return nil, fmt.Errorf("规则集包含不支持的类型%s", r.Type)
				}
			}
			rules = append(rules, cond)
		}
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("规则集内容为空")
	}
	return rules, nil
}
func validateSetRule(r map[string]any) error {
	for k, v := range r {
		switch k {
		case "type":
			if v != "logical" && v != "default" {
				return fmt.Errorf("规则集类型无效")
			}
		case "mode":
			if v != "or" && v != "and" {
				return fmt.Errorf("规则集逻辑无效")
			}
		case "rules":
			children, ok := v.([]any)
			if !ok || len(children) == 0 {
				return fmt.Errorf("规则集逻辑内容无效")
			}
			for _, child := range children {
				m, ok := child.(map[string]any)
				if !ok {
					return fmt.Errorf("规则集格式无效")
				}
				if err := validateSetRule(m); err != nil {
					return err
				}
			}
		case "domain", "domain_suffix", "domain_keyword", "domain_regex", "ip_cidr", "source_ip_cidr", "port", "port_range", "source_port", "source_port_range", "process_name", "process_path", "process_path_regex", "network", "invert", "package_name", "network_type", "network_is_expensive", "network_is_constrained", "wifi_ssid", "wifi_bssid":
			if k == "domain_regex" {
				items, ok := v.([]any)
				if single, yes := v.(string); yes {
					items, ok = []any{single}, true
				}
				if !ok {
					return fmt.Errorf("域名正则需要字符串或列表")
				}
				for _, item := range items {
					str, ok := item.(string)
					if !ok {
						return fmt.Errorf("域名正则无效")
					}
					if _, err := regexp.Compile(str); err != nil {
						return err
					}
				}
			}
		default:
			return fmt.Errorf("不支持的规则集字段%s", k)
		}
	}
	return nil
}

func RuleSetNeedsIP(rules []map[string]any) bool {
	for _, r := range rules {
		if _, ok := r["ip_cidr"]; ok {
			return true
		}
		if children, ok := r["rules"].([]any); ok {
			for _, child := range children {
				if m, ok := child.(map[string]any); ok && RuleSetNeedsIP([]map[string]any{m}) {
					return true
				}
			}
		}
	}
	return false
}

func parseDomainSet(data []byte) ([]map[string]any, error) {
	var clash struct {
		Payload []string `yaml:"payload"`
	}
	_ = yaml.Unmarshal(data, &clash)
	lines := clash.Payload
	if len(lines) == 0 {
		lines = strings.Split(string(data), "\n")
	}
	var rules []map[string]any
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		key := "domain"
		if strings.HasPrefix(line, "+.") || strings.HasPrefix(line, "*.") {
			key = "domain_suffix"
			line = line[2:]
		} else if strings.HasPrefix(line, ".") {
			key = "domain_regex"
			line = `^.+\.` + regexp.QuoteMeta(line[1:]) + `$`
		}
		if line == "" || strings.ContainsAny(line, ",/ :\t\r") {
			return nil, fmt.Errorf("域名集内容无效")
		}
		rules = append(rules, map[string]any{key: []string{line}})
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("域名集内容为空")
	}
	return rules, nil
}

func parseIPSet(data []byte) ([]map[string]any, error) {
	var clash struct {
		Payload []string `yaml:"payload"`
	}
	_ = yaml.Unmarshal(data, &clash)
	lines := clash.Payload
	if len(lines) == 0 {
		lines = strings.Split(string(data), "\n")
	}
	var prefixes []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if ip, err := netip.ParseAddr(line); err == nil {
			line = netip.PrefixFrom(ip, ip.BitLen()).String()
		}
		prefix, err := netip.ParsePrefix(line)
		if err != nil {
			return nil, fmt.Errorf("IP 规则集格式无效: %s", line)
		}
		prefixes = append(prefixes, prefix.Masked().String())
	}
	if len(prefixes) == 0 {
		return nil, fmt.Errorf("规则集内容为空")
	}
	return []map[string]any{{"ip_cidr": prefixes}}, nil
}

// MatchBinary evaluates binary rule sets with the shared in-process core.
func MatchBinary(ctx context.Context, core, setPath string, req Request) (bool, error) {
	targets := []string{}
	kind := filepath.Base(filepath.Dir(setPath))
	hostIP, _ := netip.ParseAddr(req.Host)
	if kind == "geoip" || kind == "asn" {
		if req.IP.IsValid() {
			targets = append(targets, req.IP.String())
		} else if hostIP.IsValid() {
			targets = append(targets, hostIP.String())
		}
	} else if kind == "geosite" {
		if req.Host != "" && !hostIP.IsValid() {
			targets = append(targets, req.Host)
		}
	} else {
		if req.Host != "" {
			targets = append(targets, req.Host)
		}
		if req.IP.IsValid() && req.IP.String() != req.Host {
			targets = append(targets, req.IP.String())
		}
	}
    if err:=ctx.Err();err!=nil{return false,err}
    data,err:=os.ReadFile(setPath);if err!=nil{return false,err}
    for _,target:=range targets {
     matched,err:=allinone.MatchRuleSet(data,target)
     if err!=nil{return false,fmt.Errorf("规则集匹配失败：%w",err)}
     if matched{return true,nil}
    }
    return false,nil
}
