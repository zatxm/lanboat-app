package profile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

func mapString(m map[string]any, key string) string {
	if v := m[key]; v != nil {
		return fmt.Sprint(v)
	}
	return ""
}
func mapObject(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}
func mapList(m map[string]any, key string) []any { v, _ := m[key].([]any); return v }
func put(n *Node, key string, v any) {
	if v != nil && fmt.Sprint(v) != "" {
		n.SetOption(key, v)
	}
}

func unsupportedKeys(m map[string]any, allowed ...string) error {
	valid := map[string]bool{}
	for _, key := range allowed {
		valid[key] = true
	}
	for key, value := range m {
		if !valid[key] && value != nil {
			return fmt.Errorf("暂不支持配置项 %s", key)
		}
	}
	return nil
}

func validateImportedHeaders(headers map[string]any) error {
	for key, raw := range headers {
		if !validImportedHeaderName(key) {
			return fmt.Errorf("HTTP 请求头名称无效：%s", key)
		}
		values, err := importedHeaderValues(raw)
		if err != nil {
			return fmt.Errorf("HTTP 请求头 %s：%w", key, err)
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("HTTP 请求头 %s 含无效字符", key)
			}
		}
	}
	return nil
}

func validImportedHeaderName(key string) bool {
	if key == "" || http.CanonicalHeaderKey(key) == "" {
		return false
	}
	for _, r := range key {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}

func importedHeaderValues(raw any) ([]string, error) {
	switch value := raw.(type) {
	case string:
		return []string{value}, nil
	case []string:
		return value, nil
	case []any:
		values := make([]string, len(value))
		for i, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, errors.New("值必须是文本")
			}
			values[i] = text
		}
		return values, nil
	default:
		return nil, errors.New("值必须是文本或文本列表")
	}
}

func importedHost(headers map[string]any, key string) (any, error) {
	var raw any
	found := false
	for name, value := range headers {
		if http.CanonicalHeaderKey(name) == http.CanonicalHeaderKey(key) {
			raw, found = value, true
			break
		}
	}
	if !found {
		return nil, nil
	}
	values, err := importedHeaderValues(raw)
	if err != nil || len(values) == 0 {
		return nil, fmt.Errorf("%s 必须包含主机名", key)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("%s 主机名无效", key)
		}
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return values, nil
}

func importedHTTPHeaders(headers map[string]any) (map[string][]string, error) {
	if err := validateImportedHeaders(headers); err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(headers))
	for key, raw := range headers {
		values, _ := importedHeaderValues(raw)
		canonical := http.CanonicalHeaderKey(key)
		result[canonical] = append(result[canonical], values...)
	}
	return result, nil
}

// NormalizeHTTPHeaders validates and canonicalizes HTTP headers in imported profiles.
func NormalizeHTTPHeaders(headers map[string]any) (map[string][]string, error) {
	return importedHTTPHeaders(headers)
}

func parseOutboundConfig(doc map[string]any, outbounds []any, limit int, rules *[]string) ([]Node, error) {
	var nodes []Node
	aliases := map[string]string{}
	var groupDocs []any
	if rules != nil {
		for _, item := range outbounds {
			o, ok := item.(map[string]any)
			if !ok {
				continue
			}
			kind, tag := mapString(o, "type"), mapString(o, "tag")
			if kind == "" {
				kind = mapString(o, "protocol")
			}
			switch kind {
			case "direct", "freedom":
				aliases[tag] = "DIRECT"
			case "block", "blackhole":
				aliases[tag] = "REJECT"
			case "selector", "urltest":
				if err := unsupportedKeys(o, "type", "tag", "outbounds", "default", "url", "interval", "tolerance"); err != nil {
					return nil, err
				}
				typ := "select"
				if kind == "urltest" {
					typ = "url-test"
				}
				group := map[string]any{"name": tag, "type": typ, "proxies": mapList(o, "outbounds"), "url": o["url"], "tolerance": o["tolerance"], "default": o["default"]}
				if interval := mapString(o, "interval"); interval != "" {
					d, e := time.ParseDuration(interval)
					if e != nil || d < time.Second || d%time.Second != 0 {
						return nil, fmt.Errorf("分组 %s 更新间隔无效", tag)
					}
					group["interval"] = int(d / time.Second)
				}
				groupDocs = append(groupDocs, group)
			}
		}
		for _, item := range groupDocs {
			g := item.(map[string]any)
			members := mapList(g, "proxies")
			for i, v := range members {
				if alias := aliases[fmt.Sprint(v)]; alias != "" {
					members[i] = alias
				}
			}
		}
		groupNames, err := clashGroups(map[string]any{"proxy-groups": groupDocs}, rules)
		if err != nil {
			return nil, err
		}
		for name, mapped := range groupNames {
			aliases[name] = mapped
		}
	}
	for i, item := range outbounds {
		o, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("出站 %d 格式无效", i+1)
		}
		kind := mapString(o, "type")
		if kind == "" {
			kind = mapString(o, "protocol")
		}
		switch kind {
		case "direct", "block", "dns", "selector", "urltest", "freedom", "blackhole":
			continue
		}
		name := mapString(o, "tag")
		if mapString(o, "protocol") != "" {
			converted, err := parseV2Outbound(o, name)
			if err != nil {
				return nil, fmt.Errorf("出站 %d：%w", i+1, err)
			}
			nodes = append(nodes, converted...)
		} else {
			node, err := parseSingOutbound(o, name)
			if err != nil {
				return nil, fmt.Errorf("出站 %d：%w", i+1, err)
			}
			nodes = append(nodes, node)
		}
		if len(nodes) > limit {
			return nil, fmt.Errorf("一次最多导入 %d 个节点", limit)
		}
	}
	if len(nodes) == 0 {
		return nil, errors.New("配置中没有可导入的代理出站")
	}
	if rules != nil {
		if r := mapObject(doc, "route"); r != nil {
			if err := parseNativeRoute(r, false, rules, aliases); err != nil {
				return nil, err
			}
		}
		if r := mapObject(doc, "routing"); r != nil {
			if err := parseNativeRoute(r, true, rules, aliases); err != nil {
				return nil, err
			}
		}
	}
	return nodes, nil
}

func parseSingOutbound(o map[string]any, name string) (Node, error) {
	if err := unsupportedKeys(o, "type", "tag", "server", "server_port", "password", "method", "uuid", "security", "alter_id", "flow", "username", "congestion_control", "up_mbps", "down_mbps", "plugin", "tls", "transport", "obfs"); err != nil {
		return Node{}, err
	}
	kind := strings.ToLower(mapString(o, "type"))
	switch kind {
	case "shadowsocks":
		kind = "ss"
	case "socks":
		kind = "socks5"
	case "hy2":
		kind = "hysteria2"
	}
	if _, ok := ProtocolByID(kind); !ok {
		return Node{}, fmt.Errorf("不支持的出站类型 %s", kind)
	}
	if mapString(o, "detour") != "" {
		return Node{}, errors.New("暂不支持导入带 detour 的出站")
	}
	server := mapString(o, "server")
	port, _ := strconv.Atoi(mapString(o, "server_port"))
	if name == "" {
		name = server
	}
	n := Node{ID: ID(), Type: kind, Name: name, Server: server, Port: port, Password: mapString(o, "password")}
	if kind == "ss" {
		n.Method = mapString(o, "method")
	}
	for source, dest := range map[string]string{"uuid": "uuid", "security": "cipher", "alter_id": "alterId", "flow": "flow", "username": "username", "congestion_control": "congestion-controller", "up_mbps": "up", "down_mbps": "down", "plugin": "plugin"} {
		if source == "plugin" {
			n.Plugin = mapString(o, source)
		} else {
			put(&n, dest, o[source])
		}
	}
	if kind == "vless" || kind == "vmess" {
		n.SetOption("tls", false)
	}
	if tls := mapObject(o, "tls"); tls != nil {
		if err := unsupportedKeys(tls, "enabled", "server_name", "insecure", "alpn", "utls", "reality"); err != nil {
			return Node{}, err
		}
		if tls["certificate"] != nil || tls["certificate_path"] != nil || tls["client_certificate"] != nil || tls["client_certificate_path"] != nil {
			return Node{}, errors.New("暂不支持导入自定义 TLS 证书")
		}
		put(&n, "tls", tls["enabled"])
		put(&n, "sni", tls["server_name"])
		put(&n, "skip-cert-verify", tls["insecure"])
		put(&n, "alpn", tls["alpn"])
		if utls := mapObject(tls, "utls"); utls != nil {
			if err := unsupportedKeys(utls, "enabled", "fingerprint"); err != nil {
				return Node{}, err
			}
			if fmt.Sprint(utls["enabled"]) != "false" {
				put(&n, "client-fingerprint", utls["fingerprint"])
			}
		}
		if reality := mapObject(tls, "reality"); reality != nil {
			if err := unsupportedKeys(reality, "enabled", "public_key", "short_id"); err != nil {
				return Node{}, err
			}
			put(&n, "reality-opts.public-key", reality["public_key"])
			put(&n, "reality-opts.short-id", reality["short_id"])
		}
	}
	if n.TextOption("tls") == "true" {
		if n.Kind() == "socks5" {
			n.Type = "socks5s"
		}
		if n.Kind() == "http" {
			n.Type = "https"
		}
	}
	if tr := mapObject(o, "transport"); tr != nil {
		if err := unsupportedKeys(tr, "type", "path", "service_name", "headers", "host", "method"); err != nil {
			return Node{}, err
		}
		network := mapString(tr, "type")
		if network != "ws" && network != "websocket" && network != "http" && network != "httpupgrade" && network != "grpc" {
			return Node{}, fmt.Errorf("不支持的传输方式 %s", network)
		}
		if network == "websocket" {
			network = "ws"
		}
		if network == "http" {
			network = "http"
		}
		put(&n, "network", network)
		put(&n, "transport-path", tr["path"])
		put(&n, "transport-method", tr["method"])
		put(&n, "service-name", tr["service_name"])
		if headers := mapObject(tr, "headers"); headers != nil {
			if err := validateImportedHeaders(headers); err != nil {
				return Node{}, err
			}
			if network == "grpc" && len(headers) > 0 {
				return Node{}, errors.New("gRPC 传输不支持自定义 HTTP 请求头")
			}
			n.SetOption(network+"-opts.headers", headers)
			host, err := importedHost(headers, "Host")
			if err != nil {
				return Node{}, err
			}
			put(&n, "transport-host", host)
		}
		if hosts := tr["host"]; hosts != nil {
			put(&n, "transport-host", hosts)
		}
	}
	if obfs := mapObject(o, "obfs"); obfs != nil {
		if err := unsupportedKeys(obfs, "type", "password"); err != nil {
			return Node{}, err
		}
		if n.Kind() == "hysteria2" {
			if mapString(obfs, "type") != "salamander" {
				return Node{}, fmt.Errorf("不支持的 Hysteria2 混淆类型 %s", mapString(obfs, "type"))
			}
			put(&n, "obfs", "salamander")
			put(&n, "obfs-password", obfs["password"])
		} else {
			return Node{}, fmt.Errorf("不支持的 %s 混淆配置", n.Kind())
		}
	}
	applyDefaults(&n)
	return n, n.Validate()
}

func parseV2Outbound(o map[string]any, name string) ([]Node, error) {
	if err := unsupportedKeys(o, "protocol", "tag", "settings", "streamSettings"); err != nil {
		return nil, err
	}
	kind := strings.ToLower(mapString(o, "protocol"))
	switch kind {
	case "shadowsocks":
		kind = "ss"
	case "socks":
		kind = "socks5"
	}
	if _, ok := ProtocolByID(kind); !ok {
		return nil, fmt.Errorf("不支持的 V2Ray 出站类型 %s", kind)
	}
	settings := mapObject(o, "settings")
	if err := unsupportedKeys(settings, "vnext", "servers"); err != nil {
		return nil, err
	}
	servers := mapList(settings, "vnext")
	if len(servers) == 0 {
		servers = mapList(settings, "servers")
	}
	if len(servers) == 0 {
		return nil, errors.New("V2Ray 出站没有服务器")
	}
	var nodes []Node
	for _, v := range servers {
		server, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("V2Ray 服务器格式无效")
		}
		if err := unsupportedKeys(server, "address", "port", "users", "password", "method", "email", "level"); err != nil {
			return nil, err
		}
		users := mapList(server, "users")
		if len(users) == 0 {
			users = []any{map[string]any{}}
		}
		for _, entry := range users {
			user, ok := entry.(map[string]any)
			if !ok {
				return nil, errors.New("V2Ray 用户格式无效")
			}
			if err := unsupportedKeys(user, "id", "alterId", "security", "flow", "user", "pass", "encryption", "email", "level"); err != nil {
				return nil, err
			}
			addr := mapString(server, "address")
			port, _ := strconv.Atoi(mapString(server, "port"))
			nodeName := name
			if nodeName == "" {
				nodeName = addr
			}
			n := Node{ID: ID(), Type: kind, Name: nodeName, Server: addr, Port: port, Password: mapString(server, "password")}
			n.Method = mapString(server, "method")
			if n.Password == "" {
				n.Password = mapString(user, "pass")
			}
			for source, dest := range map[string]string{"id": "uuid", "alterId": "alterId", "security": "cipher", "flow": "flow", "user": "username", "encryption": "encryption"} {
				put(&n, dest, user[source])
			}
			stream := mapObject(o, "streamSettings")
			network := mapString(stream, "network")
			if network == "raw" {
				network = "tcp"
			}
			if err := unsupportedKeys(stream, "network", "security", "tlsSettings", "realitySettings", network+"Settings"); err != nil {
				return nil, err
			}
			if security := mapString(stream, "security"); security != "" && security != "none" && security != "tls" && security != "reality" {
				return nil, fmt.Errorf("不支持的传输安全类型 %s", security)
			}
			put(&n, "network", network)
			if kind == "vless" || kind == "vmess" {
				n.SetOption("tls", false)
			}
			if mapString(stream, "security") == "tls" || mapString(stream, "security") == "reality" {
				n.SetOption("tls", true)
			}
			for _, key := range []string{"tlsSettings", "realitySettings"} {
				if tls := mapObject(stream, key); tls != nil {
					if err := unsupportedKeys(tls, "serverName", "allowInsecure", "alpn", "fingerprint", "publicKey", "shortId"); err != nil {
						return nil, err
					}
					put(&n, "sni", tls["serverName"])
					put(&n, "skip-cert-verify", tls["allowInsecure"])
					put(&n, "alpn", tls["alpn"])
					put(&n, "client-fingerprint", tls["fingerprint"])
					put(&n, "reality-opts.public-key", tls["publicKey"])
					put(&n, "reality-opts.short-id", tls["shortId"])
				}
			}
			transport := mapObject(stream, network+"Settings")
			if transport != nil {
				if network == "xhttp" {
					if err := importXHTTP(&n, transport); err != nil {
						return nil, err
					}
				} else if err := unsupportedKeys(transport, "path", "host", "serviceName", "headers", "method"); err != nil {
					return nil, err
				}
				if transport["method"] != nil && network != "http" && network != "h2" {
					return nil, fmt.Errorf("%sSettings.method 仅适用于 HTTP/H2 传输", network)
				}
				if network == "grpc" {
					for _, unsupported := range []string{"path", "host", "headers", "method"} {
						if transport[unsupported] != nil {
							return nil, fmt.Errorf("grpcSettings.%s 目前不支持", unsupported)
						}
					}
				}
				put(&n, "transport-path", transport["path"])
				put(&n, "transport-host", transport["host"])
				put(&n, "transport-method", transport["method"])
				put(&n, "service-name", transport["serviceName"])
				if headers := mapObject(transport, "headers"); headers != nil {
					if err := validateImportedHeaders(headers); err != nil {
						return nil, err
					}
					if network == "grpc" && len(headers) > 0 {
						return nil, errors.New("gRPC 传输不支持自定义 HTTP 请求头")
					}
					n.SetOption(network+"-opts.headers", headers)
					host, err := importedHost(headers, "Host")
					if err != nil {
						return nil, err
					}
					put(&n, "transport-host", host)
				}
			}
			if n.Kind() == "ss" && network != "" && network != "tcp" {
				if network != "xhttp" {
					return nil, fmt.Errorf("Shadowsocks 暂不支持导入传输方式 %s", network)
				}
				n.SetOption("obfs", "xhttp")
				n.SetOption("obfs-host", n.TextOption("transport-host"))
				n.SetOption("obfs-path", n.TextOption("transport-path"))
				n.SetOption("xhttp-mode", n.TextOption("xhttp-opts.mode"))
				n.SetOption("obfs-tls", n.TextOption("tls") == "true")
			}
			applyDefaults(&n)
			if err := n.Validate(); err != nil {
				return nil, err
			}
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func firstConditionKey(r map[string]any) string {
	for key := range r {
		if key != "outbound" && key != "outboundTag" && key != "type" && key != "action" {
			return key
		}
	}
	return ""
}

func convertV2Conditions(r map[string]any) (map[string]any, error) {
	var all []any
	for key, value := range r {
		if key == "type" || key == "outboundTag" {
			continue
		}
		values, ok := value.([]any)
		if !ok {
			values = []any{value}
		}
		condition := map[string]any{}
		for _, v := range values {
			text := fmt.Sprint(v)
			field := ""
			switch key {
			case "domain":
				field = "domain_keyword"
				for _, pair := range []struct{ prefix, field string }{{"full:", "domain"}, {"domain:", "domain_suffix"}, {"regexp:", "domain_regex"}} {
					if strings.HasPrefix(text, pair.prefix) {
						field, text = pair.field, strings.TrimPrefix(text, pair.prefix)
						break
					}
				}
				if strings.HasPrefix(text, "geosite:") || field == "domain_keyword" && strings.Contains(text, ":") {
					return nil, fmt.Errorf("不支持的域名集 %s", text)
				}
			case "ip":
				field = "ip_cidr"
				if ip, err := netip.ParseAddr(text); err == nil {
					text = netip.PrefixFrom(ip, ip.BitLen()).String()
				}
				if _, err := netip.ParsePrefix(text); err != nil {
					return nil, fmt.Errorf("不支持的 IP 条件 %s", text)
				}
			case "network":
				field = "network"
				for _, part := range strings.Split(text, ",") {
					condition[field] = appendValue(condition[field], strings.TrimSpace(part))
				}
				continue
			case "port":
				for _, part := range strings.Split(text, ",") {
					part = strings.TrimSpace(part)
					if strings.Contains(part, "-") {
						condition["port_range"] = appendValue(condition["port_range"], strings.Replace(part, "-", ":", 1))
						continue
					}
					n, err := strconv.Atoi(part)
					if err != nil || n < 1 || n > 65535 {
						return nil, fmt.Errorf("端口无效 %s", part)
					}
					condition["port"] = appendValue(condition["port"], n)
				}
				continue
			default:
				return nil, fmt.Errorf("不支持的条件 %s", key)
			}
			condition[field] = appendValue(condition[field], text)
		}
		all = append(all, condition)
	}
	if len(all) == 0 {
		return nil, errors.New("路由规则没有条件")
	}
	if len(all) == 1 {
		return all[0].(map[string]any), nil
	}
	return map[string]any{"type": "logical", "mode": "and", "rules": all}, nil
}

func appendValue(values any, value any) []any {
	list, _ := values.([]any)
	return append(list, value)
}

func parseNativeRoute(route map[string]any, v2 bool, out *[]string, aliases map[string]string) error {
	fields := map[string]string{"domain": "DOMAIN", "domain_suffix": "DOMAIN-SUFFIX", "domain_keyword": "DOMAIN-KEYWORD", "domain_regex": "DOMAIN-REGEX", "ip_cidr": "IP-CIDR", "port": "DST-PORT", "network": "NETWORK"}
	for i, item := range mapList(route, "rules") {
		r, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("路由规则 %d 格式无效", i+1)
		}
		if !v2 && mapString(r, "action") != "" && mapString(r, "action") != "route" && mapString(r, "action") != "reject" {
			return fmt.Errorf("路由规则 %d 不支持的操作 %s", i+1, mapString(r, "action"))
		}
		target := mapString(r, "outbound")
		if !v2 && mapString(r, "action") == "reject" {
			target = "REJECT"
		}
		if v2 {
			target = mapString(r, "outboundTag")
		}
		if target == "" {
			return fmt.Errorf("路由规则 %d 不支持的操作", i+1)
		}
		if mapped := aliases[target]; mapped != "" {
			target = mapped
		}
		action := target
		switch strings.ToLower(target) {
		case "direct", "freedom":
			action = "DIRECT"
		case "block", "reject", "blackhole":
			action = "REJECT"
		default:
			action = target
		}
		conditions := 0
		for key := range r {
			if key != "outbound" && key != "outboundTag" && key != "type" && key != "action" {
				conditions++
			}
		}
		if !v2 && conditions == 1 && firstConditionKey(r) == "rule_set" {
			refs, ok := r["rule_set"].([]any)
			if !ok {
				refs = []any{r["rule_set"]}
			}
			for _, ref := range refs {
				var found map[string]any
				for _, item := range mapList(route, "rule_set") {
					set, _ := item.(map[string]any)
					if mapString(set, "tag") == fmt.Sprint(ref) {
						found = set
						break
					}
				}
				if found == nil {
					return fmt.Errorf("路由规则 %d 的规则集 %s 不存在", i+1, ref)
				}
				if mapString(found, "download_detour") != "" {
					return fmt.Errorf("规则集 %s 的下载代理暂不支持", ref)
				}
				switch mapString(found, "type") {
				case "inline":
					condition := map[string]any{"type": "logical", "mode": "or", "rules": mapList(found, "rules")}
					data, err := json.Marshal(condition)
					if err != nil {
						return err
					}
					*out = append(*out, "NATIVE,"+base64.RawURLEncoding.EncodeToString(data)+","+action)
				case "local", "remote":
					address := mapString(found, "path")
					if mapString(found, "type") == "remote" {
						address = mapString(found, "url")
					}
					kind := "RULE-SET"
					if mapString(found, "format") == "binary" {
						kind = "SRS-SET"
					}
					*out = append(*out, kind+","+address+","+action)
				default:
					return fmt.Errorf("不支持的规则集类型 %s", mapString(found, "type"))
				}
			}
			continue
		}
		_, simple := fields[firstConditionKey(r)]
		if v2 && firstConditionKey(r) == "ip" {
			simple = true
		}
		complexV2 := v2 && (firstConditionKey(r) == "port" && strings.ContainsAny(fmt.Sprint(r["port"]), ",-") || firstConditionKey(r) == "network" && strings.Contains(fmt.Sprint(r["network"]), ","))
		if conditions != 1 || !simple || complexV2 || (!v2 && mapString(r, "type") == "logical") {
			var condition map[string]any
			var err error
			if v2 {
				condition, err = convertV2Conditions(r)
			} else {
				condition = make(map[string]any, len(r))
				for key, value := range r {
					if key != "outbound" && key != "action" {
						condition[key] = value
					}
				}
			}
			if err != nil {
				return fmt.Errorf("路由规则 %d：%w", i+1, err)
			}
			data, err := json.Marshal(condition)
			if err != nil {
				return err
			}
			*out = append(*out, "NATIVE,"+base64.RawURLEncoding.EncodeToString(data)+","+action)
			continue
		}
		for key, value := range r {
			if key == "outbound" || key == "outboundTag" || key == "type" || key == "action" {
				continue
			}
			kind, ok := fields[key]
			if v2 && key == "ip" {
				kind, ok = "IP-CIDR", true
			}
			if !ok {
				return fmt.Errorf("路由规则 %d 不支持 %s", i+1, key)
			}
			values, ok := value.([]any)
			if !ok {
				values = []any{value}
			}
			for _, v := range values {
				text := fmt.Sprint(v)
				k := kind
				if v2 && key == "domain" {
					switch {
					case strings.HasPrefix(text, "full:"):
						text = strings.TrimPrefix(text, "full:")
					case strings.HasPrefix(text, "domain:"):
						k = "DOMAIN-SUFFIX"
						text = strings.TrimPrefix(text, "domain:")
					case strings.HasPrefix(text, "regexp:"):
						k = "DOMAIN-REGEX"
						text = strings.TrimPrefix(text, "regexp:")
					case strings.HasPrefix(text, "geosite:"):
						k = "GEOSITE"
						text = strings.TrimPrefix(text, "geosite:")
					case strings.Contains(text, ":"):
						return fmt.Errorf("路由规则 %d 不支持域名集 %s", i+1, text)
					default:
						k = "DOMAIN-KEYWORD"
					}
				}
				if v2 && key == "ip" && strings.HasPrefix(text, "geoip:") {
					k = "GEOIP"
					text = strings.TrimPrefix(text, "geoip:")
				}
				if k == "IP-CIDR" {
					if ip, err := netip.ParseAddr(text); err == nil {
						text = netip.PrefixFrom(ip, ip.BitLen()).String()
					}
				}
				if k == "IP-CIDR" && strings.Contains(text, ":") {
					k = "IP-CIDR6"
				}
				*out = append(*out, k+","+text+","+action)
			}
		}
	}
	if final := mapString(route, "final"); final != "" {
		if mapped := aliases[final]; mapped != "" {
			final = mapped
		}
		*out = append(*out, "FINAL,"+final)
	}
	return nil
}

func importXHTTP(n *Node, transport map[string]any) error {
	names := map[string]string{"noGRPCHeader": "NoGRPCHeader", "xPaddingObfsMode": "XPaddingObfsMode", "xPaddingBytes": "XPaddingBytes", "xPaddingKey": "XPaddingKey", "xPaddingHeader": "XPaddingHeader", "xPaddingPlacement": "XPaddingPlacement", "xPaddingMethod": "XPaddingMethod", "uplinkHTTPMethod": "UplinkHTTPMethod", "sessionPlacement": "SessionPlacement", "sessionIDPlacement": "SessionPlacement", "sessionKey": "SessionKey", "sessionIDKey": "SessionKey", "seqPlacement": "SeqPlacement", "seqKey": "SeqKey", "uplinkDataPlacement": "UplinkDataPlacement", "uplinkDataKey": "UplinkDataKey", "uplinkChunkSize": "UplinkChunkSize", "scMaxEachPostBytes": "ScMaxEachPostBytes"}
	allowed := []string{"path", "host", "mode", "headers", "extra", "xmux", "downloadSettings"}
	for k := range names {
		allowed = append(allowed, k)
	}
	read := func(m map[string]any) error {
		for key, value := range m {
			if target := names[key]; target != "" {
				put(n, "xhttp."+target, value)
				continue
			}
			switch key {
			case "headers":
				headers, ok := value.(map[string]any)
				if !ok {
					return errors.New("XHTTP headers 必须是对象")
				}
				var existing map[string]any
				if text := n.TextOption("xhttp-extra"); text != "" {
					_ = json.Unmarshal([]byte(text), &existing)
				}
				if existing == nil {
					existing = map[string]any{}
				}
				existing["headers"] = headers
				data, err := json.Marshal(existing)
				if err != nil {
					return err
				}
				n.SetOption("xhttp-extra", string(data))
			case "path", "host", "mode", "extra", "xmux", "downloadSettings":
			default:
				return fmt.Errorf("不支持的 XHTTP 参数 %s", key)
			}
		}
		return nil
	}
	if err := unsupportedKeys(transport, allowed...); err != nil {
		return err
	}
	if err := read(transport); err != nil {
		return err
	}
	extra := mapObject(transport, "extra")
	if err := read(extra); err != nil {
		return err
	}
	download := mapObject(transport, "downloadSettings")
	if inner := mapObject(extra, "downloadSettings"); inner != nil {
		download = inner
	}
	if download != nil {
		if err := unsupportedKeys(download, "address", "port", "network", "security", "tlsSettings", "xhttpSettings"); err != nil {
			return err
		}
		if network := mapString(download, "network"); network != "" && network != "xhttp" {
			return fmt.Errorf("不支持的 XHTTP 下载网络 %s", network)
		}
		security := mapString(download, "security")
		if security == "" {
			security = "none"
		}
		if (security == "tls") != (n.TextOption("tls") == "true") {
			return fmt.Errorf("XHTTP 下载安全类型必须与上传一致")
		}
		if security != "" && security != "tls" && security != "none" {
			return fmt.Errorf("不支持的 XHTTP 下载安全类型 %s", security)
		}
		put(n, "xhttp.DownloadTargetHost", download["address"])
		put(n, "xhttp.DownloadTargetPort", download["port"])
		if tls := mapObject(download, "tlsSettings"); tls != nil {
			if err := unsupportedKeys(tls, "serverName"); err != nil {
				return err
			}
			put(n, "xhttp.DownloadServerName", tls["serverName"])
		}
		if settings := mapObject(download, "xhttpSettings"); settings != nil {
			if err := unsupportedKeys(settings, "path", "host"); err != nil {
				return err
			}
			if p := mapString(settings, "path"); p != "" && p != mapString(transport, "path") {
				return errors.New("XHTTP 上传和下载路径必须一致")
			}
			put(n, "xhttp.DownloadHTTPHost", settings["host"])
		}
	}
	put(n, "xhttp-opts.mode", transport["mode"])
	xmux := mapObject(transport, "xmux")
	if m := mapObject(extra, "xmux"); m != nil {
		xmux = m
	}
	for k, v := range xmux {
		target := map[string]string{"maxConcurrency": "MaxConcurrency", "maxConnections": "MaxConnections", "cMaxReuseTimes": "CMaxReuseTimes", "hMaxReusableSecs": "HMaxReusableSecs", "hKeepAlivePeriod": "HKeepAlivePeriod"}[k]
		if target == "" {
			return fmt.Errorf("不支持的 XHTTP xmux 参数 %s", k)
		}
		put(n, "xhttp."+target, v)
	}
	return nil
}
