package desktop

import (
	"context"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *App) ReadImport(input string) (string, error) {
	return a.readImport(input, routing.MaxConfig)
}
func (a *App) readImport(input string, limit int64) (string, error) {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://") {
		u, err := url.Parse(input)
		if err != nil || u.User != nil || u.Hostname() == "" {
			return "", errors.New("配置地址无效")
		}
		client := *a.client
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.User != nil || (req.URL.Scheme != "https" && req.URL.Scheme != "http") {
				return errors.New("配置重定向无效")
			}
			return nil
		}
		resp, err := client.Get(input)
		if err != nil {
			return "", errors.New("下载配置失败")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil {
			return "", errors.New("读取配置失败")
		}
		if int64(len(b)) > limit {
			return "", errors.New("输入文件过大")
		}
		return string(b), nil
	}
	if !strings.Contains(input, "\n") && !strings.HasPrefix(input, "[") && !strings.HasPrefix(input, "{") {
		path := input
		if strings.HasPrefix(path, "~/") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[2:])
		}
		f, err := os.Open(path)
		if err != nil {
			return "", errors.New("无法读取文件，请填写文件路径、HTTPS URL 或完整配置内容")
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, limit+1))
		if int64(len(b)) > limit {
			return "", errors.New("输入文件过大")
		}
		return string(b), err
	}
	if int64(len(input)) > limit {
		return "", errors.New("输入内容过大")
	}
	return input, nil
}
func (a *App) ParseConfigImport(input string) (string, error) {
	content, err := a.ReadImport(input)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", errors.New("配置内容不能为空")
	}
	if !utf8.ValidString(content) {
		return "", errors.New("配置文件必须为 UTF-8 文本")
	}
	if _, err = routing.Parse(content); err != nil {
		return "", err
	}
	return content, nil
}
func (a *App) SaveConfigFile(id, name, input string) error {
	if id == "default" {
		return errors.New("默认配置为只读，请先复制后修改")
	}
	content, err := a.ParseConfigImport(input)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if !profile.ValidFileName(name) {
		return errors.New("配置名称无效")
	}
	if !strings.HasSuffix(name, ".conf") {
		name += ".conf"
	}
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Files = append([]profile.ConfigFile{}, c.Files...)
	if id == "" {
		id = profile.ID()
		if len(c.Files) >= 30 {
			return errors.New("最多保存 30 个配置")
		}
		c.Files = append(c.Files, profile.ConfigFile{ID: id})
	}
	content, err = c.ImportPolicyGroups(content, "file:"+id, nil)
	if err != nil {
		return err
	}
	found := false
	for i, f := range c.Files {
		if f.ID == id {
			c.Files[i] = profile.ConfigFile{ID: id, Name: name, Content: content, UpdatedAt: time.Now().Format(time.RFC3339), Source: "本地导入或编辑"}
			found = true
		}
	}
	if !found {
		return errors.New("配置文件不存在")
	}
	return a.commit(c)
}

func (a *App) CopyConfigFile(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.config.Files) >= 30 {
		return errors.New("最多保存 30 个配置")
	}
	for _, f := range a.config.Files {
		if f.ID != id {
			continue
		}
		names := make(map[string]bool, len(a.config.Files))
		for _, file := range a.config.Files {
			names[file.Name] = true
		}
		base := strings.TrimSuffix(f.Name, ".conf")
		if len(base) > 120 {
			base = "config"
		}
		for i := 1; ; i++ {
			name := fmt.Sprintf("%s-copy-%d.conf", base, i)
			if names[name] {
				continue
			}
			f.ID, f.Name, f.Source, f.UpdatedAt = profile.ID(), name, "本地复制", time.Now().Format(time.RFC3339)
			c := a.config
			c.Files = append(append([]profile.ConfigFile{}, c.Files...), f)
			return a.commit(c)
		}
	}
	return errors.New("配置文件不存在")
}
func (a *App) ActivateConfigFile(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, f := range a.config.Files {
		if f.ID == id {
			if _, err := routing.Parse(f.Content); err != nil {
				return err
			}
			c := a.config
			c.ActiveFile = id
			return a.commit(c)
		}
	}
	return errors.New("配置不存在")
}
func (a *App) DeleteConfigFile(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == "default" || id == a.config.ActiveFile {
		return errors.New("默认或正在使用的配置不能删除")
	}
	c := a.config
	c.Files = nil
	for _, f := range a.config.Files {
		if f.ID != id {
			c.Files = append(c.Files, f)
		}
	}
	return a.commit(c)
}
func (a *App) TestRule(host, ip string) (string, error) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if ip != "" && err != nil {
		return "", fmt.Errorf("IP地址无效")
	}
	return a.TestRuleRequest(routing.Request{Host: strings.TrimSpace(host), IP: addr, Port: 443, Network: "tcp"})
}
func (a *App) TestRuleRequest(req routing.Request) (string, error) {
	a.ruleUpdateMu.Lock()
	defer a.ruleUpdateMu.Unlock()
	if req.Port < 1 || req.Port > 65535 || req.Network != "tcp" && req.Network != "udp" {
		return "", fmt.Errorf("测试端口或网络协议无效")
	}
	a.mu.Lock()
	c := a.config
	a.mu.Unlock()
	p, err := profile.EffectiveRouting(c)
	if err != nil {
		return "", err
	}
	if err := profile.InsertCustomRouting(&p, c.Rules); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = routing.ResolveSets(ctx, &p, a.engine.CorePath(), filepath.Join(filepath.Dir(a.path), "runtime")); err != nil {
		return "", err
	}
	action, rule := p.Final, "FINAL"
	for _, entry := range p.Rules {
		matched := false
		if entry.SetPath != "" {
			matched, err = routing.MatchBinary(ctx, a.engine.CorePath(), entry.SetPath, req)
			if err != nil {
				return "", err
			}
		} else {
			matched = routing.MatchRule(entry, req)
		}
		if matched {
			action = entry.Action
			rule = fmt.Sprintf("%s,%s（第 %d 行）", entry.Type, entry.Value, entry.Line)
			break
		}
	}
	if action == "direct" || action == "proxy" || action == "reject" {
		action = strings.ToUpper(action)
	} else {
		for _, g := range c.PolicyGroups {
			if action == g.Ref() {
				action = g.Name
				break
			}
		}
		for _, n := range c.Nodes {
			if action == "node:"+n.ID {
				action = n.Name
				break
			}
		}
	}
	return action + " · " + rule, nil
}
func (a *App) exportPayload(kind, id string) (string, []byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var data []byte
	name := ""
	switch kind {
	case "nodes":
		b := nodeBundle{Format: nodeFormat, Nodes: a.config.Nodes, Subscriptions: a.config.Subscriptions, Selected: a.config.Selected, RuleOverlays: a.config.RuleOverlays}
		contents := make([]string, 0, len(a.config.RuleOverlays))
		for _, overlay := range a.config.RuleOverlays {
			contents = append(contents, overlay.Content)
		}
		groups, err := a.config.ReferencedPolicyGroups(contents)
		if err != nil {
			return "", nil, err
		}
		b.PolicyGroups = groups
		data, err = json.Marshal(b)
		if err != nil {
			return "", nil, err
		}
		name = "lanboat-nodes-" + time.Now().Format("20060102-150405") + ".json"
	case "config":
		for _, f := range a.config.Files {
			if f.ID == id {
				content, err := a.config.ExportRoutingConfig(f.Content)
				if err != nil {
					return "", nil, err
				}
				data = []byte(content)
				name = filepath.Base(f.Name)
			}
		}
	}
	if name == "" {
		return "", nil, errors.New("没有可导出的内容")
	}
	return name, data, nil
}

func (a *App) ExportDataTo(kind, id, directory string) (string, error) {
	name, data, err := a.exportPayload(kind, id)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("请选择有效的本地目录")
	}
	return writeUniqueFile(directory, name, data)
}

func writeUniqueFile(directory, name string, data []byte) (string, error) {
	base, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
	for i := 1; ; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		path := filepath.Join(directory, candidate)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(data)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(path)
			return "", err
		}
		return path, nil
	}
}

func (a *App) DeleteAllNodes() error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	deleted := make(map[string]bool, len(c.Nodes))
	for _, node := range c.Nodes {
		deleted[node.ID] = true
	}
	removeOverlayNodes(&c, deleted)
	c.Nodes = []profile.Node{}
	c.Selected = ""
	return a.commit(c)
}
func (a *App) RefreshAllSubscriptions() error {
	a.mu.Lock()
	subs := append([]profile.Subscription{}, a.config.Subscriptions...)
	a.mu.Unlock()
	var failed int
	for _, s := range subs {
		if _, err := a.RefreshSubscription(s.ID); err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d 个订阅更新失败，原节点已保留", failed)
	}
	return nil
}
func (a *App) DNSLookup(host string) (string, error) {
	if !profile.ValidHost(host) {
		return "", errors.New("请输入有效域名或 IP")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	records, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		_ = a.networkLogs.Record(traffic.Entry{Kind: "dns", Domain: host, Resolver: "system", Error: err.Error(), DurationMS: time.Since(started).Milliseconds()})
		return "", errors.New("DNS 查询失败或超时")
	}
	var ips []string
	for _, r := range records {
		ips = append(ips, r.IP.String())
	}
	_ = a.networkLogs.Record(traffic.Entry{Kind: "dns", Domain: host, QueryType: "A/AAAA", Result: strings.Join(ips, ", "), Resolver: "system", DurationMS: time.Since(started).Milliseconds()})
	return strings.Join(ips, "\n"), nil
}
func (a *App) mirrorConfigs(c profile.Config) {
	dir := filepath.Join(filepath.Dir(a.path), "configs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	for _, f := range c.Files {
		if strings.ContainsAny(f.Name, "/\\") {
			continue
		}
		if err := writePrivateFile(filepath.Join(dir, f.Name), []byte(f.Content)); err != nil {
			a.log.Warn("Could not mirror configuration file")
		}
	}
}
func (a *App) ImportBackup(input string) error {
	input = strings.TrimSpace(input)
	// Earlier exports could be named .tar-2.gz after a filename collision.
	if !strings.HasPrefix(input, "{") && !strings.Contains(input, "\n") && strings.HasSuffix(strings.ToLower(input), ".gz") {
		return a.importArchive(input)
	}
	content, err := a.readImport(input, profile.MaxConfig)
	if err != nil {
		return err
	}
	var c profile.Config
	if err = json.Unmarshal([]byte(content), &c); err != nil {
		return errors.New("备份 JSON 无效")
	}
	if err = c.ValidateSnapshot(); err != nil {
		return err
	}
	profile.EnsureFiles(&c)
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if c.Settings.Language == "" {
		c.Settings.Language = a.config.Settings.Language
	}
	return a.commit(c)
}

func writePrivateFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".export-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
