package engine

import (
	"fmt"
	"github.com/zatxm/lanboat-app/internal/dnsconfig"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/udpconfig"
	"net"
	"strings"
)

func BuildConfig(c profile.Config, control, secret string) map[string]any {
	var selected profile.Node
	for _, n := range c.Nodes {
		if n.ID == c.Selected {
			selected = n.Clone()
			selected.TCP = &c.Settings.TCP
			break
		}
	}
	if c.Settings.Mode == "direct" {
		selected = profile.Node{}
	}
	outbound := coreOutbound(selected)
	if tfo := selected.Option("tfo"); tfo != nil {
		outbound["tcp_fast_open"] = selected.TextOption("tfo") == "true"
	}
	if selected.TextOption("udp") == "false" && selected.Kind() == "ss" {
		outbound["network"] = "tcp"
	}
	outbounds := []any{map[string]any{"type": "direct", "tag": "direct"}}
	if selected.ID != "" {
		outbounds = append(outbounds, outbound)
	}
	if c.Settings.DNS != nil && c.Settings.DNS.ProxyRoute == "node" {
		for _, n := range c.Nodes {
			if n.ID == c.Settings.DNS.ProxyNode {
				n = n.Clone()
				n.TCP = &c.Settings.TCP
				out := coreOutbound(n)
				out["tag"] = "dns-node"
				outbounds = append(outbounds, out)
				break
			}
		}
	}
	outbounds = appendUpstream(outbounds, &c.Settings.Proxy.Upstream)
	rules := []any{}
	final := "proxy"
	if c.Settings.Mode == "direct" {
		final = "direct"
	}
	if c.Settings.Mode == "rule" {
		manual, _ := profile.CustomRouting(c.Rules)
		rules, _ = coreProfileRules(&manual)
		rules = append(rules, map[string]any{"ip_is_private": true, "action": "route", "outbound": "direct"})
	}
	config := map[string]any{
		// The core's own log stays off: the app log records only app events,
		// while proxy and DNS records come from the runtime's callbacks.
		"log":          map[string]any{"disabled": true},
		"inbounds":     []any{mixedInbound(c.Settings.Proxy, c.Settings.Port, "mixed-in")},
		"outbounds":    outbounds,
		"route":        map[string]any{"rules": rules, "final": final},
		"experimental": map[string]any{"clash_api": map[string]any{"external_controller": control, "secret": secret, "access_control_allow_origin": []string{}}},
	}
	config["inbounds"].([]any)[0].(map[string]any)["udp_timeout"] = fmt.Sprint(c.Settings.UDP.Seconds()) + "s"
	applyCoreRouting(config, c)
	var domains []string
	hosts := []string{selected.Server, selected.TextOption("plugin-opts.server")}
	if c.Settings.Proxy.Upstream.Enabled {
		hosts = append(hosts, c.Settings.Proxy.Upstream.Server)
	}
	for _, host := range hosts {
		if host != "" && net.ParseIP(host) == nil {
			domains = append(domains, host)
		}
	}
	config["dns"] = dnsconfig.Build(c, c.Settings.Mode == "tun", "", domains)
	config["route"].(map[string]any)["default_domain_resolver"] = "bootstrap"
	if selected.TextOption("udp") == "false" && c.Settings.Mode != "direct" && c.Settings.UDP.RelayEnabled() && !c.Settings.UDP.DirectOnly(selected.Kind()) {
		route := config["route"].(map[string]any)
		route["rules"] = append([]any{map[string]any{"network": "udp", "action": "reject"}}, route["rules"].([]any)...)
	}
	if c.Settings.Mode != "tun" {
		route := config["route"].(map[string]any)
		route["rules"] = append([]any{map[string]any{"port": []int{53}, "action": "hijack-dns"}}, route["rules"].([]any)...)
	}
	route := config["route"].(map[string]any)
	route["rules"] = append(udpconfig.Rules(c.Settings.UDP, c.Settings.UDP.DirectOnly(selected.Kind())), route["rules"].([]any)...)
	return config
}

func mixedInbound(p profile.ProxySettings, port int, tag string) map[string]any {
	in := map[string]any{"type": "mixed", "tag": tag, "listen": p.ListenHost(), "listen_port": port}
	if p.Authentication {
		in["users"] = []any{map[string]any{"username": p.Username, "password": p.Password}}
	}
	return in
}

func coreTLS(n profile.Node) map[string]any {
	tls := map[string]any{"enabled": true, "server_name": fallbackValue(n.TextOption("sni"), n.TextOption("servername")), "insecure": n.TextOption("skip-cert-verify") == "true"}
	if alpn := n.TextOption("alpn"); alpn != "" {
		tls["alpn"] = optionList(n, "alpn")
	}
	fingerprint := n.TextOption("client-fingerprint")
	if !nodeUsesQUIC(n) {
		fingerprint = n.TCPFingerprint()
	}
	if fp, _ := tlsFingerprint(fingerprint); fp != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fp}
	}
	if cert, key, err := profile.ClientCertificatePEM(n); err == nil && cert != "" {
		tls["client_certificate"] = []string{cert}
		tls["client_key"] = []string{key}
	}
	if n.Kind() == "http2" || n.TextOption("http2") == "true" {
		tls["alpn"] = []string{"h2"}
	}
	return tls
}

func nodeUsesQUIC(n profile.Node) bool {
	switch n.Kind() {
	case "hysteria", "hysteria2", "tuic", "juicity", "http3":
		return true
	case "masque":
		return n.TextOption("http2") != "true"
	case "trusttunnel":
		return n.TextOption("quic") == "true"
	case "naive":
		return n.TextOption("transport") == "quic"
	}
	return n.TextOption("network") == "quic" || n.TextOption("network") == "xhttp" && strings.Contains(n.TextOption("alpn"), "h3")
}
