package native

import (
	"context"
	"fmt"
	"github.com/goccy/go-json"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zatxm/lanboat-app/internal/desktop"
	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"github.com/zatxm/lanboat-app/internal/traffic"
)

const pageSize = 40

type field struct{ value string }

func (f *field) SetText(value string) { f.value = value }
func (f *field) Text() string         { return f.value }

type update struct {
	network      *traffic.Page
	networkQuery logQuery
	version      int64
	state        *desktop.State
	status       desktop.Status
	err          error
	job          bool
	background   bool
	apply        func()
}
type deletion struct {
	kind, id, name string
	prompt         string
	run            func()
}
type UI struct {
	flags                              map[string]flagEntry
	flagGeneration                     string
	flagIndex                          ruledata.CountryIndex
	flagsEnabled, flagsBusy            bool
	flagHosts                          []string
	networkQuery                       atomic.Value
	networkPolling                     atomic.Bool
	appLogPolling                      atomic.Bool
	memoryVisible                      atomic.Bool
	flagDeadline                       atomic.Int64
	networkPage                        traffic.Page
	history                            []screen
	popup                              *popup
	expanded                           map[string]bool
	fileID, detail                     string
	result                             string
	version                            atomic.Int64
	traffic                            atomic.Bool
	backend                            *desktop.App
	state                              desktop.State
	status                             desktop.Status
	page, editor                       string
	draft                              profile.Node
	rule                               profile.Rule
	confirm                            *deletion
	busy                               bool
	actionID, busyControl, testControl int
	testing                            bool
	testNotice                         string
	savingSettings                     bool
	err, notice                        string
	fields                             map[string]*field
	delays                             map[string]int64
	connectedProbe                     context.CancelFunc
	connectedProbeID                   string
	connectedDelay                     int64
	nodePage                           int
	updates                            chan update
	done                               chan struct{}
	notify                             func()
	view                               *nativeView
	dirty, nodesDirty, resetScroll     bool
	focus                              string
}

func newUI(backend *desktop.App, notify func()) *UI {
	u := &UI{flags: map[string]flagEntry{}, backend: backend, page: "home", expanded: map[string]bool{"": true}, fields: map[string]*field{}, delays: map[string]int64{}, updates: make(chan update, 4), done: make(chan struct{}), notify: notify}
	u.networkQuery.Store(logQuery{Kind: "proxy"})
	u.networkPage.Kind = "proxy"
	u.state = backend.GetState()
	for id, ms := range u.state.NodeDelays {
		u.delays[id] = ms
	}
	for _, n := range u.state.Config.Nodes {
		if n.ID == u.state.Config.Selected {
			u.expanded[n.Source] = true
		}
	}
	u.status = backend.GetConnectionStatus()
	u.field("port").SetText(strconv.Itoa(u.state.Config.Settings.Port))
	return u
}
func (u *UI) field(id string) *field {
	if f := u.fields[id]; f != nil {
		return f
	}
	f := &field{}
	u.fields[id] = f
	return f
}
func (u *UI) render() { u.dirty = true; u.notify() }
func (u *UI) post(next update) {
	select {
	case <-u.done:
		return
	default:
	}
	select {
	case u.updates <- next:
		u.notify()
	case <-u.done:
	}
}
func (u *UI) snapshot() desktop.Status {
	var status desktop.Status
	if u.traffic.Load() {
		status = u.backend.GetStatus()
	} else {
		status = u.backend.GetConnectionStatus()
	}
	if u.memoryVisible.Load() {
		status.Memory = u.backend.MemoryUsage()
	}
	return status
}
func (u *UI) poll() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastStatus desktop.Status
	var lastVersion int64 = -1
	for {
		select {
		case <-u.done:
			return
		case now := <-ticker.C:
			version := u.version.Load()
			status := u.snapshot()
			deadline := u.flagDeadline.Load()
			flagsDue := deadline != 0 && now.UnixNano() >= deadline
			next := update{version: version, status: status}
			if u.networkPolling.Load() {
				q := u.networkQuery.Load().(logQuery)
				if p, err := u.backend.NetworkLogs(q.Kind, q.Page); err == nil {
					next.network = &p
					next.networkQuery = q
				}
			}
			if status == lastStatus && version == lastVersion && !flagsDue &&
				next.network == nil && !u.appLogPolling.Load() && !(u.traffic.Load() && status.Connected) {
				continue
			}
			lastStatus, lastVersion = status, version
			u.post(next)
		}
	}
}
func (u *UI) submit(fn func() error, apply func()) {
	if u.busy {
		return
	}
	readState := !u.savingSettings
	u.busy = true
	u.busyControl = u.actionID
	u.notify()
	u.version.Add(1)
	u.err = ""
	u.notice = ""
	go func() {
		err := fn()
		next := update{job: true, err: err, apply: apply}
		if err == nil && readState {
			s := u.backend.GetState()
			next.state = &s
		}
		next.status = u.snapshot()
		u.post(next)
	}()
}
func (u *UI) drain() {
	for {
		select {
		case next := <-u.updates:
			if next.background {
				if next.apply != nil {
					next.apply()
				}
				continue
			}
			if !next.job && next.version != u.version.Load() {
				continue
			}
			if next.status.Memory.Processes == 0 && u.memoryVisible.Load() {
				next.status.Memory = u.status.Memory
			}
			u.status = next.status
			if next.network != nil && u.networkPolling.Load() && next.networkQuery == u.networkQuery.Load().(logQuery) {
				u.networkPage = *next.network
			}
			if next.job {
				u.busy = false
				u.busyControl = 0
				u.savingSettings = false
				u.version.Add(1)
				if next.err != nil {
					u.err = next.err.Error()
				} else {
					if next.state != nil {
						for id, ms := range next.state.NodeDelays {
							u.delays[id] = ms
						}
						before, after := u.state.Config, next.state.Config
						u.state = *next.state
						if before.Settings.Port != after.Settings.Port && u.field("port").Text() == strconv.Itoa(before.Settings.Port) {
							u.field("port").SetText(strconv.Itoa(after.Settings.Port))
						}
						if !reflect.DeepEqual(before.Nodes, after.Nodes) || !reflect.DeepEqual(before.Subscriptions, after.Subscriptions) {
							if u.page == "home" && u.editor == "" && u.detail == "" {
								u.nodesDirty = true
							} else {
								u.render()
							}
						}
						if !reflect.DeepEqual(before.Files, after.Files) || !reflect.DeepEqual(before.Rules, after.Rules) {
							if u.page != "home" {
								u.render()
							}
						}
					}
					beforePage, beforeEditor, beforeDetail := u.page, u.editor, u.detail
					if next.apply != nil {
						next.apply()
					}
					if beforePage != u.page || beforeEditor != u.editor || beforeDetail != u.detail {
						u.notice = ""
					}
				}
			}
		default:
			u.probeConnectedNode()
			return
		}
	}
}

type screen struct {
	page, editor, detail, fileID, result string
	draft                                profile.Node
	rule                                 profile.Rule
	fields                               map[string]string
}

func (u *UI) pushScreen() {
	f := map[string]string{}
	for key, value := range u.fields {
		f[key] = value.Text()
	}
	u.history = append(u.history, screen{u.page, u.editor, u.detail, u.fileID, u.result, u.draft.Clone(), u.rule, f})
}
func (u *UI) back() {
	if u.busy {
		return
	}
	if u.confirm != nil {
		u.confirm = nil
		u.notify()
		return
	}
	if len(u.history) == 0 {
		u.navigate("home")
		return
	}
	last := u.history[len(u.history)-1]
	u.history[len(u.history)-1] = screen{}
	u.history = u.history[:len(u.history)-1]
	u.page, u.editor, u.detail, u.fileID, u.result = last.page, last.editor, last.detail, last.fileID, last.result
	u.draft, u.rule = last.draft, last.rule
	u.fields = map[string]*field{}
	for key, value := range last.fields {
		u.field(key).SetText(value)
	}
	if _, ok := last.fields["port"]; !ok {
		u.field("port").SetText(strconv.Itoa(u.state.Config.Settings.Port))
	}
	u.err = ""
	u.notice = ""
	u.resetScroll = true
	u.render()
}
func (u *UI) navigate(page string) {
	u.history = nil
	if page != "home" {
		u.history = append(u.history, screen{page: "home", fields: map[string]string{}})
	}
	u.clearEditor()
	u.detail = ""
	u.result = ""
	u.page = page
	u.confirm = nil
	u.err = ""
	u.notice = ""
	u.resetScroll = true
	u.render()
}
func (u *UI) openDetail(detail string) {
	if u.detail != detail || u.editor != "" {
		u.pushScreen()
	}
	u.detail = detail
	u.editor = ""
	u.result = ""
	u.resetScroll = true
	u.render()
}
func (u *UI) openEditor(kind string) {
	if u.editor != kind {
		u.pushScreen()
	}
	u.editor = kind
	u.confirm = nil
	u.err = ""
	u.resetScroll = true
	u.render()
}
func (u *UI) clearEditor() {
	u.editor = ""
	u.field("password").SetText("")
	u.field("subPassword").SetText("")
	for key, f := range u.fields {
		if strings.HasPrefix(key, "opt.") {
			f.SetText("")
		}
	}
	u.field("content").SetText("")
	u.field("fileContent").SetText("")
}
func (u *UI) settings(mode string) {
	u.changeSettings(func(s *profile.Settings) { s.Mode = mode })
}

func (u *UI) toggleAutoSelect(group string) {
	enabled := !u.state.Config.Settings.AutoSelectEnabled(group)
	u.submit(func() error { return u.backend.SetAutoSelect(group, enabled) }, nil)
}
func (u *UI) editNode(node profile.Node) {
	if node.Kind() == "mieru" {
		node = node.Clone()
		if value := node.TextOption("transport"); value != "" {
			node.SetOption("transport", strings.ToLower(value))
		}
	}
	if node.Kind() == "brook" && node.TextOption("network") == "tcp" {
		node = node.Clone()
		node.SetOption("network", "none")
	}
	if u.editor != "node" {
		for _, key := range []string{"subURL", "subUDP", "subInsecure", "subFastOpen", "subMax", "subUA", "subPassword", "subFilter", "subKeepRules"} {
			u.field(key).SetText("")
		}
		for _, key := range []string{"subUDP", "subInsecure", "subFastOpen"} {
			u.field(key).SetText("false")
		}
	}
	if node.Port == 0 {
		node.Port = 443
		if node.Kind() == "ss" || node.Kind() == "ssr" {
			node.Port = 8388
		}
	}
	if node.Kind() == "ssr" && node.Method == "" {
		node.Method = node.TextOption("cipher")
	}
	if node.Method == "" {
		node.Method = profile.DefaultSSMethod
	}
	node = node.Clone()
	profile.NormalizeSS(&node)
	if node.TextOption("xhttp-extra") != "" {
		if extra, err := node.XHTTPExtraSettings(); err == nil {
			if node.Option("xhttp.NoGRPCHeader") == nil {
				node.SetOption("xhttp.NoGRPCHeader", extra.NoGRPCHeader)
			}
			if node.Option("xhttp.XPaddingBytes") == nil && len(extra.Padding) > 0 {
				value := string(extra.Padding)
				if strings.HasPrefix(value, "\"") {
					_ = json.Unmarshal(extra.Padding, &value)
				}
				node.SetOption("xhttp.XPaddingBytes", value)
			}
		}
	}
	u.draft = node
	for _, spec := range profile.NodeFields(node) {
		value := node.TextOption(spec.Key)
		if spec.Key == "plugin" {
			value = node.Plugin
		}
		if value == "" {
			value = spec.Default
		}
		u.field("opt." + spec.Key).SetText(value)
	}
	for key, value := range map[string]string{"name": node.Name, "server": node.Server, "nodePort": strconv.Itoa(node.Port), "password": "", "pluginOpts": node.PluginOpts} {
		u.field(key).SetText(value)
	}
	u.openEditor("node")
}

func (u *UI) editSubscription(s profile.Subscription) {
	u.editNode(profile.Node{Type: "subscribe", ID: s.ID, Name: s.Name})
	maximum := ""
	if s.MaxNodes > 0 {
		maximum = strconv.Itoa(s.MaxNodes)
	}
	for key, value := range map[string]string{"subURL": s.URL, "subUDP": s.UDP, "subInsecure": s.Insecure, "subFastOpen": s.FastOpen, "subMax": maximum, "subUA": s.UserAgent, "subPassword": "", "subFilter": s.Filter, "subKeepRules": strconv.FormatBool(s.KeepRules)} {
		if key == "subUDP" || key == "subInsecure" || key == "subFastOpen" {
			value = strconv.FormatBool(value == "true")
		}
		u.field(key).SetText(value)
	}
	u.render()
}

func (u *UI) saveEditor() {
	var fn func() error
	switch u.editor {
	case "file-rule":
		u.saveRoutingRule()
		return
	case "routing-group":
		u.saveRoutingGroup()
		return
	case "node":
		if u.draft.Type == "subscribe" {
			name, address := strings.TrimSpace(u.field("name").Text()), strings.TrimSpace(u.field("subURL").Text())
			if name == "" {
				if parsed, err := url.Parse(address); err == nil {
					name = parsed.Hostname()
				}
			}
			maximum := 0
			if value := strings.TrimSpace(u.field("subMax").Text()); value != "" {
				var err error
				maximum, err = strconv.Atoi(value)
				if err != nil || maximum < 1 || maximum > profile.MaxNodes {
					u.err = "最大值需为 1–100000，留空表示不限"
					return
				}
			}
			sub := profile.Subscription{ID: u.draft.ID, Name: name, URL: address, UDP: u.field("subUDP").Text(), Insecure: u.field("subInsecure").Text(), FastOpen: u.field("subFastOpen").Text(), MaxNodes: maximum, UserAgent: u.field("subUA").Text(), Password: u.field("subPassword").Text(), Filter: u.field("subFilter").Text(), KeepRules: u.field("subKeepRules").Text() == "true"}
			fn = func() error { _, err := u.backend.SaveSubscriptionOptions(sub); return err }
			break
		}
		n := u.draft.Clone()
		n.Name = u.field("name").Text()
		n.Server = u.field("server").Text()
		n.Password = u.field("password").Text()
		if profile.DetailedProtocol(n.Kind()) && profile.NodePasswordField(n.Kind()) && n.Kind() != "shadowtls" && n.ID == "" && n.Password == "" {
			u.err = "请填写密码"
			return
		}
		n.PluginOpts = u.field("pluginOpts").Text()
		portText := u.field("nodePort").Text()
		if n.Kind() == "tailscale" {
			portText = "443"
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			u.err = "服务器端口必须是数字"
			return
		}
		n.Port = port
		if strings.TrimSpace(n.Name) == "" {
			if n.Kind() == "tailscale" {
				n.Name = u.field("opt.exit-node").Text()
			} else {
				n.Name = ssRemark(n.Server, n.Port)
			}
		}
		if n.Kind() == "tailscale" {
			n.Server = "login.tailscale.com"
			n.Port = 443
		}
		for _, spec := range profile.NodeFields(n) {
			value := u.field("opt." + spec.Key).Text()
			if spec.Key == "plugin" {
				n.Plugin = value
				if n.Plugin == "none" {
					n.Plugin = ""
				}
				continue
			}
			if spec.Secret && value == "" && n.ID != "" {
				continue
			}
			if value == "" {
				n.SetOption(spec.Key, "")
				continue
			}
			var v any = value
			if spec.Kind == "number" {
				number, err := strconv.Atoi(value)
				if err != nil {
					u.err = spec.Title + "必须是数字"
					return
				}
				v = number
			}
			if value == "true" || value == "false" {
				v = value == "true"
			}
			n.SetOption(spec.Key, v)
		}
		fn = func() error { return u.backend.SaveNode(n) }
	case "dns-settings":
		u.saveDNS()
		return
	case "import":
		if u.field("nodeImportMethod").Text() == "subscription" {
			address := strings.TrimSpace(u.field("nodeImportURL").Text())
			keepRules := u.field("nodeImportRules").Text() == "true"
			fn = func() error { _, err := u.backend.ImportSubscription(address, keepRules); return err }
			break
		}
		content := u.field("content").Text()
		if u.field("nodeImportMethod").Text() == "file" {
			content = u.field("nodeImportPath").Text()
		}
		fn = func() error { _, err := u.backend.Import(content); return err }
	case "subscription":
		name, url := u.field("subName").Text(), u.field("subURL").Text()
		fn = func() error { _, err := u.backend.SaveSubscription(name, url); return err }
	case "config-file", "config-import":
		id, name, content := u.fileID, u.field("fileName").Text(), u.field("fileContent").Text()
		if strings.TrimSpace(name) == "" {
			u.err = "请填写名称"
			return
		}
		if strings.TrimSpace(content) == "" {
			u.err = "配置内容不能为空"
			return
		}
		fn = func() error { return u.backend.SaveConfigFile(id, name, content) }
	case "restore-backup":
		content := u.field("content").Text()
		fn = func() error { return u.backend.ImportBackup(content) }
	case "rule":
		r := u.rule
		r.Value = u.field("ruleValue").Text()
		r.NoResolve = u.field("custom.noResolve").Text() == "true"
		rules := append([]profile.Rule{}, u.state.Config.Rules...)
		if r.ID == "" {
			rules = append(rules, r)
		} else {
			for i := range rules {
				if rules[i].ID == r.ID {
					rules[i] = r
					break
				}
			}
		}
		fn = func() error { return u.backend.SaveRules(rules) }
	default:
		return
	}
	u.submit(fn, u.back)
}

func filterNodes(nodes []profile.Node, query string) []profile.Node {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nodes
	}
	filtered := []profile.Node{}
	for _, n := range nodes {
		if strings.Contains(strings.ToLower(n.Name+" "+n.Server+" "+n.Method+" "+n.Kind()), query) {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func pageBounds(count, page int) (int, int, int) {
	pages := max(1, (count+pageSize-1)/pageSize)
	page = max(0, min(page, pages-1))
	start := page * pageSize
	return start, min(count, start+pageSize), page
}

func (u *UI) locked() bool { return u.busy }
func (u *UI) nodeLocked(id string) bool {
	active := u.status.ActiveNode
	if active == "" && u.state.Config.Settings.Mode != "direct" {
		active = u.state.Config.Selected
	}
	return u.status.Connected && id != "" && id == active
}
func (u *UI) deleteConfirmed() {
	d := u.confirm
	if d == nil {
		return
	}
	var removed []string
	for _, n := range u.state.Config.Nodes {
		if d.kind == "allnodes" || d.kind == "node" && n.ID == d.id || d.kind == "subscription" && n.Source == d.id {
			removed = append(removed, n.ID)
		}
	}
	u.submit(func() error {
		if d.kind == "allnodes" {
			return u.backend.DeleteAllNodes()
		}
		if d.kind == "file" {
			return u.backend.DeleteConfigFile(d.id)
		}
		if d.kind == "logs" {
			return u.backend.ClearLogs()
		}
		if d.kind == "subscription" {
			return u.backend.DeleteSubscription(d.id)
		}
		return u.backend.DeleteNode(d.id)
	}, func() {
		for _, id := range removed {
			delete(u.delays, id)
		}
		u.confirm = nil
		u.notice = ""
		if u.page == "home" && u.editor == "" && u.detail == "" {
			u.nodesDirty = true
			u.notify()
		} else {
			u.render()
		}
	})
}
func (u *UI) connect() {
	connected := u.status.Connected
	u.submit(func() error {
		if connected {
			return u.backend.Disconnect()
		}
		return u.backend.Connect()
	}, nil)
}
func (u *UI) probeConnectedNode() {
	if u.status.Switching {
		return
	}
	id := u.status.ActiveNode
	if !u.status.Connected {
		id = ""
	}
	if id == u.connectedProbeID {
		return
	}
	if u.connectedProbe != nil {
		u.connectedProbe()
		u.connectedProbe = nil
	}
	u.connectedProbeID, u.connectedDelay = id, 0
	if id == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.connectedProbe = cancel
	go func() {
		ms, err := u.backend.TestNodeContext(ctx, id)
		if err != nil {
			ms = -1
		}
		u.post(update{background: true, apply: func() {
			if ctx.Err() == nil && u.status.Connected && u.status.ActiveNode == id {
				u.connectedDelay = ms
			}
		}})
	}()
}

func formatBytes(value int64) string {
	if value < 1000 {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	n := float64(value) / 1000
	i := 0
	for n >= 1000 && i < len(units)-1 {
		n /= 1000
		i++
	}
	if n < 10 {
		return fmt.Sprintf("%.1f %s", n, units[i])
	}
	return fmt.Sprintf("%.0f %s", n, units[i])
}
func (u *UI) openPage(page string) {
	u.pushScreen()
	u.page = page
	u.detail = ""
	u.editor = ""
	u.resetScroll = true
	u.render()
}
