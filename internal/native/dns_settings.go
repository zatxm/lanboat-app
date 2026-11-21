package native

import (
	"net/netip"
	"strconv"
	"strings"

	"github.com/zatxm/fego/ui"
	"github.com/zatxm/lanboat-app/internal/profile"
)

const (
	dnsBootstrapIP  = "1.1.1.1"
	dnsBootstrapURL = "udp://" + dnsBootstrapIP
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

	bootstrapMode := s.Bootstrap
	if bootstrapMode != "system" && bootstrapMode != "local" {
		if _, err := netip.ParseAddr(s.Bootstrap); err == nil {
			bootstrapMode = "ip"
		} else {
			bootstrapMode = "protocol"
			if !strings.Contains(s.Bootstrap, "://") {
				u.field("dns.bootstrap").SetText("udp://" + s.Bootstrap)
			}
		}
	}
	u.field("dns.bootstrapMode").SetText(bootstrapMode)

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
	bootstrap := text("bootstrap")
	switch text("bootstrapMode") {
	case "ip":
		if bootstrap == "" {
			bootstrap = dnsBootstrapIP
		}
		if ip, err := netip.ParseAddr(bootstrap); err != nil || ip.IsUnspecified() {
			u.err = "引导解析请输入IPv4或IPv6地址"
			u.notify()
			return
		}
	case "protocol":
		if bootstrap == "" {
			bootstrap = dnsBootstrapURL
		}
		if !strings.Contains(bootstrap, "://") {
			u.err = "引导解析请输入带协议的IP地址"
			u.notify()
			return
		}
	}
	s := profile.DNSSettings{Persist: yes("persist"), ProxyNode: text("proxyNode"), Bootstrap: bootstrap, Direct: text("direct"), Proxy: text("proxy"), Policy: text("policy"), Final: text("final"), ProxyRoute: text("proxyRoute"), FollowRules: yes("followRules"), Strategy: text("strategy"), Timeout: number("timeout"), Cache: yes("cache"), Capacity: number("capacity"), Optimistic: yes("optimistic"), StaleTTL: number("staleTTL"), FakeIP: yes("fakeIP"), FakeIPv4: text("fakeIPv4"), FakeIPv6: text("fakeIPv6"), FakeIPFilter: text("filter")}
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
	return w
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
func (v *nativeView) dnsToolbar() *widget {
	u := v.ui
	return v.box("", true, 4, false,
		v.button("泄漏检测", "link icon-globe home-action tight-icon flush-left", false, false, u.diagnoseDNS),
		v.button("查询诊断", "link icon-test home-action tight-icon", false, false, func() { u.result = ""; u.openDetail("dns-query") }),
		v.button("还原默认", "link icon-refresh home-action tight-icon flush-left", false, false, u.resetDNSDefaults))
}

func (v *nativeView) dnsFields(box *widget) {
	u := v.ui
	labelWidth := float32(104)
	if u.state.Config.Settings.Language == "en" {
		labelWidth = 130
	}
	fieldRow := func(title string, control *widget, trailing ...*widget) *widget {
		label := v.box("", true, 0, false, v.label(title, "field-label"))
		label.pointer.Width(labelWidth).Justify(ui.End)
		row := v.box("", true, 12, false, label, control)
		v.pack(row, trailing...)
		if control.kind == "text" {
			row.pointer.AlignItems(ui.Start)
			label.pointer.Padding(10, 0, 0, 0)
		}
		return row
	}
	textField := func(key, placeholder string) *widget {
		input := v.input("dns."+key, placeholder, false, false, false)
		input.setClasses("dns-control")
		return input
	}
	fullField := func(key, placeholder string, multiline bool) *widget {
		input := v.input("dns."+key, placeholder, multiline, false, false)
		input.setExpand(true)
		return input
	}
	switchField := func(title, key string) *widget {
		return fieldRow(title, v.dnsSwitch(title, key))
	}
	selectField := func(title, key string, options []option, rebuild bool) *widget {
		choice := v.selectWidget(u.field("dns."+key).Text(), options, false, func(value string) {
			u.field("dns." + key).SetText(value)
			if rebuild {
				u.render()
			}
		})
		choice.setClasses("dns-control")
		return fieldRow(title, choice)
	}
	layer := func(title, description string, children ...*widget) {
		card := v.box("panel", false, 16, false, v.label(title, "dialog-title"))
		if description != "" {
			v.pack(card, v.label(description, "help"))
		}
		v.pack(card, children...)
		v.pack(box, card)
	}
	bootstrapMode := u.field("dns.bootstrapMode").Text()
	bootstrap := v.selectWidget(bootstrapMode, []option{{"system", "system"}, {"local", "local"}, {"ip", "IP（UDP 53）"}, {"protocol", "带协议IP"}}, false, func(mode string) {
		if mode == bootstrapMode {
			return
		}
		u.field("dns.bootstrapMode").SetText(mode)
		if mode == "ip" || mode == "protocol" {
			u.field("dns.bootstrap").SetText("")
		} else {
			u.field("dns.bootstrap").SetText(mode)
		}
		u.render()
	})
	bootstrap.pointer.Width(148)
	bootstrapRow := v.box("", true, 12, false, bootstrap)
	if bootstrapMode == "ip" || bootstrapMode == "protocol" {
		example := dnsBootstrapIP
		if bootstrapMode == "protocol" {
			example = dnsBootstrapURL
		}
		v.pack(bootstrapRow, fullField("bootstrap", example, false))
	}
	layer("引导解析", "", bootstrapRow, v.label(
		"解析DNS及节点服务器域名，避免循环依赖。\n带协议IP还可以填写如https://1.1.1.1/dns-query或udp://[2606:4700:4700::1111]。",
		"help"))

	addPolicy := v.button("添加DNS策略", "link icon-only icon-add", false, false, func() {
		u.field("dnsRuleType").SetText("DOMAIN-SUFFIX")
		u.field("dnsRuleValue").SetText("")
		u.field("dnsRuleTarget").SetText("direct")
		u.popup = &popup{kind: "dns-rule", title: "添加DNS策略"}
		u.notify()
	})
	policy := v.input("dns.policy", "DOMAIN-SUFFIX,example.com,direct\nDOMAIN,blocked.example,reject", true, false, false)
	help := v.box("", true, 8, false,
		v.label("每行：类型,匹配内容,direct / proxy / bootstrap / reject，或者点击", "help"), addPolicy)
	layer("解析策略", "", policy, v.box("", false, 2, false, help,
		v.label("从上到下匹配，优先于路由文件中的域名规则。", "help")))

	resolvers := v.box("", false, 12, false)
	resolverRow := func(title string, control *widget) *widget {
		control.pointer.Width(320)
		slot := v.box("", false, 0, true, control)
		slot.pointer.AlignItems(ui.Start)
		return fieldRow(title, slot)
	}
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
		v.pack(resolvers, resolverRow(title, choice))
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
			v.pack(resolvers, resolverRow("协议", typ))
			if u.field("dns."+key+"Type").Text() == "local" {
				return
			}
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
			v.pack(resolvers, resolverRow("服务器", input), fieldRow("", v.label("Bootstrap：自动使用引导DNS", "help")))
		}
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
	route = append(route,
		switchField("DNS跟随域名分流", "followRules"),
		v.label("根据路由中的域名及规则集自动选择直连或代理DNS。", "help"),
		selectField("地址策略", "strategy", []option{{"", "自动（推荐）"}, {"prefer_ipv4", "优先IPv4"}, {"prefer_ipv6", "优先IPv6"}, {"ipv4_only", "仅IPv4"}, {"ipv6_only", "仅IPv6"}}, false),
		fieldRow("查询超时", textField("timeout", "5"), v.label("秒，1–30", "help")))
	layer("解析路由", "指定解析器及DNS请求出口。", route...)
	grace := []option{{"300", "5分钟"}, {"1800", "30分钟（推荐）"}, {"3600", "1小时"}, {"86400", "1天"}}
	currentGrace := u.field("dns.staleTTL").Text()
	if currentGrace != "300" && currentGrace != "1800" && currentGrace != "3600" && currentGrace != "86400" {
		grace = append(grace, option{currentGrace, currentGrace + "秒"})
	}
	layer("缓存与虚拟地址", "缓存解析结果；FakeIP用于TUN的A/AAAA查询。",
		switchField("DNS缓存", "cache"),
		fieldRow("缓存容量", textField("capacity", "4096"), v.label("1024–65536", "help")),
		switchField("持久化DNS缓存", "persist"),
		switchField("乐观缓存", "optimistic"),
		selectField("过期容忍", "staleTTL", grace, false),
		switchField("FakeIP", "fakeIP"),
		fieldRow("FakeIP的IPv4范围", fullField("fakeIPv4", "198.18.0.0/15", false)),
		fieldRow("FakeIP的IPv6范围", fullField("fakeIPv6", "2001:2::/48", false)),
		fieldRow("FakeIP排除域名", fullField("filter", "*.lan\n*.local\nlocalhost", true)))
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
