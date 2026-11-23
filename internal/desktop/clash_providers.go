package desktop

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
	"go.yaml.in/yaml/v3"
)

func clashProviderString(values map[string]any, key string) string {
	if value := values[key]; value != nil {
		return fmt.Sprint(value)
	}
	return ""
}

// Expand provider nodes before parsing so the same source owns the nodes and rules.
func (a *App) expandClashProviders(content string, parent profile.Subscription) (string, error) {
	if !strings.Contains(content, "proxy-providers") {
		if decoded, err := profile.DecodeBase64(strings.Join(strings.Fields(content), "")); err == nil && strings.Contains(string(decoded), "proxy-providers") {
			content = string(decoded)
		} else {
			return content, nil
		}
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", err
	}
	providers, _ := doc["proxy-providers"].(map[string]any)
	if len(providers) == 0 {
		return content, nil
	}
	if len(providers) > 30 {
		return "", errors.New("代理提供器不能超过 30 个")
	}
	keys := make([]string, 0, len(providers))
	for name := range providers {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	proxies, _ := doc["proxies"].([]any)
	members := map[string][]string{}
	for _, name := range keys {
		p, ok := providers[name].(map[string]any)
		if !ok {
			return "", fmt.Errorf("代理提供器 %s 格式无效", name)
		}
		if interval := clashProviderString(p, "interval"); interval != "" {
			seconds, err := strconv.Atoi(interval)
			if err != nil || seconds < 1 || seconds > 31536000 {
				return "", fmt.Errorf("代理提供器 %s 的 interval 必须为 1–31536000 秒", name)
			}
		}
		for _, key := range []string{"filter", "exclude-filter"} {
			if value := clashProviderString(p, key); value != "" {
				if _, err := regexp.Compile(value); err != nil {
					return "", fmt.Errorf("代理提供器 %s 的 %s 正则无效: %w", name, key, err)
				}
			}
		}
		var data string
		switch p["type"] {
		case "http":
			address, _ := p["url"].(string)
			sub := profile.Subscription{Name: name, URL: githubRaw(address), UserAgent: parent.UserAgent, Insecure: parent.Insecure, Password: parent.Password}
			if err := sub.Validate(); err != nil {
				return "", err
			}
			var headers map[string][]string
			if raw := p["header"]; raw != nil {
				values, ok := raw.(map[string]any)
				if !ok {
					return "", fmt.Errorf("代理提供器 %s 的 header 必须是请求头对象", name)
				}
				var err error
				headers, err = profile.NormalizeHTTPHeaders(values)
				if err != nil {
					return "", fmt.Errorf("代理提供器 %s 的 header：%w", name, err)
				}
			}
			var err error
			data, err = a.downloadSubscription(sub, headers)
			if err != nil {
				return "", fmt.Errorf("代理提供器 %s：%w", name, err)
			}
		case "file":
			if p["header"] != nil {
				return "", fmt.Errorf("代理提供器 %s 仅支持 HTTP 类型的 header", name)
			}
			file, _ := p["path"].(string)
			if !strings.HasPrefix(file, "/") && !strings.HasPrefix(file, "~/") {
				return "", fmt.Errorf("代理提供器 %s 的本地路径需要绝对路径", name)
			}
			var err error
			data, err = a.readImport(file, profile.MaxSubscription)
			if err != nil {
				return "", err
			}
		case "inline":
			if p["header"] != nil {
				return "", fmt.Errorf("代理提供器 %s 仅支持 HTTP 类型的 header", name)
			}
			payload, err := yaml.Marshal(map[string]any{"proxies": p["payload"]})
			if err != nil {
				return "", err
			}
			data = string(payload)
		default:
			return "", fmt.Errorf("不支持的代理提供器类型 %v", p["type"])
		}
		nodes, _, err := profile.ParseSubscription(data, profile.Subscription{Name: name, URL: "http://localhost"}, "[Rule]\n")
		if err != nil {
			return "", fmt.Errorf("代理提供器 %s：%w", name, err)
		}
		var filter, exclude *regexp.Regexp
		if value, _ := p["filter"].(string); value != "" {
			filter, err = regexp.Compile(value)
			if err != nil {
				return "", err
			}
		}
		if value, _ := p["exclude-filter"].(string); value != "" {
			exclude, err = regexp.Compile(value)
			if err != nil {
				return "", err
			}
		}
		for _, n := range nodes {
			if filter != nil && !filter.MatchString(n.Name) || exclude != nil && exclude.MatchString(n.Name) {
				continue
			}
			profile.NormalizeSS(&n)
			entry := map[string]any{"type": n.Type, "name": n.Name, "server": n.Server, "port": n.Port, "password": n.Password}
			for key, value := range n.Options {
				entry[key] = value
			}
			if n.Method != "" {
				entry["cipher"] = n.Method
			}
			if n.Plugin != "" {
				entry["plugin"] = n.Plugin
			}
			proxies = append(proxies, entry)
			members[name] = append(members[name], n.Name)
		}
	}
	if groups, ok := doc["proxy-groups"].([]any); ok {
		for _, item := range groups {
			group, _ := item.(map[string]any)
			use, _ := group["use"].([]any)
			if len(use) == 0 {
				continue
			}
			existing, _ := group["proxies"].([]any)
			for _, provider := range use {
				name := fmt.Sprint(provider)
				list, ok := members[name]
				if !ok {
					return "", fmt.Errorf("策略组引用不存在或为空的提供器 %s", name)
				}
				for _, member := range list {
					existing = append(existing, member)
				}
			}
			group["proxies"] = existing
			delete(group, "use")
		}
	}
	doc["proxies"] = proxies
	delete(doc, "proxy-providers")
	result, err := yaml.Marshal(doc)
	return string(result), err
}
