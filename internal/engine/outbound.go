package engine

import (
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
)

func coreOutbound(n profile.Node) map[string]any {
	out := map[string]any{"type": n.Kind(), "tag": "proxy", "server": n.Server, "server_port": n.Port}
	number := func(key string, def int) int {
		if text := n.TextOption(key); text != "" {
			v, _ := strconv.Atoi(text)
			return v
		}
		return def
	}
	if n.Option("tfo") != nil {
		out["tcp_fast_open"] = n.TextOption("tfo") == "true"
	}
	switch n.Kind() {
	case "naive":
		out["username"], out["password"] = n.TextOption("username"), n.Password
		out["quic"] = n.TextOption("transport") == "quic"
		out["insecure_concurrency"] = number("insecure-concurrency", 0)
		headers, _ := n.NaiveHeaders()
		out["extra_headers"] = headers
		tls := map[string]any{"enabled": true, "server_name": n.EffectiveOption("sni", n.Server)}
		if certificate, _ := n.NaiveCertificatePEM(); certificate != "" {
			tls["certificate"] = []string{certificate}
		}
		out["tls"] = tls
		if version := number("uot-version", 0); version != 0 {
			out["udp_over_tcp"] = map[string]any{"enabled": true, "version": version}
		}
	case "ss":
		out["type"] = "shadowsocks"
		out["method"] = n.Method
		out["password"] = n.Password
	case "vmess":
		out["uuid"] = n.TextOption("uuid")
		out["security"] = fallbackValue(n.TextOption("cipher"), "auto")
		out["alter_id"] = number("alterId", 0)
	case "vless":
		out["uuid"] = n.TextOption("uuid")
		out["flow"] = cleanFlow(n.TextOption("flow"))
	case "socks5":
		out["type"] = "socks"
		out["version"] = "5"
		out["username"] = n.TextOption("username")
		out["password"] = n.Password
	case "http", "https", "http2":
		out["type"] = "http"
		out["username"] = n.TextOption("username")
		out["password"] = n.Password
		if path := n.TextOption("path"); path != "" {
			out["path"] = path
		}
	case "trojan", "anytls":
		out["password"] = n.Password
	case "hysteria":
		out["auth_str"] = n.Password
		out["up_mbps"] = number("up", 10)
		out["down_mbps"] = number("down", 50)
		out["obfs"] = n.TextOption("obfs")
	case "hysteria2":
		out["password"] = n.Password
		for _, key := range []string{"up", "down"} {
			if speed := number(key, 0); speed > 0 {
				out[key+"_mbps"] = speed
			}
		}
		if n.TextOption("obfs") != "" {
			out["obfs"] = map[string]any{"type": n.TextOption("obfs"), "password": n.TextOption("obfs-password")}
		}
		if ports := n.TextOption("ports"); ports != "" {
			delete(out, "server_port")
			out["server_ports"] = strings.FieldsFunc(strings.ReplaceAll(ports, "-", ":"), func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t'
			})
			if interval := n.TextOption("hop-interval"); interval != "" {
				out["hop_interval"] = interval
			}
		}
	case "tuic":
		out["uuid"] = n.TextOption("uuid")
		out["password"] = n.Password
		out["congestion_control"] = fallbackValue(n.TextOption("congestion-controller"), "cubic")
		out["udp_relay_mode"] = fallbackValue(n.TextOption("udp-relay-mode"), "native")
	case "snell":
		out["password"] = n.Password
	case "ssh":
		out["user"] = n.TextOption("username")
		out["password"] = n.Password
		if key := n.TextOption("private-key"); key != "" {
			out["private_key"] = strings.Split(key, "\n")
		}
		if passphrase := n.TextOption("private-key-passphrase"); passphrase != "" {
			out["private_key_passphrase"] = passphrase
		}
		if hostKey := n.TextOption("host-key"); hostKey != "" {
			out["host_key"] = strings.Split(hostKey, ",")
		}
	}
	tlsEnabled := false
	switch n.Kind() {
	case "https", "http2", "trojan", "hysteria", "hysteria2", "anytls", "tuic":
		tlsEnabled = true
	case "vmess", "vless":
		tlsEnabled = n.TextOption("tls") == "true"
	}
	if tlsEnabled {
		tls := coreTLS(n)
		if key := n.TextOption("reality-opts.public-key"); key != "" {
			tls["reality"] = map[string]any{"enabled": true, "public_key": key, "short_id": n.TextOption("reality-opts.short-id")}
			if tls["utls"] == nil {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
			}
		}
		out["tls"] = tls
	}
	if n.Kind() == "vmess" || n.Kind() == "vless" || n.Kind() == "trojan" {
		network := n.TextOption("network")
		if network != "" && network != "tcp" {
			transport := map[string]any{"type": network}
			path, host := n.TextOption("transport-path"), n.TextOption("transport-host")
			if path == "" {
				path = n.TextOption(network + "-opts.path")
			}
			headers := map[string]any{}
			if original, ok := n.Option(network + "-opts.headers").(map[string]any); ok {
				for key, value := range original {
					headers[key] = value
				}
			}
			var hostHeader any
			for key, value := range headers {
				if strings.EqualFold(key, "Host") {
					hostHeader = value
					delete(headers, key)
				}
			}
			hosts := []string{}
			switch value := hostHeader.(type) {
			case string:
				hosts = []string{value}
			case []string:
				hosts = append(hosts, value...)
			case []any:
				for _, item := range value {
					if text, ok := item.(string); ok {
						hosts = append(hosts, text)
					}
				}
			}
			if host != "" {
				hosts = strings.Split(host, ",")
				for i := range hosts {
					hosts[i] = strings.TrimSpace(hosts[i])
				}
				filtered := hosts[:0]
				for _, item := range hosts {
					if item != "" {
						filtered = append(filtered, item)
					}
				}
				hosts = filtered
			}

			switch network {
			case "ws":
				transport["path"] = path
				if len(hosts) > 0 {
					headers["Host"] = hosts[0]
				}
				if len(headers) > 0 {
					transport["headers"] = headers
				}
			case "h2", "http":
				transport["type"] = "http"
				transport["path"] = path
				if len(hosts) > 0 {
					transport["host"] = hosts
				}
				if method := n.TextOption("transport-method"); method != "" {
					transport["method"] = method
				}
				if len(headers) > 0 {
					transport["headers"] = headers
				}
			case "grpc":
				transport["service_name"] = fallbackValue(n.TextOption("service-name"), n.TextOption("grpc-opts.grpc-service-name"))
			case "httpupgrade":
				transport["path"] = path
				if len(hosts) > 0 {
					transport["host"] = hosts[0]
				}
				if len(headers) > 0 {
					transport["headers"] = headers
				}
			}
			out["transport"] = transport
		}
	}
	if n.TextOption("mux") == "true" {
		if n.Kind() == "snell" {
			out["multiplex"] = map[string]any{"enabled": true}
		} else {
			out["multiplex"] = map[string]any{"enabled": true, "protocol": "smux", "max_connections": 4, "min_streams": 4, "max_streams": 16}
		}
	}
	coreOptions(n, out)
	return out
}
func fallbackValue(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

func optionList(n profile.Node, key string) []string {
	values := strings.Split(n.TextOption(key), ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	return values
}

func cleanFlow(flow string) string {
	if flow == "none" {
		return ""
	}
	return flow
}
func nodeInt(n profile.Node, key string, def int) int {
	value := n.TextOption(key)
	if value == "" {
		return def
	}
	i, _ := strconv.Atoi(value)
	return i
}
