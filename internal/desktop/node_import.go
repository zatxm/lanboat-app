package desktop

import (
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"go.uber.org/zap"
)

type importGroup struct {
	sub   profile.Subscription
	nodes []profile.Node
	rules string
}

func githubRaw(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "github.com" {
		return raw
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) >= 5 && parts[2] == "blob" {
		u.Host = "raw.githubusercontent.com"
		u.Path = "/" + strings.Join(append(parts[:2], parts[3:]...), "/")
		return u.String()
	}
	return raw
}

func (a *App) Import(content string) (int, error) {
	content = strings.TrimSpace(content)
	if filepath.IsAbs(content) || strings.HasPrefix(content, "~/") {
		var err error
		content, err = a.readImport(content, profile.MaxConfig)
		if err != nil {
			return 0, err
		}
	}
	if len(content) <= profile.MaxConfig && strings.HasPrefix(content, "{") {
		var kind struct {
			Format string `json:"format"`
		}
		if json.Unmarshal([]byte(content), &kind) == nil && strings.HasPrefix(kind.Format, "lanboat-nodes/") {
			return a.importNodeBundle(content)
		}
	}
	if len(content) > profile.MaxImport {
		return 0, errors.New("导入内容不能超过 2 MiB")
	}
	// Single encoded subscriptions can themselves contain a list of URLs.
	if !strings.ContainsAny(content, "\n{}[]") && !strings.Contains(content, "://") {
		if decoded, err := profile.DecodeBase64(strings.Join(strings.Fields(content), "")); err == nil && len(decoded) <= profile.MaxImport {
			content = strings.TrimSpace(string(decoded))
		}
	}
	var groups []importGroup
	var local []string
	structured := profile.IsProxyText(content) || strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") || strings.Contains(content, "proxies:") || strings.Contains(content, "outbounds:") || strings.Contains(content, "proxy-providers:")
	if structured {
		local = append(local, content)
	} else {
		type inputLine struct {
			text  string
			depth int
		}
		var queue []inputLine
		for _, line := range strings.Split(content, "\n") {
			queue = append(queue, inputLine{line, 0})
		}
		for i := 0; i < len(queue); i++ {
			line := strings.TrimSpace(queue[i].text)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.Contains(line, "://") && queue[i].depth < 4 {
				if decoded, err := profile.DecodeBase64(line); err == nil && len(decoded) <= profile.MaxImport {
					value := strings.TrimSpace(string(decoded))
					if profile.IsProxyText(value) || strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") || strings.Contains(value, "proxies:") || strings.Contains(value, "outbounds:") || strings.Contains(value, "proxy-providers:") {
						value, err = a.expandClashProviders(value, profile.Subscription{})
						if err != nil {
							return 0, err
						}
						nodes, rules, err := profile.ParseImportWithRules(value, "[Rule]\n")
						if err != nil {
							return 0, err
						}
						groups = append(groups, importGroup{nodes: nodes, rules: rules})
					} else {
						for _, piece := range strings.Split(value, "\n") {
							queue = append(queue, inputLine{piece, queue[i].depth + 1})
						}
					}
					continue
				}
			}
			parsed, err := url.Parse(line)
			name, password := "", ""
			if err == nil && ((parsed.Scheme == "karing" || parsed.Scheme == "clash" || parsed.Scheme == "stash") && parsed.Host == "install-config" || parsed.Scheme == "sing-box" && parsed.Host == "import-remote-profile") {
				name, password = parsed.Query().Get("name"), parsed.Query().Get("decrypt-password")
				if name == "" {
					name = parsed.Fragment
				}
				line = parsed.Query().Get("url")
				if strings.Contains(line, "|") && !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
					local = append(local, strings.Split(line, "|")...)
					continue
				}
				parsed, err = url.Parse(line)
				if err != nil || line == "" {
					return 0, errors.New("订阅链接缺少有效地址")
				}
			}
			if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil {
				group, err := a.subscriptionImportGroup(profile.Subscription{URL: line, Name: name, Password: password, KeepRules: true})
				if err != nil {
					return 0, err
				}
				groups = append(groups, group)
			} else {
				local = append(local, line)
			}
		}
	}
	if len(local) > 0 {
		content, err := a.expandClashProviders(strings.Join(local, "\n"), profile.Subscription{})
		if err != nil {
			return 0, err
		}
		nodes, rules, err := profile.ParseImportWithRules(content, "[Rule]\n")
		if err != nil {
			return 0, err
		}
		groups = append(groups, importGroup{nodes: nodes, rules: rules})
	}
	if len(groups) == 0 {
		return 0, errors.New("没有找到可导入节点")
	}
	return a.commitImport(groups)
}

func (a *App) ImportSubscription(address string, keepRules bool) (int, error) {
	return a.ImportSubscriptionOptions(profile.Subscription{URL: address, KeepRules: keepRules})
}

func (a *App) ImportSubscriptionOptions(sub profile.Subscription) (int, error) {
	group, err := a.subscriptionImportGroup(sub)
	if err != nil {
		return 0, err
	}
	return a.commitImport([]importGroup{group})
}

func (a *App) subscriptionImportGroup(sub profile.Subscription) (importGroup, error) {
	sub.URL = githubRaw(sub.URL)
	sub.ID = profile.ID()
	if parsed, err := url.Parse(sub.URL); err == nil && sub.Name == "" {
		sub.Name = parsed.Hostname()
		if sub.Name == "raw.githubusercontent.com" {
			sub.Name = path.Base(parsed.Path)
		}
	}
	if err := sub.Validate(); err != nil {
		return importGroup{}, err
	}
	payload, err := a.downloadSubscription(sub)
	if err != nil {
		return importGroup{}, fmt.Errorf("订阅 %s：%w", sub.Name, err)
	}
	payload, err = a.expandClashProviders(payload, sub)
	if err != nil {
		return importGroup{}, err
	}
	nodes, rules, err := profile.ParseSubscription(payload, sub, "[Rule]\n")
	if err != nil {
		return importGroup{}, fmt.Errorf("订阅 %s：%w", sub.Name, err)
	}
	sub.UpdatedAt = time.Now().Format(time.RFC3339)
	for i := range nodes {
		nodes[i].Source = sub.ID
	}
	return importGroup{sub, nodes, rules}, nil
}

func (a *App) commitImport(groups []importGroup) (int, error) {
	nodeGroups := make([][]profile.Node, len(groups))
	for i := range groups {
		nodeGroups[i] = groups[i].nodes
	}
	a.fillNodeFlags(nodeGroups...)
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Nodes = append([]profile.Node{}, c.Nodes...)
	c.Subscriptions = append([]profile.Subscription{}, c.Subscriptions...)
	count := 0
	for _, g := range groups {
		if g.sub.ID != "" {
			if len(c.Subscriptions) >= 30 {
				return 0, errors.New("最多保存 30 个订阅")
			}
			c.Subscriptions = append(c.Subscriptions, g.sub)
		}
		if g.rules != "" {
			ruleSub := g.sub
			if ruleSub.ID == "" {
				ruleSub.ID = profile.ID()
				ruleSub.Name = "导入节点"
			}
			if err := syncSubscriptionRules(&c, ruleSub, g.rules, g.nodes); err != nil {
				return 0, err
			}
		}
		c.Nodes = append(c.Nodes, g.nodes...)
		count += len(g.nodes)
	}
	if len(c.Nodes) > profile.MaxNodes {
		return 0, errors.New("最多保存 100000 个节点")
	}
	if c.Selected == "" {
		c.Selected = groups[0].nodes[0].ID
	}
	if err := a.commit(c); err != nil {
		return 0, err
	}
	a.log.Info("nodes imported", zap.Int("count", count))
	return count, nil
}
