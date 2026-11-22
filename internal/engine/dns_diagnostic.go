package engine

import (
	"context"
	"fmt"
	"github.com/goccy/go-json"
	allinone "github.com/zatxm/lanboat-allinone"
	"net"
	"os/exec"
	"strings"

	"github.com/miekg/dns"
	"github.com/zatxm/lanboat-app/internal/dnscheck"
	"github.com/zatxm/lanboat-app/internal/dnsconfig"
	"github.com/zatxm/lanboat-app/internal/i18n"
	"github.com/zatxm/lanboat-app/internal/profile"
)

// Uses native DNS transports and the active proxy bridge, without changing the running configuration.
func (e *Engine) DNSDiagnostics(ctx context.Context, c profile.Config) (string, error) {
	e.mu.Lock()
	connected := e.runtime != nil
	path, dir := e.path, e.dir
	ports := append([]int(nil), e.dnsPorts...)
	servers := e.dnsServers
	language := c.Settings.Language
	if connected {
		c = e.dnsConfig
		c.Settings.Language = language
	}
	e.mu.Unlock()
	if path == "" {
		return "", fmt.Errorf("未找到DNS内核")
	}
	s := profile.EffectiveDNS(c, c.Settings.Mode == "tun")
	var lines []string
	if connected {
		lines = append(lines, "检测当前连接配置；未保存的页面设置不参与检测。")
	} else {
		lines = append(lines, "尚未连接：仅检测引导和直连DNS，代理及TUN路径未验证。")
	}
	if !connected {
		d := dnsconfig.Build(c, false, "", nil)
		config := map[string]any{"log": map[string]any{"disabled": true}, "dns": d, "inbounds": []any{}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}, map[string]any{"type": "socks", "tag": "proxy", "server": "127.0.0.1", "server_port": 9, "version": "5"}, map[string]any{"type": "socks", "tag": "dns-node", "server": "127.0.0.1", "server_port": 9, "version": "5"}}, "route": map[string]any{"default_domain_resolver": "bootstrap", "rules": []any{}}}
		var release func()
		var err error
		ports, release, err = configureDNSChecks(config)
		if err != nil {
			return "", err
		}
		defer release()
		raw, err := json.Marshal(config)
		if err != nil {
			return "", err
		}
		release()
		rt, err := allinone.New(ctx, raw)
		if err != nil {
			return "", err
		}
		defer rt.Close()
		if err = rt.Start(); err != nil {
			return "", err
		}

		servers = d["servers"].([]any)
	}
	if len(ports) != 3 || len(servers) < 3 {
		return "", fmt.Errorf("DNS检测服务不可用")
	}
	for i, label := range []string{"引导DNS", "直连DNS", "代理DNS"} {
		server := servers[i].(map[string]any)
		address := fmt.Sprint(server["server"])
		if server["type"] == "local" {
			address = i18n.Text(c.Settings.Language, "system（由系统解析器选择）")
		}
		route := "DIRECT"
		if i == 2 {
			route = s.ProxyRoute
			if route == "node" {
				route = i18n.Text(c.Settings.Language, "指定节点")
				for _, n := range c.Nodes {
					if n.ID == s.ProxyNode {
						route += " / " + n.Name
						break
					}
				}
			} else if route == "proxy" {
				for _, n := range c.Nodes {
					if n.ID == c.Selected {
						route = "PROXY / " + n.Name
						break
					}
				}
			}
			if c.Settings.Mode == "direct" {
				route = i18n.Text(c.Settings.Language, "DIRECT（直连模式）")
			}
		}
		lines = append(lines, fmt.Sprintf("\n%s\n%s · %s\n配置出口：%s", label, server["type"], address, route))
		if i == 2 && !connected {
			lines = append(lines, "未检测：请先连接")
			continue
		}
		probe := dnscheck.Query(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", ports[i]), "example.com", dns.TypeA)
		if probe.Error != "" {
			lines = append(lines, "查询失败："+probe.Error)
		} else {
			lines = append(lines, "查询成功："+strings.Join(probe.Answers, ", "))
		}
		if server["type"] != "local" && net.ParseIP(address) == nil {
			p := dnscheck.Query(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", ports[0]), address, dns.TypeA)
			if p.Error == "" {
				lines = append(lines, "服务器引导解析IP："+strings.Join(p.Answers, ", "))
			} else {
				lines = append(lines, "服务器IP未确认："+p.Error)
			}
		}
	}
	lines = append(lines, "\nTUN的DNS劫持与FakeIP")
	if !connected || c.Settings.Mode != "tun" {
		lines = append(lines, "未检测：TUN未开启")
	} else {
		binary, cleanup, probeErr := prepareDNSProbe(path, dir)
		if probeErr != nil {
			lines = append(lines, "未检测：缺少检测组件")
		} else {
			defer cleanup()
			raw, err := exec.CommandContext(ctx, binary, "--dns-probe").Output()
			var probes []dnscheck.Probe
			if err != nil || json.Unmarshal(raw, &probes) != nil || len(probes) != 4 {
				lines = append(lines, "检测失败：无法运行独立网络探针")
			} else {
				for i, p := range probes {
					if i < 2 {
						result := "未确认劫持"
						if dnscheck.InRange(p, "192.0.2.53/32") {
							result = "已确认劫持（本地标记响应）"
						}
						lines = append(lines, strings.ToUpper(p.Network)+"/53："+result)
					} else {
						prefix := s.FakeIPv4
						if i == 3 {
							prefix = s.FakeIPv6
						}
						result := "未启用"
						if s.FakeIP {
							result = "返回值未落入配置范围"
							if dnscheck.InRange(p, prefix) {
								result = "范围验证通过"
							}
							if p.Error != "" {
								result = "查询失败：" + p.Error
							}
						}
						lines = append(lines, "FakeIP "+p.Type+"："+result)
					}
				}
			}
		}
	}
	lines = append(lines, "\n检测边界：出口依据正在使用的原生配置；查询成功不等于已排除所有DNS泄漏。系统解析器可能转发至系统服务，其最终上游IP需结合网页检测或抓包确认。")
	for i, line := range lines {
		lines[i] = i18n.Text(c.Settings.Language, line)
	}
	var report strings.Builder
	column := 0
	for _, r := range strings.Join(lines, "\n") {
		if r == '\n' {
			column = 0
			report.WriteRune(r)
			continue
		}
		width := 1
		if r >= 0x2e80 {
			width = 2
		}
		if column+width > 80 {
			report.WriteByte('\n')
			column = 0
		}
		report.WriteRune(r)
		column += width
	}
	return report.String(), nil
}

// Private loopback listeners isolate resolver probes from normal DNS policy and cache.
func configureDNSChecks(config map[string]any) ([]int, func(), error) {
	var listeners []net.Listener
	release := func() {
		for _, l := range listeners {
			l.Close()
		}
	}
	var ports []int
	var rules []any
	var tags []string
	for _, tag := range []string{"bootstrap", "dns-direct", "dns-proxy"} {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			release()
			return nil, release, err
		}
		listeners = append(listeners, listener)
		ports = append(ports, listener.Addr().(*net.TCPAddr).Port)
		inbound := "check-" + tag
		tags = append(tags, inbound)
		config["inbounds"] = append(config["inbounds"].([]any), map[string]any{"type": "direct", "tag": inbound, "listen": "127.0.0.1", "listen_port": ports[len(ports)-1], "network": "tcp"})
		rules = append(rules, map[string]any{"inbound": []string{inbound}, "action": "route", "server": tag, "disable_cache": true})
	}
	d := config["dns"].(map[string]any)
	d["rules"] = append(rules, d["rules"].([]any)...)
	route := config["route"].(map[string]any)
	route["rules"] = append([]any{map[string]any{"inbound": tags, "action": "hijack-dns"}}, route["rules"].([]any)...)
	return ports, release, nil
}
