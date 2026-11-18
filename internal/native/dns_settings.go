package native

import (
	"strconv"

	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func (u *UI) fillDNS(s profile.DNSSettings) {
	for k, v := range map[string]string{"bootstrap": s.Bootstrap, "direct": s.Direct, "proxy": s.Proxy, "policy": s.Policy, "final": s.Final, "proxyRoute": s.ProxyRoute, "strategy": s.Strategy, "fakeIPv4": s.FakeIPv4, "fakeIPv6": s.FakeIPv6, "filter": s.FakeIPFilter, "proxyNode": s.ProxyNode} {
		u.field("dns." + k).SetText(v)
	}
	for k, v := range map[string]bool{"followRules": s.FollowRules, "cache": s.Cache, "optimistic": s.Optimistic, "fakeIP": s.FakeIP, "persist": s.Persist} {
		u.field("dns." + k).SetText(strconv.FormatBool(v))
	}
	for k, v := range map[string]int{"timeout": s.Timeout, "capacity": s.Capacity, "staleTTL": s.StaleTTL} {
		u.field("dns." + k).SetText(strconv.Itoa(v))
	}

	for _, key := range []string{"direct", "proxy"} {
		u.prepareDNSResolver(key)
	}
}
func (u *UI) openDNS() {
	c := u.state.Config
	if c.Settings.DNS == nil {
		r, err := profile.EffectiveRouting(c)
		if err != nil {
			u.err = err.Error()
			u.notify()
			return
		}
		c.Routing = &r
	}
	u.fillDNS(profile.EffectiveDNS(c, c.Settings.Mode == "tun"))
	u.openEditor("dns-settings")
}
func (u *UI) resetDNSDefaults() {
	u.fillDNS(profile.DefaultDNS())
	u.err = ""
	u.notice = ""
	u.render()
}
func (u *UI) saveDNS() {
	text := func(k string) string { return strings.TrimSpace(u.field("dns." + k).Text()) }
	yes := func(k string) bool { return text(k) == "true" }
	number := func(k string) int { n, _ := strconv.Atoi(text(k)); return n }
	s := profile.DNSSettings{Persist: yes("persist"), ProxyNode: text("proxyNode"), Bootstrap: text("bootstrap"), Direct: text("direct"), Proxy: text("proxy"), Policy: text("policy"), Final: text("final"), ProxyRoute: text("proxyRoute"), FollowRules: yes("followRules"), Strategy: text("strategy"), Timeout: number("timeout"), Cache: yes("cache"), Capacity: number("capacity"), Optimistic: yes("optimistic"), StaleTTL: number("staleTTL"), FakeIP: yes("fakeIP"), FakeIPv4: text("fakeIPv4"), FakeIPv6: text("fakeIPv6"), FakeIPFilter: text("filter")}
	if err := s.Validate(); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	settings := u.state.Config.Settings
	if settings.DNS != nil {
		s.Advanced = settings.DNS.Advanced
	}
	settings.DNS = &s
	u.savingSettings = true
	u.submit(func() error { return u.backend.SaveSettings(settings) }, func() { u.state.Config.Settings = settings; u.back(); u.notice = "DNS设置已保存" })
}
func (v *nativeView) dnsSwitch(title, key string) *widget {
	u := v.ui
	w := v.button(title, "switch", false, false, func() {
		u.field("dns." + key).SetText(strconv.FormatBool(u.field("dns."+key).Text() != "true"))
	})
	w.update = func() {
		c := "button switch"
		if u.field("dns."+key).Text() == "true" {
			c += " switch-on"
		}
		w.setClasses(c)
	}
	return v.box("", true, 10, false, v.label(title, "node-name"), w)
}
func (u *UI) prepareDNSResolver(key string) {
	address := u.field("dns." + key).Text()
	kind, server := "local", ""
	if address == "auto" {
		kind = "auto"
	} else if a, err := profile.ParseDNSAddress(address); err == nil {
		kind = a.Type
		server = strings.TrimPrefix(address, strings.Split(address, "://")[0]+"://")
		if a.Type == "local" {
			server = ""
		}
	}
	u.field("dns." + key + "Type").SetText(kind)
	u.field("dns." + key + "Server").SetText(server)
}
func (v *nativeView) dnsFields(box *widget) {
	u := v.ui
	selectField := func(title, key string, options []option, rebuild bool) *widget {
		choice := v.selectWidget(u.field("dns."+key).Text(), options, false, func(value string) {
			u.field("dns." + key).SetText(value)
			if rebuild {
				u.render()
			}
		})
		choice.setClasses("protocol-choice")
		return v.box("", true, 16, false, v.label(title, "field-label dns-select-key"), choice)
	}
	layer := func(title, description string, children ...*widget) {
		card := v.box("panel", false, 16, false, v.label(title, "dialog-title"), v.label(description, "help"))
		v.pack(card, children...)
		v.pack(box, card)
	}
	v.pack(box, v.box("", true, 4, false, v.button("泄漏检测", "link icon-globe home-action tight-icon flush-left", false, false, u.diagnoseDNS), v.button("查询诊断", "link icon-test home-action tight-icon", false, false, func() { u.result = ""; u.openDetail("dns-query") }), v.button("还原默认", "link icon-refresh home-action tight-icon flush-left", false, false, u.resetDNSDefaults), v.spacer()))
	layer("引导解析", "解析DNS及节点服务器域名，避免循环依赖。", v.field("dns.bootstrap", "引导DNS", "system或udp://1.1.1.1", false))
	policy := v.box("field", false, 7, false, v.label("域名策略", "field-label"), v.input("dns.policy", "DOMAIN-SUFFIX,example.com,direct\nDOMAIN,blocked.example,reject", true, false, false))
	help := "每行：类型,匹配内容,direct / proxy / bootstrap / reject"
	layer("解析策略", "从上到下匹配，优先于路由文件中的域名规则。", v.box("", true, 8, false, v.label("解析规则", "field-label"), v.spacer(), v.button("添加", "link icon-add", false, false, func() {
		u.field("dnsRuleType").SetText("DOMAIN-SUFFIX")
		u.field("dnsRuleValue").SetText("")
		u.field("dnsRuleTarget").SetText("direct")
		u.popup = &popup{kind: "dns-rule", title: "添加DNS策略"}
		u.notify()
	})), policy, v.label(help, "help"))
	resolvers := v.box("", false, 18, false)
	resolver := func(key, title string) {
		current := u.field("dns." + key).Text()
		normal := []option{{"system", "系统DNS"}, {"custom", "自定义"}}
		if key == "proxy" {
			normal = []option{{"auto", "自动（推荐）"}, {"custom", "自定义"}}
		}

		mode := current
		if current != "system" && current != "auto" {
			mode = "custom"
		}
		if key == "proxy" && current == "system" {
			normal = append(normal, option{"system", "系统DNS"})
		}
		choice := v.selectWidget(mode, normal, false, func(value string) {
			if value == "custom" {
				if key == "proxy" {
					u.field("dns." + key).SetText("https://1.1.1.1/dns-query")
				} else {
					u.field("dns." + key).SetText("udp://1.1.1.1")
				}
			} else {
				u.field("dns." + key).SetText(value)
			}
			u.prepareDNSResolver(key)
			u.render()
		})
		choice.setClasses("protocol-choice")
		card := v.box("panel", false, 12, false, v.box("", true, 16, false, v.label(title, "field-label dns-select-key"), choice))
		if mode == "custom" {
			protocols := []option{{"local", "System"}, {"udp", "UDP"}, {"tcp", "TCP"}, {"tls", "DoT"}, {"https", "DoH"}, {"quic", "DoQ"}, {"h3", "HTTP/3"}}
			typ := v.selectWidget(u.field("dns."+key+"Type").Text(), protocols, false, func(value string) {
				server := u.field("dns." + key + "Server").Text()
				if server == "" {
					server = "1.1.1.1"
				}
				if value != "https" && value != "h3" {
					server = strings.SplitN(server, "/", 2)[0]
				} else if !strings.Contains(server, "/") {
					server += "/dns-query"
				}
				address := value + "://" + server
				if value == "local" {
					address = "system"
					if key == "proxy" {
						u.field("dns.proxyRoute").SetText("direct")
					}
				}
				u.field("dns." + key).SetText(address)
				u.prepareDNSResolver(key)
				u.render()
			})
			typ.setClasses("protocol-choice")
			v.pack(card, v.box("", true, 16, false, v.label("协议", "field-label dns-select-key"), typ))
			input := v.input("dns."+key+"Server", "域名或IP，可指定端口和路径", false, false, false)
			input.event = func(event, value string) {
				u.field("dns." + key + "Server").SetText(value)
				kind := u.field("dns." + key + "Type").Text()
				if kind == "local" {
					u.field("dns." + key).SetText("system")
				} else {
					u.field("dns." + key).SetText(kind + "://" + value)
				}
			}
			if u.field("dns."+key+"Type").Text() != "local" {
				v.pack(card, v.box("field", false, 7, false, v.label("服务器", "field-label"), input), v.label("Bootstrap：自动使用引导DNS", "help"))
			}
		}
		v.pack(resolvers, card)
	}
	resolver("direct", "直连DNS")
	resolver("proxy", "代理DNS")
	layer("解析器", "自动：有代理时使用远程加密DNS，无代理时使用系统DNS。", resolvers)
	finals := []option{{"auto", "自动（推荐）"}, {"proxy", "代理DNS"}, {"direct", "直连DNS"}}
	finals = append(finals, option{"bootstrap", "引导DNS"})
	route := []*widget{selectField("默认解析器", "final", finals, false), selectField("代理DNS出口", "proxyRoute", []option{{"proxy", "跟随代理（推荐）"}, {"direct", "直连"}, {"node", "指定出站"}}, true)}
	if u.field("dns.proxyRoute").Text() == "node" {
		var nodes []option
		for _, n := range u.state.Config.Nodes {
			nodes = append(nodes, option{n.ID, n.Name})
		}
		route = append(route, selectField("出站节点", "proxyNode", nodes, false))
	}
	route = append(route, v.dnsSwitch("DNS跟随域名分流", "followRules"), v.label("根据路由中的域名及规则集自动选择直连或代理DNS。", "help"), selectField("地址策略", "strategy", []option{{"", "自动（推荐）"}, {"prefer_ipv4", "优先IPv4"}, {"prefer_ipv6", "优先IPv6"}, {"ipv4_only", "仅IPv4"}, {"ipv6_only", "仅IPv6"}}, false), v.field("dns.timeout", "查询超时（秒，1–30）", "5", false))
	layer("解析路由", "指定解析器及DNS请求出口。", route...)
	grace := []option{{"300", "5分钟"}, {"1800", "30分钟（推荐）"}, {"3600", "1小时"}, {"86400", "1天"}}
	currentGrace := u.field("dns.staleTTL").Text()
	if currentGrace != "300" && currentGrace != "1800" && currentGrace != "3600" && currentGrace != "86400" {
		grace = append(grace, option{currentGrace, currentGrace + "秒"})
	}
	layer("缓存与虚拟地址", "缓存解析结果；FakeIP用于TUN的A/AAAA查询。", v.dnsSwitch("DNS缓存", "cache"), v.field("dns.capacity", "缓存容量（1024–65536）", "4096", false), v.dnsSwitch("持久化DNS缓存", "persist"), v.dnsSwitch("乐观缓存", "optimistic"), selectField("过期容忍", "staleTTL", grace, false), v.dnsSwitch("FakeIP", "fakeIP"), v.field("dns.fakeIPv4", "FakeIP的IPv4范围", "198.18.0.0/15", false), v.field("dns.fakeIPv6", "FakeIP的IPv6范围", "2001:2::/48", false), v.box("field", false, 7, false, v.label("FakeIP排除域名（每行一项）", "field-label"), v.input("dns.filter", "*.lan\n*.local\nlocalhost", true, false, false)))
}
func (u *UI) addDNSPolicy() {
	kind, value, target := u.field("dnsRuleType").Text(), strings.TrimSpace(u.field("dnsRuleValue").Text()), u.field("dnsRuleTarget").Text()
	line := kind + "," + value + "," + target
	if _, err := profile.ParseDNSPolicies(line); err != nil {
		u.err = err.Error()
		u.notify()
		return
	}
	old := strings.TrimSpace(u.field("dns.policy").Text())
	if old != "" {
		old += "\n"
	}
	u.field("dns.policy").SetText(old + line)
	u.closePopup()
	u.render()
}

func (u *UI) diagnoseDNS() {
	var report string
	u.submit(func() error { var err error; report, err = u.backend.DNSDiagnostics(); return err }, func() {
		u.field("dns.report").SetText(report)
		u.popup = &popup{kind: "dns-report", title: "DNS路径检测"}
	})
}
