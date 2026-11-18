package profile

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ShareURI uses protocol schemes; URI remains the lossless internal export format.
func ShareURI(n Node) string {
	n = n.Clone()
	NormalizeSS(&n)
	b64 := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	if n.Kind() == "brook" {
		kind := "server"
		server := net.JoinHostPort(n.Server, strconv.Itoa(n.Port))
		q := url.Values{"password": {n.Password}, "name": {n.Name}}
		if network := n.TextOption("network"); network == "ws" || network == "wss" {
			kind = network + "server"
			host := server
			if v := n.TextOption("host"); v != "" {
				host = net.JoinHostPort(v, strconv.Itoa(n.Port))
				q.Set("address", server)
			}
			server = (&url.URL{Scheme: network, Host: host, Path: n.EffectiveOption("path", "/")}).String()
		}
		q.Set(kind, server)
		return "brook://" + kind + "?" + q.Encode()
	}
	if n.Kind() == "mieru" {
		host := n.Server
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		q := url.Values{"profile": {n.Name}, "port": {n.EffectiveOption("port-range", strconv.Itoa(n.Port))}, "protocol": {strings.ToUpper(n.EffectiveOption("transport", "tcp"))}}
		for _, key := range []string{"multiplexing", "mtu", "handshake-mode", "traffic-pattern"} {
			if v := n.TextOption(key); v != "" {
				q.Set(key, v)
			}
		}
		return (&url.URL{Scheme: "mierus", Host: host, User: url.UserPassword(n.TextOption("username"), n.Password), RawQuery: q.Encode()}).String()
	}
	if n.Kind() == "ssr" {
		q := url.Values{"remarks": {b64(n.Name)}}
		for key, dest := range map[string]string{"obfs-param": "obfsparam", "protocol-param": "protoparam"} {
			if s := n.TextOption(key); s != "" {
				q.Set(dest, b64(s))
			}
		}
		return "ssr://" + b64(net.JoinHostPort(n.Server, strconv.Itoa(n.Port))+":"+n.EffectiveOption("protocol", "origin")+":"+n.EffectiveOption("cipher", n.Method)+":"+n.EffectiveOption("obfs", "plain")+":"+b64(n.Password)+"/?"+q.Encode())
	}
	if n.Kind() == "vmess" {
		m := map[string]any{"v": "2", "ps": n.Name, "add": n.Server, "port": strconv.Itoa(n.Port), "id": n.TextOption("uuid"), "aid": n.EffectiveOption("alterId", "0"), "scy": n.EffectiveOption("cipher", "auto"), "net": n.EffectiveOption("network", "tcp"), "type": "none", "host": n.TextOption("transport-host"), "path": n.TextOption("transport-path"), "sni": n.TextOption("sni"), "alpn": n.TextOption("alpn"), "fp": n.TextOption("client-fingerprint")}
		if m["net"] == "grpc" {
			m["path"] = n.TextOption("service-name")
		}
		if NodeTLSEnabled(n) {
			m["tls"] = "tls"
		}
		if n.TextOption("skip-cert-verify") == "true" {
			m["allowInsecure"] = true
		}
		raw, _ := json.Marshal(m)
		return "vmess://" + base64.StdEncoding.EncodeToString(raw)
	}
	q := url.Values{}
	// Query names follow commonly supported link formats, including transport/TLS extensions.
	names := map[string]string{"skip-cert-verify": "allowInsecure", "client-fingerprint": "fp", "network": "type", "transport-host": "host", "transport-path": "path", "service-name": "serviceName", "reality-opts.public-key": "pbk", "reality-opts.short-id": "sid", "cipher": "encryption", "tls-sha256": "pinSHA256"}
	var put func(string, any)
	put = func(key string, value any) {
		if value == nil {
			return
		}
		if values, ok := value.([]any); ok {
			parts := make([]string, 0, len(values))
			for _, v := range values {
				raw, _ := json.Marshal(v)
				parts = append(parts, strings.Trim(string(raw), "\""))
			}
			value = strings.Join(parts, ",")
		}
		if values, ok := value.([]string); ok {
			value = strings.Join(values, ",")
		}
		if values, ok := value.(map[string]any); ok {
			for k, v := range values {
				put(key+"."+k, v)
			}
			return
		}
		raw, _ := json.Marshal(value)
		s := strings.Trim(string(raw), "\"")
		if v, ok := value.(string); ok {
			s = v
		}
		if s == "" {
			return
		}
		if dest := names[key]; dest != "" {
			key = dest
		}
		q.Set(key, s)
	}
	for key, value := range n.Options {
		if key != "tls" && key != "uuid" && key != "username" && key != "plugin-opts" {
			put(key, value)
		}
	}
	scheme := n.Kind()
	user := url.UserPassword(n.TextOption("username"), n.Password)
	switch scheme {
	case "ss":
		user = url.User(b64(n.Method + ":" + n.Password))
		if strings.HasPrefix(n.Method, "2022-") {
			user = url.UserPassword(n.Method, n.Password)
		}
		plugin := n.Plugin
		opts := n.PluginOpts
		if plugin != "" && plugin != "none" {
			if opts == "" {
				if m, ok := n.Options["plugin-opts"].(map[string]any); ok {
					keys := make([]string, 0, len(m))
					for k := range m {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					var values []string
					for _, k := range keys {
						if b, ok := m[k].(bool); ok {
							if b {
								values = append(values, k)
							}
							continue
						}
						raw, _ := json.Marshal(m[k])
						s := strings.Trim(string(raw), "\"")
						if v, ok := m[k].(string); ok {
							s = v
						}
						if s != "" {
							values = append(values, k+"="+s)
						}
					}
					opts = strings.Join(values, ";")
				}
			}
			q.Set("plugin", plugin)
			if opts != "" {
				q.Set("plugin", plugin+";"+opts)
			}
		} else if mode := n.TextOption("obfs"); mode == "http" || mode == "tls" {
			q.Set("plugin", "obfs-local;obfs="+mode+";obfs-host="+n.TextOption("obfs-host"))
		}
		q.Del("encryption")
	case "vless":
		user = url.User(n.TextOption("uuid"))
		q.Set("encryption", n.EffectiveOption("encryption", "none"))
		q.Del("cipher")
	case "juicity", "tuic":
		user = url.UserPassword(n.TextOption("uuid"), n.Password)
		if scheme == "tuic" && n.TUICVersion() == 4 {
			user = url.User(n.EffectiveOption("token", n.Password))
		}
	case "trojan", "hysteria2", "anytls", "snell", "brook":
		user = url.User(n.Password)
	case "wireguard":
		user = url.User(n.TextOption("private-key"))
	case "hysteria":
		user = nil
		if n.TextOption("obfs") != "" {
			q.Set("obfsParam", n.TextOption("obfs"))
			q.Set("obfs", "xplus")
		}
		q.Set("auth", n.Password)
		q.Set("peer", n.TextOption("sni"))
		q.Set("upmbps", n.EffectiveOption("up", "100"))
		q.Set("downmbps", n.EffectiveOption("down", "100"))
	case "naive":
		scheme = "naive+https"
		if n.TextOption("transport") == "quic" {
			scheme = "naive+quic"
		}
	}
	if scheme == "vless" || scheme == "trojan" {
		if NodeTLSEnabled(n) {
			q.Set("security", "tls")
			if n.TextOption("reality-opts.public-key") != "" {
				q.Set("security", "reality")
			}
		} else {
			q.Set("security", "none")
		}
	}
	if q.Get("flow") == "none" {
		q.Del("flow")
	}
	if n.Kind() == "hysteria2" {
		q.Del("allowInsecure")
		if n.TextOption("skip-cert-verify") == "true" {
			q.Set("insecure", "1")
		}
		if n.TextOption("obfs-password") != "" {
			q.Set("obfs", "salamander")
		}
		q.Del("type")
	}
	if user != nil && user.Username() == "" && n.Password == "" {
		user = nil
	}
	u := url.URL{Scheme: scheme, Host: net.JoinHostPort(n.Server, strconv.Itoa(n.Port)), User: user, Fragment: n.Name, RawQuery: q.Encode()}
	if n.Kind() == "ss" && q.Has("plugin") {
		u.Path = "/"
	}
	return u.String()
}
