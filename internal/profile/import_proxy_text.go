package profile

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

var textProxyKinds = map[string]string{
	"shadowsocks": "ss", "ss": "ss", "shadowsocksr": "ssr", "ssr": "ssr",
	"vmess": "vmess", "vless": "vless", "trojan": "trojan",
	"http": "http", "https": "https", "socks5": "socks5", "socks5-tls": "socks5s",
	"hysteria2": "hysteria2", "hy2": "hysteria2", "anytls": "anytls",
	"snell": "snell", "wireguard": "wireguard", "tailscale": "tailscale",
}

// IsProxyText recognizes Quantumult X and Loon node lists before URL or YAML import.
func IsProxyText(content string) bool {
	for line := range strings.SplitSeq(strings.TrimPrefix(content, "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if proxyTextComment(line) {
			continue
		}
		switch strings.ToLower(line) {
		case "[server_local]", "[proxy]":
			return true
		}
		key, body, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		first, _, comma := strings.Cut(body, ",")
		first = strings.ToLower(strings.TrimSpace(first))
		if textProxyKinds[first] != "" && comma {
			return true
		}
		if _, _, err := net.SplitHostPort(first); err == nil && comma && textProxyKinds[strings.ToLower(strings.TrimSpace(key))] != "" {
			return true
		}
		if !strings.HasPrefix(line, "[") && (strings.HasPrefix(line, "{") || strings.HasPrefix(line, "proxies:") || strings.HasPrefix(line, "- ")) {
			return false
		}
	}
	return false
}

func proxyTextComment(line string) bool {
	return line == "" || line[0] == '#' || line[0] == ';' || strings.HasPrefix(line, "//")
}

func parseProxyText(content string, limit int, rules *[]string) ([]Node, error) {
	var nodes []Node
	section := ""
	lineNo := 0
	for line := range strings.SplitSeq(content, "\n") {
		lineNo++
		line = strings.TrimSpace(line)
		if proxyTextComment(line) {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section == "filter_local" || section == "rule" {
			if rules != nil {
				if rule := proxyTextRule(line); rule != "" {
					*rules = append(*rules, rule)
				}
			}
			continue
		}
		if section != "" && section != "server_local" && section != "proxy" {
			continue
		}
		var node Node
		var err error
		_, body, _ := strings.Cut(line, "=")
		kind, _, _ := strings.Cut(body, ",")
		scheme, _, uri := strings.Cut(line, "://")
		if textProxyKinds[strings.ToLower(strings.TrimSpace(kind))] == "" && uri && !strings.ContainsAny(scheme, " ,=") {
			node, err = ParseURI(line)
		} else {
			node, err = parseProxyTextNode(line)
		}
		if err != nil {
			return nil, fmt.Errorf("第 %d 行：%w", lineNo, err)
		}
		if node.ID == "" {
			continue
		}
		if len(nodes) == limit {
			return nil, fmt.Errorf("一次最多导入 %d 个节点", limit)
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 {
		return nil, errors.New("没有找到可导入节点")
	}
	return nodes, nil
}

// Split commas outside quoted values and structured options such as WireGuard peers.
func proxyTextFields(text string) []string {
	var fields []string
	start, depth := 0, 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(text) {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case ',':
			if depth == 0 {
				fields = append(fields, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	return append(fields, strings.TrimSpace(text[start:]))
}

func proxyTextValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[0] == raw[len(raw)-1] {
		if s, err := strconv.Unquote(raw); err == nil {
			return s
		}
		return raw[1 : len(raw)-1]
	}
	return raw
}

func proxyTextOptions(fields []string) map[string]string {
	options := make(map[string]string, len(fields))
	for _, field := range fields {
		if key, value, ok := strings.Cut(field, "="); ok {
			options[strings.ToLower(strings.TrimSpace(key))] = proxyTextValue(value)
		}
	}
	return options
}

func parseProxyTextNode(line string) (Node, error) {
	name, body, ok := strings.Cut(line, "=")
	if !ok {
		return Node{}, errors.New("节点定义缺少等号")
	}
	parts := proxyTextFields(body)
	first := strings.ToLower(proxyTextValue(parts[0]))
	if first == "direct" || first == "reject" || first == "reject-drop" {
		return Node{}, nil
	}

	n := Node{ID: ID(), Name: strings.TrimSpace(name)}
	kind := textProxyKinds[first]
	quantumult := kind == ""
	var positionals []string
	var options map[string]string
	if quantumult {
		kind = textProxyKinds[strings.ToLower(n.Name)]
		if kind == "" {
			return Node{}, errors.New("不支持的订阅节点类型")
		}
		host, port, err := net.SplitHostPort(proxyTextValue(parts[0]))
		if err != nil {
			return Node{}, errors.New("节点地址需包含服务器和端口")
		}
		n.Server, n.Port = host, parseProxyPort(port)
		options = proxyTextOptions(parts[1:])
		n.Name = options["tag"]
	} else {
		var fields []string
		if kind == "wireguard" || kind == "tailscale" {
			fields = parts[1:]
		} else {
			if len(parts) < 3 {
				return Node{}, errors.New("节点缺少服务器或端口")
			}
			n.Server, n.Port = strings.Trim(proxyTextValue(parts[1]), "[]"), parseProxyPort(proxyTextValue(parts[2]))
			fields = parts[3:]
		}
		i := 0
		for i < len(fields) && (!strings.Contains(fields[i], "=") || strings.HasPrefix(fields[i], `"`) || strings.HasPrefix(fields[i], "'")) {
			positionals = append(positionals, proxyTextValue(fields[i]))
			i++
		}
		options = proxyTextOptions(fields[i:])
	}
	pos := func(i int) string {
		if i < len(positionals) {
			return positionals[i]
		}
		return ""
	}
	password := options["password"]
	method := options["method"]
	switch kind {
	case "ss", "ssr":
		n.Method = firstProxyValue(pos(0), method)
		n.Password = firstProxyValue(pos(1), password)
		if options["ssr-protocol"] != "" {
			kind = "ssr"
		}
		if kind == "ssr" {
			n.SetOption("cipher", n.Method)
		}
	case "vmess":
		n.SetOption("uuid", firstProxyValue(pos(1), password))
		cipher := firstProxyValue(pos(0), method)
		if cipher == "chacha20-ietf-poly1305" {
			cipher = "chacha20-poly1305"
		}
		if cipher != "" {
			n.SetOption("cipher", cipher)
		}
	case "vless":
		n.SetOption("uuid", firstProxyValue(pos(0), password))
	case "socks5", "socks5s", "http", "https":
		n.SetOption("username", firstProxyValue(pos(0), options["username"]))
		n.Password = firstProxyValue(pos(1), password)
		if options["over-tls"] == "true" {
			if kind == "http" {
				kind = "https"
			} else if kind == "socks5" {
				kind = "socks5s"
			}
		}
	case "wireguard":
		if err := importTextWireGuard(&n, options); err != nil {
			return Node{}, err
		}
	case "tailscale":
		if options["exit-node"] == "auto" {
			return Node{}, errors.New("暂不支持自动选择 Tailscale 出口节点")
		}
	default:
		n.Password = firstProxyValue(pos(0), firstProxyValue(password, options["psk"]))
	}
	n.Type = kind
	if n.Name == "" {
		n.Name = n.Server
	}
	copyOptions := map[string]string{
		"fast-open": "tfo", "udp": "udp", "udp-relay": "udp", "transport": "network",
		"host": "transport-host", "path": "transport-path", "sni": "sni", "tls-host": "sni",
		"skip-cert-verify": "skip-cert-verify", "tls-cert-sha256": "tls-sha256",
		"tls-pubkey-sha256": "tls-pubkey-sha256", "alpn": "alpn", "alterid": "alterId",
		"public-key": "reality-opts.public-key", "reality-base64-pubkey": "reality-opts.public-key",
		"short-id": "reality-opts.short-id", "reality-hex-shortid": "reality-opts.short-id",
		"flow": "flow", "vless-flow": "flow", "protocol": "protocol", "ssr-protocol": "protocol",
		"protocol-param": "protocol-param", "ssr-protocol-param": "protocol-param", "obfs-param": "obfs-param",
		"version": "version", "server-ports": "ports", "download-bandwidth": "down",
		"upload-bandwidth": "up", "idle-session-timeout": "idle-timeout",
		"auth-key": "auth-key", "hostname": "hostname", "control-url": "control-url", "exit-node": "exit-node",
	}
	for key, field := range copyOptions {
		if value := options[key]; value != "" && !(kind == "wireguard" && key == "public-key") {
			n.SetOption(field, value)
		}
	}

	if kind == "hysteria2" {
		for _, field := range []string{"up", "down"} {
			if n.TextOption(field) == "0" {
				n.SetOption(field, "auto")
			}
		}
		if n.TextOption("ports") != "" {
			if seconds, err := strconv.Atoi(options["hop-interval"]); err == nil && seconds > 0 {
				n.SetOption("hop-interval", options["hop-interval"]+"s")
			}
		}
	}

	if options["tls-verification"] != "" {
		n.SetOption("skip-cert-verify", options["tls-verification"] == "false")
	}
	if name := options["tls-profile"]; name == "chrome" || name == "safari" || name == "firefox" {
		n.SetOption("client-fingerprint", name)
	}
	switch options["udp-over-tcp"] {
	case "true", "sp.v1":
		n.SetOption("uot-version", "1")
	case "sp.v2":
		n.SetOption("uot-version", "2")
	}
	if secret := options["salamander-password"]; secret != "" {
		n.SetOption("obfs", "salamander")
		n.SetOption("obfs-password", secret)
	}
	if kind == "vmess" || kind == "vless" || kind == "trojan" || kind == "ss" {
		importTextTransport(&n, options, quantumult)
	}
	if kind == "ssr" || kind == "snell" {
		if obfs := firstProxyValue(options["obfs-name"], options["obfs"]); obfs != "" {
			if kind == "ssr" {
				n.SetOption("obfs", obfs)
				if options["obfs-param"] == "" && options["obfs-host"] != "" {
					n.SetOption("obfs-param", options["obfs-host"])
				}
			} else {
				n.SetOption("obfs-opts.mode", obfs)
				n.SetOption("obfs-opts.host", options["obfs-host"])
				n.SetOption("obfs-opts.path", options["obfs-uri"])
			}
		}
		if kind == "snell" {
			if options["version"] == "" {
				n.SetOption("version", "4")
			}
			put(&n, "mode", options["mode"])
			put(&n, "mux", options["reuse"])
		}
	}
	if secret := options["shadow-tls-password"]; secret != "" {
		n.Plugin = "shadow-tls"
		n.SetOption("plugin-opts.password", secret)
		n.SetOption("plugin-opts.host", firstProxyValue(options["shadow-tls-sni"], n.Server))
		n.SetOption("plugin-opts.version", firstProxyValue(options["shadow-tls-version"], "3"))
	}
	applyDefaults(&n)
	return n, n.Validate()
}

func importTextTransport(n *Node, options map[string]string, quantumult bool) {
	mode := firstProxyValue(options["obfs-name"], options["obfs"])
	host, path := options["obfs-host"], options["obfs-uri"]
	tls := options["over-tls"] == "true" || mode == "over-tls" || mode == "wss" || n.TextOption("reality-opts.public-key") != ""
	if tls || n.Kind() == "vmess" || n.Kind() == "vless" {
		n.SetOption("tls", tls)
	}
	if n.TextOption("sni") == "" && tls && host != "" {
		n.SetOption("sni", host)
	}
	if n.Kind() == "ss" {
		if mode != "" && mode != "over-tls" {
			n.SetOption("obfs", mode)
			n.SetOption("obfs-host", host)
			n.SetOption("obfs-path", path)
		}
		return
	}
	if quantumult && (mode == "ws" || mode == "wss" || mode == "http") {
		if mode == "wss" {
			mode = "ws"
		}
		n.SetOption("network", mode)
		n.SetOption("transport-host", host)
		n.SetOption("transport-path", path)
	}
}

func importTextWireGuard(n *Node, options map[string]string) error {
	peers := strings.TrimSpace(options["peers"])
	if !strings.HasPrefix(peers, "[{") || !strings.HasSuffix(peers, "}]") ||
		len(proxyTextFields(peers[1:len(peers)-1])) != 1 {
		return errors.New("WireGuard 订阅节点需要一个对端")
	}
	peer := proxyTextOptions(proxyTextFields(peers[2 : len(peers)-2]))
	host, port, err := net.SplitHostPort(peer["endpoint"])
	if err != nil {
		return errors.New("WireGuard 对端地址无效")
	}
	for _, allowed := range strings.Split(peer["allowed-ips"], ",") {
		if allowed == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(strings.TrimSpace(allowed))
		if err != nil || prefix.Bits() != 0 {
			return errors.New("WireGuard 订阅节点需要默认路由对端")
		}
	}
	n.Server, n.Port = host, parseProxyPort(port)
	for key, value := range map[string]string{
		"ip": options["interface-ip"], "ipv6": options["interface-ipv6"], "private-key": options["private-key"],
		"mtu": firstProxyValue(options["mtu"], "1280"), "dns": options["dns"], "heartbeat": firstProxyValue(options["keepalive"], "45"),
		"public-key": peer["public-key"], "pre-shared-key": peer["preshared-key"], "reserved": strings.Trim(peer["reserved"], "[]"),
	} {
		if value != "" {
			n.SetOption(key, value)
		}
	}
	return nil
}

func parseProxyPort(value string) int {
	port, _ := strconv.Atoi(value)
	return port
}

func firstProxyValue(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func proxyTextRule(line string) string {
	fields := strings.SplitN(line, ",", 3)
	kind := strings.ToUpper(strings.TrimSpace(fields[0]))
	switch kind {
	case "HOST":
		fields[0] = "DOMAIN"
	case "HOST-SUFFIX":
		fields[0] = "DOMAIN-SUFFIX"
	case "HOST-KEYWORD":
		fields[0] = "DOMAIN-KEYWORD"
	case "IP6-CIDR":
		fields[0] = "IP-CIDR6"
	case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DOMAIN-REGEX", "IP-CIDR", "IP-CIDR6", "GEOIP", "USER-AGENT", "FINAL", "MATCH":
	default:
		return ""
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return strings.Join(fields, ",")
}
