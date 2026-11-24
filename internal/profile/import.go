package profile

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"net"
	"net/netip"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

func ParseURI(raw string) (Node, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > MaxImport {
		return Node{}, errors.New("节点链接过长")
	}
	if i := strings.Index(raw, "://"); i > 0 {
		raw = strings.ToLower(raw[:i]) + raw[i:]
	}
	if strings.HasPrefix(raw, "lanboat://") {
		data, err := DecodeBase64(strings.TrimPrefix(raw, "lanboat://"))
		if err != nil {
			return Node{}, err
		}
		var n Node
		if json.Unmarshal(data, &n) != nil {
			return Node{}, errors.New("岚舟分享链接无效")
		}
		n.ID = ID()
		n.Source = ""
		return n, n.Validate()
	}
	if strings.HasPrefix(strings.ToLower(raw), "ss://") {
		return parseSS(raw)
	}
	if strings.HasPrefix(raw, "ssr://") {
		return parseSSR(raw)
	}
	if strings.HasPrefix(raw, "vmess://") && !strings.Contains(strings.Split(strings.TrimPrefix(raw, "vmess://"), "#")[0], "@") {
		return parseVMess(raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Node{}, errors.New("节点链接格式无效")
	}
	if u.Scheme == "brook" && u.User == nil && u.Query().Get(u.Host) != "" {
		q := u.Query()
		kind := u.Host
		if kind != "server" && kind != "wsserver" && kind != "wssserver" {
			return Node{}, errors.New("Brook 链接传输方式无效")
		}
		server := q.Get(kind)
		network := "none"
		host, path := "", ""
		if kind == "wsserver" || kind == "wssserver" {
			network = strings.TrimSuffix(kind, "server")
			parsed, e := url.Parse(server)
			if e != nil {
				return Node{}, e
			}
			server = parsed.Host
			path = parsed.Path
			if override := q.Get("address"); override != "" {
				host = parsed.Hostname()
				server = override
			}
		}
		address, p, e := net.SplitHostPort(server)
		if e != nil {
			return Node{}, e
		}
		port, e := strconv.Atoi(p)
		if e != nil {
			return Node{}, e
		}
		n := Node{ID: ID(), Type: "brook", Name: q.Get("name"), Server: address, Port: port, Password: q.Get("password")}
		n.SetOption("network", network)
		if path != "" {
			n.SetOption("path", path)
		}
		if host != "" {
			n.SetOption("host", host)
		}
		if n.Name == "" {
			n.Name = address
		}
		applyDefaults(&n)
		return n, n.Validate()
	}
	if u.Scheme == "mierus" {
		q := u.Query()
		if u.User == nil {
			return Node{}, errors.New("Mieru 链接缺少用户信息")
		}
		first, _, _ := strings.Cut(q.Get("port"), "-")
		port, e := strconv.Atoi(first)
		if e != nil {
			return Node{}, e
		}
		password, _ := u.User.Password()
		n := Node{ID: ID(), Type: "mieru", Name: q.Get("profile"), Server: u.Hostname(), Port: port, Password: password}
		n.SetOption("username", u.User.Username())
		n.SetOption("transport", strings.ToLower(q.Get("protocol")))
		if strings.Contains(q.Get("port"), "-") {
			n.SetOption("port-range", q.Get("port"))
		}
		for _, k := range []string{"multiplexing", "mtu", "handshake-mode", "traffic-pattern"} {
			if v := q.Get(k); v != "" {
				n.SetOption(k, v)
			}
		}
		if n.Name == "" {
			n.Name = n.Server
		}
		applyDefaults(&n)
		return n, n.Validate()
	}
	kind := strings.ToLower(u.Scheme)
	aliases := map[string]string{"hy2": "hysteria2", "socks": "socks5", "socks5-tls": "socks5s", "socks5+tls": "socks5s", "http+tls": "https", "wg": "wireguard", "ss2022": "ss", "gost-relay": "relay", "naive+https": "naive", "naive+quic": "http3"}
	if alias := aliases[kind]; alias != "" {
		kind = alias
	}
	if kind == "ss" {
		return parseSS("ss://" + strings.TrimPrefix(raw, u.Scheme+"://"))
	}
	if _, ok := ProtocolByID(kind); !ok && kind != "amneziawg" {
		return Node{}, fmt.Errorf("不支持的节点协议：%s", kind)
	}
	port := 443
	if kind == "socks5" {
		port = 1080
	}
	if kind == "http" {
		port = 8080
	}
	if kind == "ssh" {
		port = 22
	}
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil {
			return Node{}, errors.New("节点端口无效")
		}
	}
	n := Node{ID: ID(), Type: kind, Name: u.Fragment, Server: u.Hostname(), Port: port, Options: map[string]any{}}
	if n.Name == "" {
		n.Name = net.JoinHostPort(n.Server, strconv.Itoa(n.Port))
	}
	if u.User != nil {
		user := u.User.Username()
		password, has := u.User.Password()
		if !has && (kind == "socks5" || kind == "socks5s" || kind == "http" || kind == "https") {
			if b, err := DecodeBase64(user); err == nil && strings.Contains(string(b), ":") {
				user, password, has = strings.Cut(string(b), ":")
			}
		}
		switch kind {
		case "vmess", "vless", "juicity":
			n.SetOption("uuid", user)
			n.Password = password
		case "tuic":
			if has {
				n.SetOption("uuid", user)
				n.Password = password
			} else {
				n.SetOption("token", user)
			}
		case "wireguard", "amneziawg":
			n.SetOption("private-key", user)
		case "trojan", "hysteria2", "anytls", "snell", "shadowtls", "brook":
			n.Password = user
			if has {
				n.Password = user + ":" + password
			}
		default:
			n.SetOption("username", user)
			n.Password = password
		}
	}
	q := u.Query()
	mapping := map[string]string{"sni": "sni", "peer": "sni", "allowInsecure": "skip-cert-verify", "insecure": "skip-cert-verify", "allow-insecure": "skip-cert-verify", "fp": "client-fingerprint", "fingerprint": "client-fingerprint", "pbk": "reality-opts.public-key", "sid": "reality-opts.short-id", "type": "network", "host": "transport-host", "path": "transport-path", "serviceName": "service-name", "flow": "flow", "alpn": "alpn", "publickey": "public-key", "public-key": "public-key", "private-key": "private-key", "presharedkey": "pre-shared-key", "pre-shared-key": "pre-shared-key", "address": "ip", "ip": "ip", "ipv6": "ipv6", "reserved": "reserved", "mtu": "mtu", "obfs": "obfs", "obfs-password": "obfs-password", "obfsParam": "obfs", "pinSHA256": "tls-sha256", "upmbps": "up", "downmbps": "down", "up": "up", "down": "down", "auth": "auth-str", "auth_str": "auth-str", "allow_insecure": "skip-cert-verify", "pinned_certchain_sha256": "pinned-certchain-sha256", "protocol": "protocol", "protocol-param": "protocol-param", "obfs-param": "obfs-param", "version": "version", "ports": "ports", "username": "username", "congestion_control": "congestion-controller", "congestion-controller": "congestion-controller", "udp_relay_mode": "udp-relay-mode", "udp-relay-mode": "udp-relay-mode", "ss-method": "ss-method", "ss-password": "ss-password", "psk": "psk", "transport": "transport", "multiplexing": "multiplexing", "key": "key", "aead-method": "aead-method", "table-type": "table-type", "http-mask": "http-mask", "uri": "uri", "cookie": "cookie", "authgroup": "authgroup"}
	if kind == "shadowtls" && !q.Has("ss-method") && !q.Has("ss-password") {
		for _, key := range []string{"host", "sni", "peer"} {
			mapping[key] = "host"
		}
	}
	for key, values := range q {
		if len(values) != 1 {
			return Node{}, fmt.Errorf("节点参数重复：%s", key)
		}
		value := values[0]
		if key == "pcs" && value == "" {
			continue
		}
		if key == "headerType" && (kind == "vless" || kind == "vmess") {
			switch q.Get("type") {
			case "mkcp", "kcp":
				n.SetOption("kcp.header", value)
			case "quic":
				n.SetOption("quic.header", value)
			default:
				if value != "" && value != "none" {
					return Node{}, errors.New("不支持的节点传输头")
				}
			}
			continue
		}
		if key == "quicSecurity" && (kind == "vless" || kind == "vmess") {
			if q.Get("type") == "quic" {
				n.SetOption("quic.security", value)
			} else if value != "" && value != "none" {
				return Node{}, errors.New("QUIC 加密设置需要 QUIC 传输")
			}
			continue
		}
		if kind == "hysteria2" && key == "mport" {
			if !q.Has("ports") {
				n.SetOption("ports", strings.Trim(value, ", "))
			}
			continue
		}
		if key == "security" {
			n.SetOption("tls", value == "tls" || value == "reality")
			continue
		}
		if key == "tls" {
			n.SetOption("tls", value == "1" || value == "true" || value == "tls")
			continue
		}
		if key == "password" {
			n.Password = value
			continue
		}
		if dest := mapping[key]; dest != "" {
			if dest == "skip-cert-verify" {
				n.SetOption(dest, value == "1" || value == "true")
			} else {
				n.SetOption(dest, value)
			}
		} else if knownField(kind, key) {
			n.SetOption(key, value)
		} else {
			return Node{}, fmt.Errorf("未识别的节点参数：%s", key)
		}
	}
	if n.Type == "hysteria" && q.Get("obfsParam") != "" {
		n.SetOption("obfs", q.Get("obfsParam"))
	}
	if n.Type == "hysteria" && n.Password == "" {
		n.Password = n.TextOption("auth-str")
	}
	applyDefaults(&n)
	return n, n.Validate()
}
func applyDefaults(n *Node) {
	n.normalizeProtocol()
	if p, ok := ProtocolByID(n.Kind()); ok {
		for _, field := range p.Fields {
			if field.Default != "" && n.Option(field.Key) == nil {
				n.SetOption(field.Key, field.Default)
			}
		}
	}
}
func parseVMess(raw string) (Node, error) {
	body := strings.TrimPrefix(raw, "vmess://")
	if i := strings.IndexByte(body, '#'); i >= 0 {
		body = body[:i]
	}
	data, err := DecodeBase64(body)
	if err != nil {
		return Node{}, err
	}
	var values map[string]any
	if json.Unmarshal(data, &values) != nil {
		return Node{}, errors.New("VMess 链接内容不是有效 JSON")
	}
	get := func(k string) string {
		if v := values[k]; v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	port, _ := strconv.Atoi(get("port"))
	n := Node{ID: ID(), Type: "vmess", Name: get("ps"), Server: get("add"), Port: port, Options: map[string]any{}}
	if n.Name == "" {
		n.Name = n.Server
	}
	for key, dest := range map[string]string{"id": "uuid", "aid": "alterId", "scy": "cipher", "net": "network", "host": "transport-host", "path": "transport-path", "sni": "sni", "fp": "client-fingerprint", "alpn": "alpn"} {
		if value := get(key); value != "" {
			n.SetOption(dest, value)
		}
	}
	if get("net") == "grpc" {
		n.SetOption("service-name", get("path"))
	}
	n.SetOption("tls", get("tls") == "tls" || get("tls") == "true")
	if get("allowInsecure") == "true" || get("allowInsecure") == "1" {
		n.SetOption("skip-cert-verify", true)
	}
	applyDefaults(&n)
	return n, n.Validate()
}
func parseSSR(raw string) (Node, error) {
	body, err := DecodeBase64(strings.TrimPrefix(raw, "ssr://"))
	if err != nil {
		return Node{}, err
	}
	authority, query, _ := strings.Cut(string(body), "/?")
	parts := strings.Split(authority, ":")
	if len(parts) < 6 {
		return Node{}, errors.New("SSR 链接缺少字段")
	}
	// The final five components are fixed; preceding colons belong to an IPv6 host.
	split := len(parts) - 5
	server := strings.Trim(strings.Join(parts[:split], ":"), "[]")
	fixed := parts[split:]
	port, _ := strconv.Atoi(fixed[0])
	password, err := DecodeBase64(fixed[4])
	if err != nil {
		return Node{}, err
	}
	n := Node{ID: ID(), Type: "ssr", Server: server, Port: port, Password: string(password), Options: map[string]any{"protocol": fixed[1], "cipher": fixed[2], "obfs": fixed[3]}}
	q, err := url.ParseQuery(query)
	if err != nil {
		return Node{}, errors.New("SSR 参数编码无效")
	}
	for key, dest := range map[string]string{"obfsparam": "obfs-param", "protoparam": "protocol-param", "remarks": "name"} {
		if v := q.Get(key); v != "" {
			decoded, e := DecodeBase64(v)
			if e != nil {
				return Node{}, e
			}
			if dest == "name" {
				n.Name = string(decoded)
			} else {
				n.SetOption(dest, string(decoded))
			}
		}
	}
	if n.Name == "" {
		n.Name = net.JoinHostPort(server, strconv.Itoa(port))
	}
	return n, n.Validate()
}
func ParseImport(content string) ([]Node, error) {
	if len(content) > MaxImport {
		return nil, errors.New("导入内容不能超过 2 MiB")
	}
	return parseImport(content, 1000, nil)
}

// ParseImportWithRules keeps supported routing rules alongside imported local nodes.
func ParseImportWithRules(content, base string) ([]Node, string, error) {
	if len(content) > MaxImport {
		return nil, "", errors.New("导入内容不能超过 2 MiB")
	}
	var rules []string
	nodes, err := parseImport(content, 1000, &rules)
	if err != nil || len(rules) == 0 {
		return nodes, "", err
	}
	merged, err := mergeSubscriptionRulesKnown(base, rules, nodes)
	return nodes, merged, err
}
func parseImport(content string, limit int, rules *[]string) ([]Node, error) {
	if len(content) > MaxSubscription {
		return nil, errors.New("订阅内容不能超过 32 MiB")
	}
	content = strings.TrimSpace(strings.TrimPrefix(content, "\ufeff"))
	for depth := 0; depth <= 4; depth++ {
		if content == "" {
			return nil, errors.New("订阅内容为空")
		}
		if IsProxyText(content) {
			return parseProxyText(content, limit, rules)
		}
		if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") || strings.Contains(content, "proxies:") || strings.Contains(content, "outbounds:") {
			return parseStructured(content, limit, rules)
		}
		if strings.Contains(content, "://") {
			break
		}
		if depth == 4 {
			return nil, errors.New("Base64 订阅嵌套层数过多")
		}
		decoded, err := DecodeBase64(strings.Join(strings.Fields(content), ""))
		if err != nil {
			return nil, errors.New("内容不是节点链接、Clash/JSON 配置或有效 Base64")
		}
		content = strings.TrimSpace(string(decoded))
		if len(content) > MaxSubscription {
			return nil, errors.New("Base64 解码内容过大")
		}
	}
	var nodes []Node
	for lineNo, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if proxyTextComment(line) || strings.HasPrefix(line, "STATUS=") || strings.HasPrefix(line, "REMARKS=") {
			continue
		}
		n, err := ParseURI(line)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行：%w", lineNo+1, err)
		}
		nodes = append(nodes, n)
		if len(nodes) > limit {
			return nil, fmt.Errorf("一次最多导入 %d 个节点", limit)
		}
	}
	if len(nodes) == 0 {
		return nil, errors.New("没有找到可导入节点")
	}
	return nodes, nil
}
func parseStructured(content string, limit int, rules *[]string) ([]Node, error) {
	var doc any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, errors.New("订阅 JSON/YAML 格式无效")
	}
	var items []any
	switch value := doc.(type) {
	case []any:
		items = value
	case map[string]any:
		if outbounds, ok := value["outbounds"].([]any); ok {
			return parseOutboundConfig(value, outbounds, limit, rules)
		}
		if outbound, ok := value["outbound"].(map[string]any); ok {
			return parseOutboundConfig(value, []any{outbound}, limit, rules)
		}
		if rules != nil {
			groupNames, err := clashGroups(value, rules)
			if err != nil {
				return nil, err
			}
			if list, ok := value["rules"].([]any); ok {
				for _, item := range list {
					line, ok := item.(string)
					if !ok {
						return nil, errors.New("订阅分流规则必须是文本列表")
					}
					parts := strings.Split(line, ",")
					var inline []any
					inlineBehavior := ""
					providerInterval := ""
					providerHeaders := ""
					if len(parts) >= 3 && strings.EqualFold(strings.TrimSpace(parts[0]), "RULE-SET") {
						providers, _ := value["rule-providers"].(map[string]any)
						providerValue, exists := providers[strings.TrimSpace(parts[1])]
						if !exists {
							return nil, fmt.Errorf("规则引用了未定义的规则提供器 %s", strings.TrimSpace(parts[1]))
						}
						provider, ok := providerValue.(map[string]any)
						if !ok {
							return nil, fmt.Errorf("规则提供器 %s 格式无效", strings.TrimSpace(parts[1]))
						}
						if err := validateClashRuleProvider(provider); err != nil {
							return nil, fmt.Errorf("规则提供器 %s：%w", strings.TrimSpace(parts[1]), err)
						}
						behavior := mapString(provider, "behavior")
						format := mapString(provider, "format")
						switch behavior {
						case "", "classical":
						case "domain":
							parts[0] = "DOMAIN-SET"
						case "ipcidr":
							parts[0] = "IP-SET"
						}
						if format == "mrs" {
							if behavior == "domain" {
								parts[0] = "MRS-DOMAIN"
							} else if behavior == "ipcidr" {
								parts[0] = "MRS-IP"
							}
						}
						switch mapString(provider, "type") {
						case "inline":
							inline, _ = provider["payload"].([]any)
							inlineBehavior = behavior
						case "http":
							parts[1] = mapString(provider, "url")
							providerInterval = mapString(provider, "interval")
							if rawHeaders := mapObject(provider, "header"); len(rawHeaders) > 0 {
								headers, err := importedHTTPHeaders(rawHeaders)
								if err != nil {
									return nil, fmt.Errorf("规则提供器 %s 请求头：%w", strings.TrimSpace(parts[1]), err)
								}
								encoded, err := json.Marshal(headers)
								if err != nil {
									return nil, err
								}
								providerHeaders = "headers=" + base64.RawURLEncoding.EncodeToString(encoded)
							}
						case "file":
							parts[1] = mapString(provider, "path")
						}
					}
					policy := 2
					if len(parts) > 0 && (strings.EqualFold(strings.TrimSpace(parts[0]), "MATCH") || strings.EqualFold(strings.TrimSpace(parts[0]), "FINAL")) {
						policy = 1
					}
					if policy < len(parts) {
						if name, ok := groupNames[strings.TrimSpace(parts[policy])]; ok {
							parts[policy] = name
						}
					}
					if inline != nil {
						for _, raw := range inline {
							entry := strings.TrimSpace(fmt.Sprint(raw))
							kind, match, flags := "", entry, ""
							switch inlineBehavior {
							case "domain":
								kind = "DOMAIN"
								if strings.HasPrefix(entry, "+.") || strings.HasPrefix(entry, "*.") {
									kind, match = "DOMAIN-SUFFIX", entry[2:]
								} else if strings.HasPrefix(entry, ".") {
									kind, match = "DOMAIN-REGEX", `^.+\.`+regexp.QuoteMeta(entry[1:])+`$`
								}
							case "ipcidr":
								kind = "IP-CIDR"
								if ip, e := netip.ParseAddr(entry); e == nil {
									match = netip.PrefixFrom(ip, ip.BitLen()).String()
								}
								if strings.Contains(match, ":") {
									kind = "IP-CIDR6"
								}
							default:
								kind, match, _ = strings.Cut(entry, ",")
								if strings.HasSuffix(match, ",no-resolve") {
									match = strings.TrimSuffix(match, ",no-resolve")
									flags = ",no-resolve"
								}
							}
							*rules = append(*rules, kind+","+match+","+parts[policy]+flags)
						}
						continue
					}
					if providerInterval != "" {
						parts = append(parts, "interval="+providerInterval)
					}
					if providerHeaders != "" {
						parts = append(parts, providerHeaders)
					}
					*rules = append(*rules, strings.Join(parts, ","))
				}
			}
		}
		if proxies, ok := value["proxies"].([]any); ok {
			items = proxies
		} else if list, ok := value["nodes"].([]any); ok {
			if len(list) == 0 || len(list) > limit {
				return nil, fmt.Errorf("订阅节点数量必须为 1–%d", limit)
			}
			data, _ := json.Marshal(list)
			var n []Node
			if err := json.Unmarshal(data, &n); err != nil {
				return nil, errors.New("节点 JSON 无效")
			}
			for i := range n {
				n[i].ID = ID()
				n[i].Source = ""
				applyDefaults(&n[i])
				if err := n[i].Validate(); err != nil {
					return nil, fmt.Errorf("节点 %d：%w", i+1, err)
				}
			}
			return n, nil
		} else {
			return nil, errors.New("JSON/YAML 中没有 proxies 或 nodes 列表")
		}
	}
	if len(items) == 0 || len(items) > limit {
		return nil, fmt.Errorf("订阅节点数量必须为 1–%d", limit)
	}
	var result []Node
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("节点 %d 格式错误", i+1)
		}
		var n Node
		kind, _ := m["type"].(string)
		switch kind {
		case "shadowsocks":
			kind = "ss"
		case "gost-relay":
			kind = "relay"
		}
		n = Node{ID: ID(), Type: kind, Options: map[string]any{}}
		for key, value := range m {
			switch key {
			case "name":
				n.Name, _ = value.(string)
			case "server":
				n.Server, _ = value.(string)
			case "port":
				n.Port, _ = strconv.Atoi(fmt.Sprint(value))
			case "password", "psk":
				n.Password, _ = value.(string)
			case "cipher":
				if kind == "ss" {
					n.Method, _ = value.(string)
				} else {
					n.Options[key] = value
				}
			case "plugin":
				n.Plugin, _ = value.(string)
			case "type":
			case "mport":
				if kind != "hysteria2" {
					return nil, fmt.Errorf("节点 %d：mport 仅适用于 Hysteria2", i+1)
				}
				if m["ports"] == nil {
					n.SetOption("ports", strings.Trim(fmt.Sprint(value), ", "))
				}
			case "ws-opts", "h2-opts", "http-opts", "grpc-opts", "httpupgrade-opts", "xhttp-opts", "reality-opts", "obfs-opts":
			default:
				if key == "tls" && (kind == "socks5" || kind == "http") {
					n.SetOption("tls", value)
					continue
				}
				option := map[string]string{"servername": "sni", "server-name": "sni", "host": "transport-host", "path": "transport-path", "serviceName": "service-name", "grpc-service-name": "service-name", "alter-id": "alterId", "smux": "mux", "auth": "auth-str"}[key]
				if option == "" {
					option = key
				}
				if option != "plugin-opts" && !knownField(kind, option) && option != "plugin-opts" {
					return nil, fmt.Errorf("节点 %d 包含不支持的 Clash 配置项 %s", i+1, key)
				}
				n.SetOption(option, value)
			}
		}
		if err := importClashNodeOptions(&n, m); err != nil {
			return nil, fmt.Errorf("节点 %d：%w", i+1, err)
		}
		if n.Type == "socks5" && n.TextOption("tls") == "true" {
			n.Type = "socks5s"
		}
		if n.Type == "http" && n.TextOption("tls") == "true" {
			n.Type = "https"
		}
		if n.Kind() == "ss" && n.TextOption("network") != "" && n.TextOption("network") != "tcp" {
			if n.TextOption("network") != "xhttp" {
				return nil, fmt.Errorf("节点 %d：Shadowsocks 不支持传输方式 %s", i+1, n.TextOption("network"))
			}
			n.SetOption("obfs", "xhttp")
			n.SetOption("obfs-host", n.TextOption("transport-host"))
			n.SetOption("obfs-path", n.TextOption("transport-path"))
			n.SetOption("xhttp-mode", n.TextOption("xhttp-opts.mode"))
			n.SetOption("obfs-tls", n.TextOption("tls") == "true")
		}
		if n.Name == "" {
			n.Name = n.Server
		}
		if n.Type == "ss" && n.Plugin == "obfs" {
			n.Plugin = "obfs-local"
			opts, _ := n.Options["plugin-opts"].(map[string]any)
			n.PluginOpts = "obfs=" + fmt.Sprint(opts["mode"]) + ";obfs-host=" + fmt.Sprint(opts["host"])
		}
		applyDefaults(&n)
		if err := n.Validate(); err != nil {
			return nil, fmt.Errorf("节点 %d：%w", i+1, err)
		}
		result = append(result, n)
	}
	return result, nil
}

func importClashNodeOptions(n *Node, values map[string]any) error {
	if options := mapObject(values, "plugin-opts"); options != nil {
		if n.Plugin == "obfs" {
			if err := unsupportedKeys(options, "mode", "host"); err != nil {
				return fmt.Errorf("plugin-opts：%w", err)
			}
		} else {
			allowed := map[string]bool{}
			for _, field := range SSPluginFields(n.Plugin) {
				allowed[strings.TrimPrefix(field.Key, "plugin-opts.")] = true
			}
			for key, value := range options {
				if !allowed[key] && value != nil {
					return fmt.Errorf("plugin-opts：不支持配置项 %s", key)
				}
			}
		}
	}
	if options := mapObject(values, "obfs-opts"); options != nil {
		if n.Kind() != "snell" {
			return errors.New("obfs-opts 目前仅支持 Snell 节点")
		}
		if err := unsupportedKeys(options, "mode", "host", "path"); err != nil {
			return fmt.Errorf("obfs-opts：%w", err)
		}
		for key, value := range options {
			n.SetOption("obfs-opts."+key, value)
		}
	}
	for _, key := range []string{"ws-opts", "h2-opts", "http-opts", "grpc-opts", "httpupgrade-opts"} {
		options := mapObject(values, key)
		if options == nil {
			continue
		}
		n.SetOption(key, options)
		expected := map[string]string{"ws-opts": "ws", "h2-opts": "h2", "http-opts": "http", "grpc-opts": "grpc", "httpupgrade-opts": "httpupgrade"}[key]
		if n.TextOption("network") != expected {
			return fmt.Errorf("%s 只能用于 network: %s", key, expected)
		}
		allowed := []string{"path", "headers", "host", "grpc-service-name", "authority", "method"}
		if err := unsupportedKeys(options, allowed...); err != nil {
			return fmt.Errorf("%s：%w", key, err)
		}
		if expected == "grpc" {
			for _, unsupported := range []string{"path", "host", "authority", "method"} {
				if options[unsupported] != nil {
					return fmt.Errorf("grpc-opts.%s 目前不支持", unsupported)
				}
			}
		}
		if expected != "http" && expected != "h2" && options["method"] != nil {
			return fmt.Errorf("%s.method 仅适用于 HTTP/H2 传输", key)
		}
		if path := options["path"]; path != nil {
			if paths, ok := path.([]any); ok {
				if len(paths) != 1 {
					return fmt.Errorf("%s.path 目前只支持单一路径", key)
				}
				path = paths[0]
			}
			n.SetOption("transport-path", path)
		}
		if host := options["host"]; host != nil {
			if hosts, ok := host.([]any); ok {
				if len(hosts) == 0 {
					return fmt.Errorf("%s.host 主机名列表为空", key)
				}
			}
			n.SetOption("transport-host", host)
		}
		if service := options["grpc-service-name"]; service != nil {
			n.SetOption("service-name", service)
		}
		if method := options["method"]; method != nil {
			n.SetOption("transport-method", method)
		}
		if authority := options["authority"]; authority != nil {
			n.SetOption("transport-host", authority)
		}
		if headers := mapObject(options, "headers"); headers != nil {
			if err := validateImportedHeaders(headers); err != nil {
				return fmt.Errorf("%s.headers：%w", key, err)
			}
			if expected == "grpc" && len(headers) > 0 {
				return errors.New("gRPC 传输不支持自定义 HTTP 请求头")
			}
			n.SetOption(expected+"-opts.headers", headers)
			host, err := importedHost(headers, "Host")
			if err != nil {
				return fmt.Errorf("%s.headers：%w", key, err)
			}
			if host != nil {
				n.SetOption("transport-host", host)
			}
		}
	}
	if reality := mapObject(values, "reality-opts"); reality != nil {
		if err := unsupportedKeys(reality, "public-key", "short-id"); err != nil {
			return fmt.Errorf("reality-opts：%w", err)
		}
		if reality["public-key"] != nil {
			n.SetOption("reality-opts.public-key", reality["public-key"])
		}
		if reality["short-id"] != nil {
			n.SetOption("reality-opts.short-id", reality["short-id"])
		}
	}
	if options := mapObject(values, "xhttp-opts"); options != nil {
		if n.TextOption("network") != "xhttp" {
			return errors.New("xhttp-opts 只能用于 network: xhttp")
		}
		allowed := []string{"path", "host", "mode", "headers", "extra", "xmux", "downloadSettings", "noGRPCHeader", "xPaddingObfsMode", "xPaddingBytes", "xPaddingKey", "xPaddingHeader", "xPaddingPlacement", "xPaddingMethod", "uplinkHTTPMethod", "sessionPlacement", "sessionKey", "seqPlacement", "seqKey", "uplinkDataPlacement", "uplinkDataKey", "uplinkChunkSize", "scMaxEachPostBytes"}
		if err := unsupportedKeys(options, allowed...); err != nil {
			return fmt.Errorf("xhttp-opts：%w", err)
		}
		if path := options["path"]; path != nil {
			n.SetOption("transport-path", path)
		}
		if host := options["host"]; host != nil {
			n.SetOption("transport-host", host)
		}
		if err := importXHTTP(n, options); err != nil {
			return err
		}
	}
	return nil
}
func protocolURI(n Node) string {
	n.ID = ""
	n.Source = ""
	data, _ := json.Marshal(n)
	return "lanboat://" + base64.RawURLEncoding.EncodeToString(data)
}

func knownField(kind, key string) bool {
	n := Node{Type: kind}
	if kind == "amneziawg" {
		n.Type = "wireguard"
	}
	for _, f := range knownNodeFields(n) {
		if f.Key == key {
			return true
		}
	}
	return false
}

func validateClashRuleProvider(provider map[string]any) error {
	typ, behavior, format := mapString(provider, "type"), mapString(provider, "behavior"), mapString(provider, "format")
	if behavior == "" {
		behavior = "classical"
	}
	if behavior != "classical" && behavior != "domain" && behavior != "ipcidr" {
		return fmt.Errorf("不支持的规则集类型 %s", behavior)
	}
	if format != "" && format != "yaml" && format != "text" && format != "mrs" {
		return fmt.Errorf("不支持的规则集格式 %s", format)
	}
	if format == "mrs" && behavior == "classical" {
		return errors.New("MRS 规则集需要 domain 或 ipcidr 类型")
	}
	switch typ {
	case "inline":
		if err := unsupportedKeys(provider, "type", "behavior", "format", "payload"); err != nil {
			return err
		}
		payload, ok := provider["payload"].([]any)
		if !ok || len(payload) == 0 {
			return errors.New("内联规则集为空")
		}
		for _, item := range payload {
			if strings.TrimSpace(fmt.Sprint(item)) == "" || item == nil {
				return errors.New("内联规则集包含空规则")
			}
		}
	case "http":
		if err := unsupportedKeys(provider, "type", "behavior", "format", "url", "path", "interval", "header"); err != nil {
			return err
		}
		u, err := url.Parse(mapString(provider, "url"))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			return errors.New("HTTP 规则集需要有效的 HTTP(S) URL")
		}
		if interval := mapString(provider, "interval"); interval != "" {
			seconds, err := strconv.Atoi(interval)
			if err != nil || seconds < 1 || seconds > 31536000 {
				return errors.New("规则集 interval 必须为 1–31536000 秒")
			}
		}
		if raw := provider["header"]; raw != nil {
			headers, ok := raw.(map[string]any)
			if !ok {
				return errors.New("HTTP 规则集 header 必须是请求头对象")
			}
			if _, err := importedHTTPHeaders(headers); err != nil {
				return err
			}
		}
	case "file":
		if err := unsupportedKeys(provider, "type", "behavior", "format", "path"); err != nil {
			return err
		}
		if path := mapString(provider, "path"); path == "" || !filepath.IsAbs(path) {
			return errors.New("本地规则集 path 必须是绝对路径")
		}
	default:
		return fmt.Errorf("不支持的规则集来源 %s", typ)
	}
	return nil
}
