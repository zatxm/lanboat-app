package desktop

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zatxm/lanboat-app/internal/appmeta"
	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/engine"
	"github.com/zatxm/lanboat-app/internal/i18n"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"go.uber.org/zap"
)

type App struct {
	ruleUpdateMu     sync.Mutex
	flagGeneration   string
	flagIndex        ruledata.CountryIndex
	connectionMu     sync.Mutex
	connectionCtx    context.Context
	connectionCancel context.CancelFunc
	connectedAt      int64
	activeNode       string
	switching        bool
	connectionError  string
	nodeDelays       map[string]int64
	lastPrune        time.Time
	mu               sync.Mutex
	config           profile.Config
	path             string
	engine           *engine.Engine
	mobile           bool
	proxy            SystemProxy
	networkLogs      *traffic.Store
	log              *zap.Logger
	logs             *Logs
	client           *http.Client
}

type State struct {
	NodeDelays           map[string]int64 `json:"nodeDelays,omitempty"`
	Config               profile.Config   `json:"config"`
	Backups              []BackupInfo     `json:"backups,omitempty"`
	Methods              []string         `json:"methods"`
	SystemProxySupported bool             `json:"systemProxySupported"`
	DataDir              string           `json:"dataDir"`
}

type Status struct {
	engine.Status
	Memory     Memory `json:"memory"`
	ActiveNode string `json:"activeNode,omitempty"`
	Switching  bool   `json:"switching,omitempty"`
}

func New(dir, core string, log *zap.Logger, logs *Logs) (*App, error) {
	path := filepath.Join(dir, "config.json")
	proxy := SystemProxy{Path: filepath.Join(dir, "proxy-backup.json")}
	if err := proxy.Restore(); err != nil {
		return nil, fmt.Errorf("恢复上次的系统代理失败：%w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "runtime"), 0700); err != nil {
		return nil, err
	}
	c, err := profile.Load(path)
	if err != nil {
		return nil, err
	}
	initialize := c.Settings.Language == ""
	if c.Settings.SystemProxy {
		c.Settings.SystemProxy = false
		initialize = true
	}
	if c.Settings.Language == "" {
		c.Settings.Language = i18n.DefaultLanguage()
	}
	if profile.EnsureFiles(&c) {
		initialize = true
	}
	migrated, err := profile.MigrateSubscriptionRules(&c)
	if err != nil {
		return nil, err
	}
	if migrated {
		initialize = true
	}
	if initialize {
		if err = profile.Save(path, c); err != nil {
			return nil, err
		}
	}
	a := &App{config: c, path: path, engine: engine.New(core, filepath.Join(dir, "runtime"), log), proxy: proxy, log: log, logs: logs,
		client: &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{ForceAttemptHTTP2: true}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.User != nil || (req.URL.Scheme != "https" && req.URL.Scheme != "http") || len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return errSubscriptionRedirect
			}
			return nil
		}}}
	a.networkLogs = traffic.New(filepath.Join(dir, "logs"))
	a.networkLogs.SetEnabled(c.Settings.ProxyLog, c.Settings.DNSLog)
	a.engine.SetRequestLogging(c.Settings.ProxyLog)
	a.engine.SetTrafficStore(a.networkLogs)
	if err := ruledata.CompactInactive(a.ruleDataDir()); err != nil {
		a.log.Warn("compress inactive rule data", zap.Error(err))
	}
	// Runtime secrets left by a killed process are never reused.
	stale, _ := filepath.Glob(filepath.Join(dir, "runtime", ".core-*.json"))
	helperStale, _ := filepath.Glob(filepath.Join(dir, "runtime", ".helper-*.json"))
	diagnosticStale, _ := filepath.Glob(filepath.Join(dir, "runtime", ".dns-check-*.json"))
	stale = append(stale, helperStale...)
	stale = append(stale, diagnosticStale...)
	for _, path := range stale {
		_ = os.Remove(path)
	}
	a.mirrorConfigs(c)
	return a, nil
}

func (a *App) UseMobileLauncher(launch coreipc.Launcher) {
	a.mobile = true
	a.engine.SetMobileLauncher(launch)
	a.normalizeMobileSettings(&a.config.Settings)
}

func (a *App) normalizeMobileSettings(settings *profile.Settings) {
	if !a.mobile {
		return
	}
	settings.SystemProxy = false
	if runtime.GOOS == "ios" {
		settings.Mode = "global"
		settings.Proxy.Upstream.Enabled = false
	} else if settings.Mode == "tun" {
		settings.Mode = "global"
	} else if settings.Mode == "direct" {
		settings.Mode = "rule"
	}
}

func (a *App) GetState() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Nodes = append([]profile.Node{}, c.Nodes...)
	c.Settings.AutoSelectGroups = slices.Clone(c.Settings.AutoSelectGroups)
	c.Files = append([]profile.ConfigFile{}, c.Files...)
	c.Modules = append([]profile.Module{}, c.Modules...)
	c.Rules = append([]profile.Rule{}, c.Rules...)
	c.PolicyGroups = profile.ClonePolicyGroups(c.PolicyGroups)
	c.Subscriptions = append([]profile.Subscription{}, c.Subscriptions...)
	c.RuleOverlays = append([]profile.RuleOverlay{}, c.RuleOverlays...)
	for i := range c.RuleOverlays {
		c.RuleOverlays[i].NodeIDs = append([]string(nil), c.RuleOverlays[i].NodeIDs...)
	}
	for i := range c.Nodes {
		c.Nodes[i] = c.Nodes[i].Masked()
	}
	for i := range c.Subscriptions {
		c.Subscriptions[i].Password = ""
	}
	delays := make(map[string]int64, len(a.nodeDelays))
	for id, ms := range a.nodeDelays {
		delays[id] = ms
	}
	if c.Settings.UDP.Relay != nil {
		relay := *c.Settings.UDP.Relay
		c.Settings.UDP.Relay = &relay
	}
	if c.Settings.DNS != nil {
		dns := *c.Settings.DNS
		c.Settings.DNS = &dns
	}
	return State{Config: c, Backups: a.Backups(), NodeDelays: delays, Methods: profile.Methods, SystemProxySupported: ProxySupported(), DataDir: filepath.Dir(a.path)}
}

func (a *App) GetStatus() Status           { return a.status(true) }
func (a *App) GetConnectionStatus() Status { return a.status(false) }
func (a *App) status(traffic bool) Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.lastPrune) > time.Hour {
		a.pruneLogs()
		a.lastPrune = time.Now()
	}
	s := engine.Status{Connected: true, CoreAvailable: true}
	if !a.switching {
		if traffic {
			s = a.engine.Snapshot()
		} else {
			s = a.engine.State()
		}
	}
	switching := a.switching
	if s.Connected {
		s.StartedAt = a.connectedAt
	} else {
		a.connectedAt = 0
	}
	if !s.Connected && a.connectionCancel != nil {
		a.connectionCancel()
		a.connectionCtx = nil
		a.connectionCancel = nil
		a.activeNode = ""
	}
	if !s.Connected && a.connectionCancel == nil {
		if err := a.proxy.Restore(); err != nil {
			s.Error = err.Error()
		}
	}
	if a.connectionError != "" {
		s.Error = a.connectionError
	}
	return Status{Status: s, ActiveNode: a.activeNode, Switching: switching}
}

func (a *App) MemoryUsage() Memory { return ProcessMemory(os.Getpid()) }

func (a *App) GetLogs() []LogEntry { return a.logs.List() }

func (a *App) commit(c profile.Config) error { return a.commitWithRuleData(c, false, nil) }
func (a *App) commitWithRuleData(c profile.Config, force bool, restoreData func() error) error {
	a.normalizeMobileSettings(&c.Settings)
	overlaysChanged := !reflect.DeepEqual(a.config.RuleOverlays, c.RuleOverlays)
	if overlaysChanged {
		if err := profile.ValidateRuleOverlays(c.RuleOverlays); err != nil {
			return err
		}
	}
	if err := a.checkActiveNode(c); err != nil {
		return err
	}
	filesChanged := !reflect.DeepEqual(a.config.Files, c.Files)
	if filesChanged || a.config.ActiveFile != c.ActiveFile {
		if err := profile.ValidateFiles(c); err != nil {
			return err
		}
	}
	if filesChanged || overlaysChanged || !reflect.DeepEqual(a.config.Rules, c.Rules) || !reflect.DeepEqual(a.config.PolicyGroups, c.PolicyGroups) {
		if err := c.ValidatePolicyGroups(); err != nil {
			return err
		}
	}
	rollback, err := a.applyConnectedConfig(c, force, restoreData)
	if err != nil {
		return err
	}
	if err := profile.Save(a.path, c); err != nil {
		return errors.Join(fmt.Errorf("保存配置失败：%w", err), rollback())
	}
	if a.connectionCancel != nil && !a.switching {
		a.activeNode = c.Selected
		if c.Settings.Mode == "direct" {
			a.activeNode = ""
		}
	}
	oldFiles := a.config.Files
	a.config = c
	a.networkLogs.SetEnabled(c.Settings.ProxyLog, c.Settings.DNSLog)
	a.engine.SetRequestLogging(c.Settings.ProxyLog)
	if !filesChanged {
		return nil
	}
	a.mirrorConfigs(c)
	names := map[string]bool{}
	for _, f := range c.Files {
		names[f.Name] = true
	}
	for _, f := range oldFiles {
		if !names[f.Name] && profile.ValidFileName(f.Name) {
			_ = os.Remove(filepath.Join(filepath.Dir(a.path), "configs", f.Name))
		}
	}
	return nil
}

func (a *App) SaveNode(n profile.Node) error {
	n = n.Clone()
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Nodes = append([]profile.Node{}, c.Nodes...)
	index := -1
	if n.ID == "" {
		n.ID = profile.ID()
	} else {
		for i, old := range c.Nodes {
			if old.ID == n.ID {
				if old.Plugin == "shadow-tls" {
					old = old.Clone()
					profile.NormalizeSS(&old)
				}
				index = i
				n.Source = old.Source
				if n.Flag == "" && n.Server == old.Server {
					n.Flag = old.Flag
				}
				if n.Password == "" && n.Kind() == old.Kind() {
					n.Password = old.Password
				}
				if n.Kind() == old.Kind() {
					// Opaque transport settings stay private and survive form edits.
					incoming := n.Options
					saved := old.Clone()
					n.Options = saved.Options
					if n.Options == nil {
						n.Options = map[string]any{}
					}
					for key, value := range incoming {
						n.Options[key] = value
					}
					for _, field := range profile.NodeFields(n) {
						if field.Secret && n.TextOption(field.Key) == "" && old.Option(field.Key) != nil && (n.Plugin == old.Plugin || !strings.HasPrefix(field.Key, "plugin-opts.")) {
							n.SetOption(field.Key, old.Option(field.Key))
						}
					}
				}
				break
			}
		}
		if index < 0 {
			return errors.New("节点不存在")
		}
	}
	if err := n.Validate(); err != nil {
		return err
	}
	n.FillFlagFromName()
	if index >= 0 {
		c.Nodes[index] = n
	} else {
		if len(c.Nodes) >= profile.MaxNodes {
			return errors.New("最多保存 100000 个节点")
		}
		n.Source = ""
		c.Nodes = append(c.Nodes, n)
	}
	if c.Selected == "" {
		c.Selected = n.ID
	}
	if err := a.commit(c); err != nil {
		return err
	}
	a.log.Info("node saved")
	return nil
}

func (a *App) CopyNode(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()

	c := a.config
	index := slices.IndexFunc(c.Nodes, func(n profile.Node) bool { return n.ID == id })
	if index < 0 {
		return errors.New("节点不存在")
	}
	if len(c.Nodes) >= profile.MaxNodes {
		return errors.New("最多保存 100000 个节点")
	}
	names := make(map[string]bool, len(c.Nodes))
	for _, n := range c.Nodes {
		names[n.Name] = true
	}
	node := c.Nodes[index].Clone()
	base := node.Name
	for i := 1; ; i++ {
		suffix := "-" + strconv.Itoa(i)
		end := min(len(base), profile.MaxNodeNameBytes-len(suffix))
		for end < len(base) && !utf8.RuneStart(base[end]) {
			end--
		}
		node.Name = base[:end] + suffix
		if !names[node.Name] {
			break
		}
	}
	node.ID, node.Source = profile.ID(), ""
	c.Nodes = slices.Concat(c.Nodes, []profile.Node{node})
	return a.commit(c)
}

func (a *App) DeleteNode(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Nodes = []profile.Node{}
	for _, n := range a.config.Nodes {
		if n.ID != id {
			c.Nodes = append(c.Nodes, n)
		}
	}
	removeOverlayNodes(&c, map[string]bool{id: true})
	if c.Selected == id {
		c.Selected = ""
		if len(c.Nodes) > 0 {
			c.Selected = c.Nodes[0].ID
		}
	}
	return a.commit(c)
}

func (a *App) DeleteLocalNodes() error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()

	c := a.config
	c.Nodes = []profile.Node{}
	deleted := map[string]bool{}
	for _, n := range a.config.Nodes {
		if c.NodeGroup(n) == "" {
			deleted[n.ID] = true
		} else {
			c.Nodes = append(c.Nodes, n)
		}
	}
	if len(deleted) == 0 {
		return nil
	}
	if dns := c.Settings.DNS; dns != nil && dns.ProxyRoute == "node" && deleted[dns.ProxyNode] {
		return errors.New("正在使用的DNS出站节点不能修改或删除")
	}
	removeOverlayNodes(&c, deleted)
	if deleted[c.Selected] {
		c.Selected = ""
		if len(c.Nodes) > 0 {
			c.Selected = c.Nodes[0].ID
		}
	}
	return a.commit(c)
}

func (a *App) SelectNode(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, n := range a.config.Nodes {
		if n.ID == id {
			c := a.config
			c.Selected = id
			return a.commit(c)
		}
	}
	return errors.New("节点不存在")
}

func (a *App) ExportNode(id string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, n := range a.config.Nodes {
		if n.ID == id {
			return profile.URI(n), nil
		}
	}
	return "", errors.New("节点不存在")
}

func (a *App) TestNode(id string) (int64, error) {
	return a.TestNodeContext(context.Background(), id)
}

func (a *App) TestNodeContext(ctx context.Context, id string) (int64, error) {
	a.mu.Lock()
	var node profile.Node
	for _, n := range a.config.Nodes {
		if n.ID == id {
			node = n
			break
		}
	}
	s := a.config.Settings
	a.mu.Unlock()
	if node.ID == "" {
		return 0, errors.New("节点不存在")
	}
	return a.testNodeContext(ctx, node, s)
}

func (a *App) testNodeContext(ctx context.Context, node profile.Node, s profile.Settings) (int64, error) {
	return a.httpTestNodeContext(ctx, node, s)
}

func (a *App) SaveSubscription(name, rawURL string) (int, error) {
	return a.SaveSubscriptionOptions(profile.Subscription{Name: name, URL: rawURL})
}

func (a *App) SaveSubscriptionOptions(sub profile.Subscription) (int, error) {
	if err := sub.Validate(); err != nil {
		return 0, err
	}
	a.mu.Lock()
	create := sub.ID == ""
	if !create {
		found := false
		for _, old := range a.config.Subscriptions {
			if old.ID == sub.ID {
				found = true
				if sub.Password == "" {
					sub.Password = old.Password
				}
				break
			}
		}
		if !found {
			a.mu.Unlock()
			return 0, errors.New("订阅不存在")
		}
	}
	count := len(a.config.Subscriptions)
	a.mu.Unlock()
	if create && count >= 30 {
		return 0, errors.New("最多保存 30 个订阅")
	}
	if create {
		if sub.UDP == "" {
			sub.UDP = "false"
		}
		if sub.Insecure == "" {
			sub.Insecure = "false"
		}
		if sub.FastOpen == "" {
			sub.FastOpen = "false"
		}
		sub.ID = profile.ID()
	}
	return a.refresh(sub, create)
}

func (a *App) RefreshSubscription(id string) (int, error) {
	a.mu.Lock()
	var sub profile.Subscription
	for _, item := range a.config.Subscriptions {
		if item.ID == id {
			sub = item
			break
		}
	}
	a.mu.Unlock()
	if sub.ID == "" {
		return 0, errors.New("订阅不存在")
	}
	return a.refresh(sub, false)
}

func (a *App) downloadSubscription(sub profile.Subscription, customHeaders ...map[string][]string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, sub.URL, nil)
	if err != nil {
		return "", errors.New("订阅地址必须是有效的 HTTP 或 HTTPS URL")
	}
	agent, fallbackAgent := sub.UserAgent, ""
	if agent == "" {
		agent, fallbackAgent = profile.SubscriptionFormatAgent(req.URL)
		if agent == "" {
			agent = "Lanboat/" + appmeta.Version
		}
	}
	req.Header.Set("User-Agent", agent)
	if len(customHeaders) > 0 {
		for name, values := range customHeaders[0] {
			if len(values) > 0 && strings.EqualFold(name, "User-Agent") {
				fallbackAgent = ""
			}
			for i, value := range values {
				if i == 0 {
					req.Header.Set(name, value)
				} else {
					req.Header.Add(name, value)
				}
			}
		}
	}
	client := a.client
	if sub.Insecure == "true" {
		copy := *a.client
		transport := a.client.Transport.(*http.Transport).Clone()
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
		defer transport.CloseIdleConnections()
		copy.Transport = transport
		client = &copy
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", subscriptionDownloadError(err)
	}
	if resp.StatusCode == http.StatusForbidden && fallbackAgent != "" {
		resp.Body.Close()
		retry := req.Clone(req.Context())
		retry.Header.Set("User-Agent", fallbackAgent)
		resp, err = client.Do(retry)
		if err != nil {
			return "", subscriptionDownloadError(err)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("订阅服务器返回 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, profile.MaxSubscription+1))
	if err != nil {
		return "", errors.New("读取订阅内容失败")
	}
	if len(b) > profile.MaxSubscription {
		return "", errors.New("订阅内容不能超过 32 MiB")
	}
	content := string(b)
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("subscription-encryption")), "true") {
		content, err = profile.DecryptSubscription(content, sub.Password)
		if err != nil {
			return "", err
		}
	}
	return content, nil
}

func (a *App) refresh(sub profile.Subscription, create bool) (int, error) {
	content, err := a.downloadSubscription(sub)
	if err != nil {
		return 0, err
	}
	content, err = a.expandClashProviders(content, sub)
	if err != nil {
		return 0, err
	}
	nodes, rules, err := profile.ParseSubscription(content, sub, "[Rule]\n")
	if err != nil {
		return 0, err
	}
	a.fillNodeFlags(nodes)
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Nodes = []profile.Node{}
	c.Subscriptions = append([]profile.Subscription{}, c.Subscriptions...)
	old := map[string]profile.Node{}
	for _, n := range a.config.Nodes {
		if n.Source == sub.ID {
			old[profile.URI(n)] = n
		} else {
			c.Nodes = append(c.Nodes, n)
		}
	}
	activeRetained := false
	for i := range nodes {
		nodes[i].Source = sub.ID
		if prior, ok := old[profile.URI(nodes[i])]; ok {
			nodes[i].ID = prior.ID
			if prior.Flag != "" {
				nodes[i].Flag = prior.Flag
			}
			if prior.ID == a.activeNode && a.connectionCancel != nil {
				nodes[i] = prior
				activeRetained = true
			}
		}
	}
	c.Nodes = append(c.Nodes, nodes...)
	if !activeRetained && a.connectionCancel != nil {
		for _, prior := range old {
			if prior.ID == a.activeNode {
				c.Nodes = append(c.Nodes, prior)
				break
			}
		}
	}

	var owned []profile.Node
	for _, node := range c.Nodes {
		if node.Source == sub.ID {
			owned = append(owned, node)
		}
	}
	if err := syncSubscriptionRules(&c, sub, rules, owned); err != nil {
		return 0, err
	}

	if len(c.Nodes) > profile.MaxNodes {
		return 0, errors.New("更新后节点数超过 100000，已保留原配置")
	}
	selected := false
	for _, n := range c.Nodes {
		if n.ID == c.Selected {
			selected = true
			break
		}
	}
	if !selected {
		c.Selected = nodes[0].ID
	}
	sub.UpdatedAt = time.Now().Format(time.RFC3339)
	found := false
	for i, old := range c.Subscriptions {
		if old.ID == sub.ID {
			c.Subscriptions[i] = sub
			found = true
			break
		}
	}
	if !found {
		if !create {
			return 0, errors.New("订阅已被删除，未应用本次更新")
		}
		if len(c.Subscriptions) >= 30 {
			return 0, errors.New("最多保存 30 个订阅")
		}
		c.Subscriptions = append(c.Subscriptions, sub)
	}
	if err := a.commit(c); err != nil {
		return 0, err
	}
	a.log.Info("subscription updated", zap.Int("count", len(nodes)))
	return len(nodes), nil
}

func (a *App) DeleteSubscription(id string) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	if err := syncSubscriptionRules(&c, profile.Subscription{ID: id}, "", nil); err != nil {
		return err
	}
	c.Settings.AutoSelectGroups = slices.DeleteFunc(slices.Clone(c.Settings.AutoSelectGroups), func(group string) bool { return group == id })
	c.Subscriptions = []profile.Subscription{}
	c.Nodes = []profile.Node{}
	for _, s := range a.config.Subscriptions {
		if s.ID != id {
			c.Subscriptions = append(c.Subscriptions, s)
		}
	}
	for _, n := range a.config.Nodes {
		if n.Source != id {
			c.Nodes = append(c.Nodes, n)
		} else if n.ID == c.Selected {
			c.Selected = ""
		}
	}
	if c.Selected == "" && len(c.Nodes) > 0 {
		c.Selected = c.Nodes[0].ID
	}
	return a.commit(c)
}

func (a *App) SaveRules(rules []profile.Rule) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(rules) > 500 {
		return errors.New("最多保存 500 条规则")
	}
	for i := range rules {
		if err := rules[i].Validate(); err != nil {
			return err
		}
		if rules[i].ID == "" {
			rules[i].ID = profile.ID()
		}
	}
	c := a.config
	c.Rules = rules
	return a.commit(c)
}

func (a *App) SaveSettings(settings profile.Settings) error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.config.Settings
	settings.AutoSelectGroups = current.AutoSelectGroups
	settings.LegacyAutoSelect = false
	settings.SystemProxy = false
	settings.TestMethod = ""
	settings.SpeedURLs = nil
	settings.TestURLs = nil
	if settings.UDP.Relay != nil {
		relay := *settings.UDP.Relay
		settings.UDP.Relay = &relay
	}
	if settings.DNS != nil {
		dns := *settings.DNS
		settings.DNS = &dns
	}
	if settings.Language == "" {
		settings.Language = current.Language
	}

	if err := settings.Validate(); err != nil {
		return err
	}
	if settings.SystemProxy && !ProxySupported() {
		return errors.New("当前桌面不支持 GNOME 自动系统代理")
	}
	c := a.config
	c.Settings = settings
	if settings.DNS != nil && settings.DNS.ProxyRoute == "node" {
		found := false
		for _, n := range c.Nodes {
			if n.ID == settings.DNS.ProxyNode {
				found = true
				break
			}
		}
		if !found {
			return errors.New("DNS出站节点不存在")
		}
	}
	err := a.commit(c)
	if err == nil {
		a.pruneLogs()
	}
	return err
}

func (a *App) Connect() error {
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	if a.connectionCancel != nil {
		a.mu.Unlock()
		return errors.New("请先断开连接")
	}
	c := a.config
	ctx, cancel := context.WithCancel(context.Background())
	a.connectionCtx = ctx
	a.connectionCancel = cancel
	a.switching = true
	a.connectionError = ""
	a.mu.Unlock()
	var err error
	previousSelection := c.Selected
	if c.Settings.Mode != "direct" {
		group := c.SelectedNodeGroup()
		if c.Settings.AutoSelectEnabled(group) {
			c.Selected, err = a.fastestNode(ctx, c, group)
		}
	}
	route, routeErr := profile.EffectiveRouting(c)
	if err == nil {
		err = routeErr
	}
	if err == nil {
		c.Routing = &route
		err = a.engine.StartContext(ctx, c)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && c.Settings.SystemProxy && c.Settings.Mode != "tun" {
		a.mu.Lock()
		err = a.proxy.Enable(c.Settings.Port)
		a.mu.Unlock()
	}
	if err == nil && c.Selected != previousSelection {
		a.mu.Lock()
		err = a.commit(c)
		a.mu.Unlock()
	}
	if err != nil {
		cancel()
		a.engine.Stop()
		a.mu.Lock()
		a.connectionCancel = nil
		a.connectionCtx = nil
		a.activeNode = ""
		a.switching = false
		err = errors.Join(err, a.proxy.Restore())
		a.mu.Unlock()
		if !errors.Is(err, context.Canceled) {
			a.log.Error("connect failed", zap.String("mode", c.Settings.Mode), zap.Error(err))
		}
		return err
	}
	fields := []zap.Field{zap.String("mode", c.Settings.Mode), zap.Bool("systemProxy", c.Settings.SystemProxy && c.Settings.Mode != "tun")}
	if c.Settings.Mode != "direct" {
		fields = append(fields, zap.String("node", nodeName(c, c.Selected)))
	}
	a.log.Info("connected", fields...)
	a.mu.Lock()
	a.connectedAt = a.engine.State().StartedAt
	a.activeNode = c.Selected
	if c.Settings.Mode == "direct" {
		a.activeNode = ""
	}
	a.switching = false
	a.mu.Unlock()
	go debug.FreeOSMemory()
	return nil
}

func (a *App) Disconnect() error {
	a.mu.Lock()
	connected := a.connectionCancel != nil
	if connected {
		a.connectionCancel()
		defer debug.FreeOSMemory()
	}
	a.mu.Unlock()
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.engine.Stop()
	if connected {
		a.log.Info("disconnected")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connectionCancel = nil
	a.connectionCtx = nil
	a.activeNode = ""
	a.switching = false
	a.connectedAt = 0
	a.connectionError = ""
	return a.proxy.Restore()
}

func (a *App) Shutdown(_ context.Context) {
	if err := a.Disconnect(); err != nil {
		a.log.Error("system proxy restore failed")
		a.engine.Stop()
	}
	_ = a.log.Sync()
}

func nodeName(c profile.Config, id string) string {
	for _, n := range c.Nodes {
		if n.ID == id {
			return n.Name
		}
	}
	return id
}
