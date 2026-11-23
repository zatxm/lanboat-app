// Package routing reads the portable parts of Shadowrocket configuration files.
package routing

import (
	"bufio"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/zatxm/lanboat-app/internal/ruledata"
)

//go:embed assets/default.conf
var DefaultConfig string

//go:embed assets/geoip-cn.json
var countryData []byte

//go:embed assets/as132203.json
var asnData []byte

type Rule struct {
	Type, Value, Action string
	SetRules            []map[string]any
	SetPath             string
	NoResolve           bool
	SetInterval         int
	SetHeaders          map[string][]string
	Line                int
}
type Rewrite struct {
	Pattern, Replacement string
	Status               int
}
type Parsed struct {
	PriorityRules int
	OverrideFinal bool
	Groups        []Group
	Rules         []Rule
	Final         string
	DNS           []string
	Hosts         map[string][]string
	Rewrites      []Rewrite
	General       map[string]string
	Warnings      []string
}

const MaxConfig = 1 << 20

func Parse(content string) (Parsed, error) {
	p := Parsed{Final: "proxy", Hosts: map[string][]string{}, General: map[string]string{}}
	if len(content) > MaxConfig {
		return p, errors.New("配置不能超过 1 MiB")
	}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(strings.TrimPrefix(content, "\ufeff")))
	scanner.Buffer(make([]byte, 4096), MaxConfig)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") || strings.HasPrefix(text, "//") {
			continue
		}
		fail := func(message string) error { return fmt.Errorf("第 %d 行：%s", line, message) }
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			section = strings.ToLower(strings.TrimSpace(text[1 : len(text)-1]))
			continue
		}
		switch section {
		case "general":
			key, value, ok := strings.Cut(text, "=")
			if !ok {
				return p, fail("配置项缺少 =")
			}
			key = strings.ToLower(strings.TrimSpace(key))
			value = strings.TrimSpace(value)
			p.General[key] = value
			if key != "dns-server" && key != "ipv6" && key != "prefer-ipv6" {
				p.Warnings = appendUnique(p.Warnings, "[General] 当前执行 dns-server、ipv6、prefer-ipv6，其余参数保留")
			}
			if (key == "ipv6" || key == "prefer-ipv6") && value != "true" && value != "false" {
				return p, fail("IPv6 参数需要 true 或 false")
			}
			if key == "dns-server" {
				for _, v := range strings.Split(value, ",") {
					v = strings.TrimSpace(v)
					if v != "" {
						if v != "system" {
							if _, err := netip.ParseAddr(v); err != nil {
								u, e := url.Parse(v)
								if e != nil || (u.Scheme != "https" && u.Scheme != "tls") || u.Hostname() == "" || u.User != nil {
									return p, fail("DNS 需要 system、IP、https:// 或 tls:// 地址")
								}
							}
						}
						p.DNS = append(p.DNS, v)
					}
				}
			}
		case "proxy group", "proxy-group":
			g, err := parseGroup(text)
			if err != nil {
				return p, fail(err.Error())
			}
			for _, old := range p.Groups {
				if old.Name == g.Name {
					return p, fail("策略组名称重复")
				}
			}
			g.Line = line
			p.Groups = append(p.Groups, g)
		case "rule":
			values := strings.Split(text, ",")
			if strings.HasPrefix(strings.ToUpper(text), "DOMAIN-REGEX,") {
				tail := text
				flags := []string{}
				for {
					index := strings.LastIndexByte(tail, ',')
					if index < 0 {
						break
					}
					value := strings.TrimSpace(tail[index+1:])
					if value != "no-resolve" && value != "force-remote-dns" {
						break
					}
					flags = append([]string{value}, flags...)
					tail = tail[:index]
				}
				first, last := strings.IndexByte(tail, ','), strings.LastIndexByte(tail, ',')
				if first != last {
					values = append([]string{tail[:first], tail[first+1 : last], tail[last+1:]}, flags...)
				}
			}
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
			}
			if len(values) < 2 {
				return p, fail("规则格式无效")
			}
			kind := strings.ToUpper(values[0])
			if kind == "FINAL" || kind == "MATCH" {
				action, ok := policy(values[1])
				if !ok {
					return p, fail("规则策略无效")
				}
				p.Final = action
				continue
			}
			if len(values) < 3 {
				return p, fail("规则缺少策略")
			}
			action, ok := policy(values[2])
			if !ok {
				return p, fail("规则策略无效")
			}
			r := Rule{Type: kind, Value: values[1], Action: action, Line: line}
			for _, v := range values[3:] {
				if v == "no-resolve" {
					r.NoResolve = true
				} else if strings.HasPrefix(v, "interval=") {
					if kind != "RULE-SET" && kind != "DOMAIN-SET" && kind != "IP-SET" && kind != "MRS-DOMAIN" && kind != "MRS-IP" && kind != "SRS-SET" {
						return p, fail("interval 只能用于规则集")
					}
					seconds, err := strconv.Atoi(strings.TrimPrefix(v, "interval="))
					if err != nil || seconds < 1 || seconds > 31536000 {
						return p, fail("规则集 interval 必须为 1–31536000 秒")
					}
					r.SetInterval = seconds
				} else if strings.HasPrefix(v, "headers=") {
					if kind != "RULE-SET" && kind != "DOMAIN-SET" && kind != "IP-SET" && kind != "MRS-DOMAIN" && kind != "MRS-IP" && kind != "SRS-SET" {
						return p, fail("headers 只能用于规则集")
					}
					raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(v, "headers="))
					if err != nil || len(raw) > 16*1024 || json.Unmarshal(raw, &r.SetHeaders) != nil || len(r.SetHeaders) == 0 {
						return p, fail("规则集请求头格式无效")
					}
					if err := validateSetHeaders(r.SetHeaders); err != nil {
						return p, fail(err.Error())
					}
				} else if v == "force-remote-dns" {
					p.Warnings = appendUnique(p.Warnings, "force-remote-dns 使用当前代理内核的 DNS 策略")
				} else if v != "" {
					return p, fail("不支持的规则选项 " + v)
				}
			}
			switch kind {
			case "NATIVE":
				data, err := base64.RawURLEncoding.DecodeString(r.Value)
				var condition map[string]any
				if err != nil || json.Unmarshal(data, &condition) != nil || len(condition) == 0 {
					return p, fail("组合规则格式无效")
				}
				if err := validateSetRule(condition); err != nil {
					return p, fail(err.Error())
				}
				r.SetRules = []map[string]any{condition}
			case "RULE-SET", "DOMAIN-SET", "IP-SET", "MRS-DOMAIN", "MRS-IP", "SRS-SET":
				if !SetReference(r.Value) {
					return p, fail("规则集需要URL或绝对文件路径")
				}
				if len(r.SetHeaders) > 0 && (filepath.IsAbs(r.Value) || strings.HasPrefix(r.Value, "lanboat://")) {
					return p, fail("规则集请求头只能用于远程规则集")
				}
			case "DOMAIN-REGEX":
				if _, err := regexp.Compile(r.Value); err != nil {
					return p, fail("域名正则无效")
				}
			case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD":
				if r.Value == "" || strings.ContainsAny(r.Value, " \t\r\n") {
					return p, fail("域名规则无效")
				}
			case "IP-CIDR", "IP-CIDR6":
				prefix, err := netip.ParsePrefix(r.Value)
				if err != nil {
					return p, fail("IP 网段格式无效")
				}
				r.Value = prefix.Masked().String()
			case "GEOIP":
				code := strings.ToLower(r.Value)
				if code == "" || !ruledata.ValidRegion(code) {
					return p, fail("国家或地区代码无效")
				}
				r.Value = strings.ToUpper(code)
			case "IP-ASN":
				asn, e := strconv.ParseUint(r.Value, 10, 32)
				if e != nil || asn == 0 {
					return p, fail("ASN无效")
				}
				r.Value = strconv.FormatUint(asn, 10)
			case "USER-AGENT":
				p.Warnings = appendUnique(p.Warnings, "USER-AGENT 仅匹配本地 HTTP 代理可见的请求头；HTTPS 隧道不解密")
			case "DST-PORT", "DEST-PORT":
				port, err := strconv.Atoi(r.Value)
				if err != nil || port < 1 || port > 65535 {
					return p, fail("目标端口无效")
				}
				r.Type = "DST-PORT"
			case "NETWORK", "PROTOCOL":
				r.Value = strings.ToLower(r.Value)
				if r.Value != "tcp" && r.Value != "udp" {
					return p, fail("网络类型需要 TCP 或 UDP")
				}
				r.Type = "NETWORK"
			default:
				return p, fail("暂不支持规则类型 " + kind)
			}
			p.Rules = append(p.Rules, r)
			if len(p.Rules) > 20000 {
				return p, fail("规则不能超过 20000 条")
			}
		case "host":
			host, addresses, ok := strings.Cut(text, "=")
			if !ok {
				return p, fail("Host 格式无效")
			}
			host = strings.TrimSpace(host)
			// A few old public profiles spell this as IP=host.
			if net.ParseIP(host) != nil {
				host, addresses = strings.TrimSpace(addresses), host
			}
			if host == "" || strings.ContainsAny(host, "/ \t\r\n") {
				return p, fail("Host 域名无效")
			}
			for _, ip := range strings.Split(addresses, ",") {
				ip = strings.TrimSpace(ip)
				if net.ParseIP(ip) == nil {
					return p, fail("Host 记录需要 IP 地址")
				}
				p.Hosts[host] = append(p.Hosts[host], ip)
			}
		case "url rewrite", "url-rewrite":
			fields := strings.Fields(text)
			if len(fields) != 3 {
				return p, fail("URL Rewrite 格式无效")
			}
			if _, err := regexp.Compile(fields[0]); err != nil {
				return p, fail("URL Rewrite 正则无效")
			}
			status, err := strconv.Atoi(fields[2])
			if err != nil || status < 300 || status > 399 {
				return p, fail("仅支持 3xx URL 重定向")
			}
			p.Rewrites = append(p.Rewrites, Rewrite{fields[0], fields[1], status})
		case "":
			return p, fail("需要 [General] 或 [Rule] 配置段")
		default:
			p.Warnings = appendUnique(p.Warnings, "["+section+"] 已保留，尚未执行")
		}
	}
	if err := scanner.Err(); err != nil {
		return p, err
	}
	if len(p.Rewrites) > 0 {
		p.Warnings = appendUnique(p.Warnings, "URL Rewrite 应用于本地 HTTP 明文代理；HTTPS 隧道不解密")
	}
	return p, nil
}

func validateSetHeaders(headers map[string][]string) error {
	if len(headers) > 64 {
		return errors.New("规则集请求头过多")
	}
	total := 0
	for key, values := range headers {
		if key == "" || http.CanonicalHeaderKey(key) == "" || strings.ContainsAny(key, " \t\r\n\x00") {
			return errors.New("规则集请求头名称无效")
		}
		for _, r := range key {
			if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
				return errors.New("规则集请求头名称无效")
			}
		}
		if len(values) == 0 {
			return errors.New("规则集请求头值不能为空")
		}
		total += len(key)
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n\x00") {
				return errors.New("规则集请求头值无效")
			}
			total += len(value)
		}
	}
	if total > 12*1024 {
		return errors.New("规则集请求头过长")
	}
	return nil
}
func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
func policy(value string) (string, bool) {
	if v, ok := map[string]string{"DIRECT": "direct", "PROXY": "proxy", "REJECT": "reject", "REJECT-DROP": "reject", "REJECT-NO-DROP": "reject"}[strings.ToUpper(value)]; ok {
		return v, true
	}
	return value, ValidPolicy(value)
}
func ValidPolicy(value string) bool {
	return value != "" && len(value) <= 200 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, ",=\r\n\x00[]")
}

var prefixOnce sync.Once
var countries, asns []string

func Prefixes(kind, value string) []string {
	prefixOnce.Do(func() {
		var data struct {
			Rules []struct {
				IPCIDR []string `json:"ip_cidr"`
			} `json:"rules"`
		}
		_ = json.Unmarshal(countryData, &data)
		if len(data.Rules) > 0 {
			countries = data.Rules[0].IPCIDR
		}
		_ = json.Unmarshal(asnData, &asns)
	})
	if kind == "GEOIP" && value == "CN" {
		return countries
	}
	if kind == "IP-ASN" && value == "132203" {
		return asns
	}
	return nil
}

func (p Parsed) Match(host string, ip netip.Addr) (string, string) {
	return p.MatchRequest(Request{Host: host, IP: ip})
}
func (p Parsed) MatchRequest(req Request) (string, string) {
	req.Host = strings.TrimSuffix(strings.ToLower(req.Host), ".")
	for _, r := range p.Rules {
		if MatchRule(r, req) {
			return r.Action, fmt.Sprintf("%s,%s（第 %d 行）", r.Type, r.Value, r.Line)
		}
	}
	return p.Final, "FINAL"
}

// RequestPolicy returns a User-Agent override only before another visible rule matches.
func (p Parsed) RequestPolicy(host, agent string) (string, bool) {
	return p.HTTPPolicy(host, 80, agent)
}
func (p Parsed) HTTPPolicy(host string, port int, agent string) (string, bool) {
	return p.HTTPPolicyWithMatcher(host, port, agent, nil)
}
func (p Parsed) HTTPPolicyWithMatcher(host string, port int, agent string, match func(Rule, Request) (bool, error)) (string, bool) {
	found := false
	for _, r := range p.Rules {
		if r.Type == "USER-AGENT" && matchUserAgent(r.Value, agent) {
			found = true
			break
		}
	}
	if !found {
		return "", false
	}
	ip, _ := netip.ParseAddr(host)
	for _, r := range p.Rules {
		if r.Type == "USER-AGENT" {
			if matchUserAgent(r.Value, agent) {
				return r.Action, true
			}
			continue
		}

		if r.Type == "DST-PORT" && r.Value == strconv.Itoa(port) || r.Type == "NETWORK" && r.Value == "tcp" {
			return "", false
		}
		if r.SetPath != "" {
			if !ip.IsValid() && !r.NoResolve && (r.Type == "GEOIP" || r.Type == "IP-ASN" || strings.HasPrefix(r.Value, "lanboat://geoip/") || strings.HasPrefix(r.Value, "lanboat://asn/")) {
				return "", false
			}
			if match == nil {
				return "", false
			}
			hit, err := match(r, Request{Host: host, IP: ip, Port: port, Network: "tcp", Agent: agent})
			if err != nil || hit {
				return "", false
			}
			continue
		}
		// Do not override a preceding IP rule whose DNS result is known only to the core.
		if !ip.IsValid() && !r.NoResolve && (r.Type == "IP-CIDR" || r.Type == "IP-CIDR6" || r.Type == "GEOIP" || r.Type == "IP-ASN" || r.Type == "IP-SET" || r.Type == "MRS-IP" || (r.Type == "RULE-SET" || r.Type == "DOMAIN-SET" || r.Type == "NATIVE" || r.Type == "SRS-SET") && RuleSetNeedsIP(r.SetRules)) {
			return "", false
		}
		one := Parsed{Rules: []Rule{r}, Final: ""}
		if action, _ := one.MatchRequest(Request{Host: host, IP: ip, Port: port, Network: "tcp", Agent: agent}); action != "" {
			return "", false
		}
	}
	return "", false
}
