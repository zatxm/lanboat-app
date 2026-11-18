package profile

import (
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type DNSSettings struct {
	Advanced     bool   `json:"advanced,omitempty"`
	Persist      bool   `json:"persist,omitempty"`
	ProxyNode    string `json:"proxyNode,omitempty"`
	Bootstrap    string `json:"bootstrap"`
	Direct       string `json:"direct"`
	Proxy        string `json:"proxy"`
	Policy       string `json:"policy,omitempty"`
	Final        string `json:"final"`
	ProxyRoute   string `json:"proxyRoute"`
	FollowRules  bool   `json:"followRules"`
	Strategy     string `json:"strategy"`
	Timeout      int    `json:"timeout"`
	Cache        bool   `json:"cache"`
	Capacity     int    `json:"capacity"`
	Optimistic   bool   `json:"optimistic"`
	StaleTTL     int    `json:"staleTTL"`
	FakeIP       bool   `json:"fakeIP"`
	FakeIPv4     string `json:"fakeIPv4"`
	FakeIPv6     string `json:"fakeIPv6"`
	FakeIPFilter string `json:"fakeIPFilter,omitempty"`
}

type DNSAddress struct {
	Type, Server, Path string
	Port               int
}
type DNSPolicy struct{ Type, Value, Resolver string }

func DefaultDNS() DNSSettings {
	return DNSSettings{Bootstrap: "system", Direct: "system", Proxy: "auto", Final: "auto", ProxyRoute: "proxy", FollowRules: true, Timeout: 5, Cache: true, Capacity: 4096, StaleTTL: 1800, FakeIPv4: "198.18.0.0/15", FakeIPv6: "2001:2::/48", FakeIPFilter: "*.lan\n*.local\nlocalhost"}
}
func ParseDNSAddress(text string) (DNSAddress, error) {
	if text == "system" || text == "local" {
		return DNSAddress{Type: "local"}, nil
	}
	if addr, err := netip.ParseAddr(text); err == nil && !addr.IsUnspecified() {
		return DNSAddress{Type: "udp", Server: addr.String(), Port: 53}, nil
	}
	if !strings.Contains(text, "://") {
		text = "udp://" + text
	}
	u, err := url.Parse(text)
	if err != nil || u.User != nil || u.Hostname() == "" || !ValidHost(u.Hostname()) || u.RawQuery != "" || u.Fragment != "" {
		return DNSAddress{}, fmt.Errorf("DNS 地址无效")
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && ip.IsUnspecified() {
		return DNSAddress{}, fmt.Errorf("DNS 地址无效")
	}
	ports := map[string]int{"udp": 53, "tcp": 53, "tls": 853, "quic": 853, "https": 443, "h3": 443}
	port, ok := ports[u.Scheme]
	if !ok {
		return DNSAddress{}, fmt.Errorf("DNS 支持 system、UDP、TCP、TLS、QUIC、HTTPS 或 H3 地址")
	}
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return DNSAddress{}, fmt.Errorf("DNS 端口必须为 1–65535")
		}
	}
	if u.Path != "" && u.Path != "/" && u.Scheme != "https" && u.Scheme != "h3" {
		return DNSAddress{}, fmt.Errorf("只有 HTTPS/H3 DNS 地址可以包含路径")
	}
	path := u.EscapedPath()
	if path == "" || path == "/" {
		path = "/dns-query"
	}
	kind := u.Scheme
	return DNSAddress{Type: kind, Server: u.Hostname(), Port: port, Path: path}, nil
}
func ParseDNSPolicies(text string) ([]DNSPolicy, error) {
	if len(text) > 16384 {
		return nil, fmt.Errorf("DNS 策略不能超过 16 KiB")
	}
	var rules []DNSPolicy
	types := map[string]string{"DOMAIN": "domain", "DOMAIN-SUFFIX": "domain_suffix", "DOMAIN-KEYWORD": "domain_keyword", "DOMAIN-REGEX": "domain_regex", "QUERY-TYPE": "query_type"}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		first, last := strings.IndexByte(line, ','), strings.LastIndexByte(line, ',')
		if first < 0 || first == last {
			return nil, fmt.Errorf("DNS 策略第 %d 行需为：类型,匹配内容,解析器", i+1)
		}
		parts := []string{line[:first], line[first+1 : last], line[last+1:]}
		for j := range parts {
			parts[j] = strings.TrimSpace(parts[j])
		}
		kind, ok := types[strings.ToUpper(parts[0])]
		if !ok || parts[1] == "" {
			return nil, fmt.Errorf("DNS 策略第 %d 行类型或匹配内容无效", i+1)
		}
		target := strings.ToLower(parts[2])
		if target != "direct" && target != "proxy" && target != "bootstrap" && target != "reject" {
			return nil, fmt.Errorf("DNS 策略第 %d 行解析器无效", i+1)
		}
		switch kind {
		case "domain", "domain_suffix":
			if !ValidHost(strings.TrimPrefix(parts[1], ".")) {
				return nil, fmt.Errorf("DNS 策略第 %d 行域名无效", i+1)
			}
		case "domain_regex":
			if _, err := regexp.Compile(parts[1]); err != nil {
				return nil, fmt.Errorf("DNS 策略第 %d 行正则表达式无效", i+1)
			}
		case "query_type":
			switch strings.ToUpper(parts[1]) {
			case "A", "AAAA", "HTTPS", "SVCB", "TXT", "MX", "NS", "SRV", "PTR", "CNAME", "SOA", "ANY":
			default:
				return nil, fmt.Errorf("DNS 策略第 %d 行查询类型无效", i+1)
			}
			parts[1] = strings.ToUpper(parts[1])
		}
		rules = append(rules, DNSPolicy{kind, parts[1], target})
	}
	if len(rules) > 256 {
		return nil, fmt.Errorf("最多保存 256 条 DNS 策略")
	}
	return rules, nil
}
func (s DNSSettings) Validate() error {
	for label, text := range map[string]string{"Bootstrap": s.Bootstrap, "直连 DNS": s.Direct, "代理 DNS": s.Proxy} {
		if label == "代理 DNS" && text == "auto" {
			continue
		}
		if len(text) > 2048 {
			return fmt.Errorf("%s过长", label)
		}
		a, err := ParseDNSAddress(text)
		if err != nil {
			return fmt.Errorf("%s：%w", label, err)
		}
		if label == "Bootstrap" && a.Type != "local" {
			if _, err := netip.ParseAddr(a.Server); err != nil {
				return fmt.Errorf("Bootstrap 必须使用 IP 地址，避免循环解析")
			}
		}
	}
	if _, err := ParseDNSPolicies(s.Policy); err != nil {
		return err
	}
	if a, err := ParseDNSAddress(s.Proxy); err == nil && a.Type == "local" && s.ProxyRoute != "direct" {
		return fmt.Errorf("系统 DNS 需要使用直连出口")
	}
	if s.Final != "auto" && s.Final != "direct" && s.Final != "proxy" && s.Final != "bootstrap" {
		return fmt.Errorf("默认 DNS 解析器无效")
	}
	if s.ProxyRoute != "direct" && s.ProxyRoute != "proxy" && s.ProxyRoute != "node" {
		return fmt.Errorf("DNS 出口无效")
	}
	if s.ProxyRoute == "node" && (s.ProxyNode == "" || len(s.ProxyNode) > 128) {
		return fmt.Errorf("请选择DNS出站节点")
	}
	switch s.Strategy {
	case "prefer_ipv4", "prefer_ipv6", "ipv4_only", "ipv6_only", "":
	default:
		return fmt.Errorf("DNS 地址策略无效")
	}
	if s.Timeout < 1 || s.Timeout > 30 || s.Capacity < 1024 || s.Capacity > 65536 || s.StaleTTL < 1 || s.StaleTTL > 86400 {
		return fmt.Errorf("DNS超时为1–30秒，缓存容量为1024–65536，过期容忍为1–86400秒")
	}
	if len(s.FakeIPFilter) > 8192 {
		return fmt.Errorf("FakeIP 排除列表不能超过 8 KiB")
	}
	for _, line := range strings.Split(s.FakeIPFilter, "\n") {
		host := strings.TrimSpace(line)
		if host != "" && !ValidHost(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(host, "*."), "+."), ".")) {
			return fmt.Errorf("FakeIP 排除域名无效：%s", host)
		}
	}
	if s.FakeIP {
		for i, text := range []string{s.FakeIPv4, s.FakeIPv6} {
			p, err := netip.ParsePrefix(text)
			if err != nil || p.Addr().Is4() != (i == 0) || p.Bits() == 0 || p.Addr().IsMulticast() || p.Addr().IsLoopback() {
				return fmt.Errorf("FakeIP 地址范围无效")
			}
		}
	}
	return nil
}

// Explicit settings are retained; unsaved profiles use the same defaults as the UI.
func EffectiveDNS(c Config, tun bool) DNSSettings {
	if c.Settings.DNS != nil {
		return *c.Settings.DNS
	}
	return DefaultDNS()
}
