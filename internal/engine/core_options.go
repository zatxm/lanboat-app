package engine

import (
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func appendUpstream(outbounds []any, p *profile.UpstreamProxy) []any {
	if p == nil || !p.Enabled {
		return outbounds
	}
	kind := p.Type
	if kind == "socks5" || kind == "" {
		kind = "socks"
	}
	for _, raw := range outbounds {
		out := raw.(map[string]any)
		if out["tag"] != "direct" {
			out["detour"] = "upstream"
		}
	}
	return append(outbounds, map[string]any{"type": kind, "tag": "upstream", "server": p.Server, "server_port": p.Port, "username": p.Username, "password": p.Password})
}
func coreOptions(n profile.Node, out map[string]any) {
	text := n.TextOption
	set := func(dst, src string) {
		if value := n.Option(src); value != nil {
			out[dst] = value
		}
	}
	integer := func(dst, src string) {
		if text(src) != "" {
			out[dst] = nodeInt(n, src, 0)
		}
	}
	seconds := func(dst, src string) {
		if text(src) != "" {
			out[dst] = text(src) + "s"
		}
	}
	for _, key := range []string{"password", "username", "uuid"} {
		if key == "password" {
			if n.Password != "" {
				out[key] = n.Password
			}
		} else {
			set(key, key)
		}
	}
	if (n.Kind() == "ss" || n.Kind() == "ssr" || n.Kind() == "lua") && n.Method != "" {
		out["method"] = n.Method
	}
	if n.Plugin != "" && n.Plugin != "none" {
		out["plugin"], out["plugin_opts"] = n.Plugin, n.PluginOpts
	}
	switch n.Kind() {
	case "shadowtls":
		out["version"] = n.EffectiveOption("version", "1")
		if out["version"] == "1" {
			delete(out, "password")
		}
	case "sudoku":
		o := map[string]any{"key": text("key"), "aead": n.EffectiveOption("aead-method", "chacha20-poly1305"), "ascii": n.EffectiveOption("table-type", "prefer_entropy"), "padding_min": nodeInt(n, "padding-min", 5), "padding_max": nodeInt(n, "padding-max", 15), "enable_pure_downlink": n.EffectiveOption("enable-pure-downlink", "true") == "true"}
		if text("custom-table") != "" {
			o["custom_table"] = text("custom-table")
		}
		if tables := n.SudokuTables(); len(tables) > 0 {
			o["custom_tables"] = tables
		}
		if text("http-mask") == "true" {
			o["httpmask"] = map[string]any{"mode": n.EffectiveOption("http-mask-mode", "auto"), "host": text("http-mask-host"), "path_root": text("path-root")}
		}
		out["sudoku"] = o
	case "naive":
		delete(out, "insecure_concurrency")

	case "ss", "ssr":
		if n.Kind() == "ssr" {
			out["type"] = "shadowsocksr"
			set("protocol", "protocol")
			set("protocol_param", "protocol-param")
			set("obfs_param", "obfs-param")
			out["obfs"] = text("obfs")
		}
		out["method"] = n.EffectiveOption("cipher", fallbackValue(n.Method, profile.DefaultSSMethod))
		out["ota"] = text("ota") == "true"
	case "socks5", "socks5s":
		out["type"] = "socks"
		out["version"] = "5"
	case "https", "http2", "http3":
		out["type"] = "http"
		if n.Kind() == "http2" {
			out["type"] = "http"
		}
		if n.Kind() == "http3" {
			out["type"] = "naive"
			out["quic"] = true
		}
		if text("naive") == "true" {
			out["type"] = "naive"
		}
	case "trojan":
		out["flow"] = cleanFlow(text("flow"))
		if text("trojan-protocol") == "shadowsocks" {
			out["shadowsocks"] = map[string]any{"enabled": true, "method": text("trojan-ss.method"), "password": text("trojan-ss.password")}
		}
	case "hysteria":
		out["obfs"] = text("obfs")
		set("protocol", "protocol")
	case "hysteria2":
		if password := text("obfs-password"); password != "" && text("obfs") != "none" {
			out["obfs"] = map[string]any{"type": n.EffectiveOption("obfs", "salamander"), "password": password}
		}
		if text("obfs") == "none" {
			delete(out, "obfs")
		}
	case "tuic":
		out["version"] = strconv.Itoa(n.TUICVersion())
		if n.TUICVersion() == 4 {
			out["token"] = n.EffectiveOption("token", n.Password)
		}
	case "snell":
		out["version"] = n.EffectiveOption("version", "1")
		delete(out, "psk")
		delete(out, "obfs_mode")
		delete(out, "obfs_host")
		if mode := text("obfs-opts.mode"); mode != "" && mode != "none" {
			out["obfs"] = map[string]any{"type": mode, "host": text("obfs-opts.host"), "path": n.EffectiveOption("obfs-opts.path", "/")}
		}
	case "ssh":
		delete(out, "user")
		out["username"] = text("username")
		out["host_key_insecure"] = text("host-key") == ""
		seconds("tcp_keep_alive", "heartbeat")
	case "wireguard", "masque":
		set("private_key", "private-key")
		set("peer_public_key", "public-key")
		set("pre_shared_key", "pre-shared-key")
		var addresses []string
		for _, key := range []string{"ip", "ipv6"} {
			if value := text(key); value != "" {
				if !strings.Contains(value, "/") {
					if strings.Contains(value, ":") {
						value += "/128"
					} else {
						value += "/32"
					}
				}
				addresses = append(addresses, value)
			}
		}
		out["local_address"] = addresses
		integer("mtu", "mtu")
		integer("persistent_keepalive_interval", "heartbeat")
		if n.Kind() == "masque" {
			out["public_key"] = text("public-key")
			delete(out, "password")
			delete(out, "username")
			delete(out, "peer_public_key")
			out["http_version"] = 3
			if text("http2") == "true" {
				out["type"] = "http"
			}
			set("masque_uri", "uri")
		}
		if value := text("reserved"); value != "" {
			var reserved []int
			for _, part := range strings.Split(value, ",") {
				v, _ := strconv.Atoi(strings.TrimSpace(part))
				reserved = append(reserved, v)
			}
			out["reserved"] = reserved
		}
	case "mieru":
		out["network"] = strings.ToLower(n.EffectiveOption("transport", "tcp"))
		set("traffic_pattern", "traffic-pattern")
		out["user_hint"] = text("user-hint") == "true"
		set("multiplexing", "multiplexing")
	case "brook":
		if network := text("network"); network == "ws" || network == "wss" {
			out["transport"] = map[string]any{"type": "ws", "path": n.EffectiveOption("path", "/"), "headers": map[string]any{"Host": text("host")}}
		}
	case "lua":
		script, _ := n.LuaScript()
		out["script"] = script
		out["method"] = n.EffectiveOption("cipher", profile.DefaultSSMethod)
	case "openconnect":
		if value := text("flavor"); value != "" && value != "anyconnect" {
			out["unsupported_openconnect_flavor"] = value
		}
		set("group", "authgroup")
		set("cookie", "cookie")
	case "trusttunnel":
		out["quic"] = text("quic") == "true"
	case "juicity":
		set("pinned_certchain_sha256", "pinned-certchain-sha256")
		out["congestion_control"] = n.EffectiveOption("congestion-controller", "bbr")
	case "tailscale":
		delete(out, "server")
		delete(out, "server_port")
		delete(out, "password")
		delete(out, "method")
		for _, key := range []string{"auth-key", "control-url", "exit-node", "state-directory", "hostname"} {
			set(strings.ReplaceAll(key, "-", "_"), key)
		}
		out["force_derp"] = text("force-derp") == "true"
		out["accept_routes"] = text("accept-routes") == "true"
	}
	if text("congestion-controller") == "none" {
		delete(out, "congestion_control")
	} else if text("congestion-controller") != "" {
		out["congestion_control"] = text("congestion-controller")
	}
	if text("idle-timeout") != "" {
		parts := strings.Split(text("idle-timeout"), ",")
		out["idle_session_timeout"] = parts[0] + "s"
		if len(parts) > 1 {
			out["idle_session_check_interval"] = parts[1] + "s"
		}
		if len(parts) > 2 {
			out["min_idle_session"], _ = strconv.Atoi(parts[2])
		}
	}
	if text("uot-version") != "" && text("uot-version") != "0" {
		out["udp_over_tcp"] = map[string]any{"enabled": true, "version": nodeInt(n, "uot-version", 0)}
	}
	if profile.NodeTLSEnabled(n) || n.Kind() == "masque" || n.Kind() == "brook" && text("network") == "wss" {
		tls := coreTLS(n)
		out["tls"] = tls
		if text("ech") != "" {
			tls["ech"] = map[string]any{"enabled": true, "config": []string{text("ech")}}
		}
		if text("tls-sha256") != "" {
			tls["certificate_sha256"] = optionList(n, "tls-sha256")
		}
		if text("fragment") == "true" {
			tls["fragment"] = true
		}
		if key := text("reality-opts.public-key"); key != "" {
			tls["reality"] = map[string]any{"enabled": true, "public_key": key, "short_id": text("reality-opts.short-id")}
		}
	}
	if transport, ok := out["transport"].(map[string]any); ok {
		if hosts, ok := transport["host"].([]string); ok && len(hosts) > 0 {
			transport["host"] = hosts[0]
		}

	}
	if n.Kind() == "shadowtls" && text("client-fingerprint") == "none" {
		if tls, ok := out["tls"].(map[string]any); ok {
			delete(tls, "utls")
		}
	}
	transportOptions(n, out)
}
