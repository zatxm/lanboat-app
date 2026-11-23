package native

import (
	"fmt"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"strconv"
	"strings"
	"time"
)

type logQuery struct {
	Kind string
	Page int
}

func (u *UI) openNetworkLogs() {
	u.networkPage = traffic.Page{Kind: "proxy"}
	u.openDetail("network-logs")
	u.fetchNetworkLogs("proxy", 0)
}
func (u *UI) fetchNetworkLogs(kind string, page int) {
	if u.busy {
		return
	}
	if kind == "app" {
		u.networkQuery.Store(logQuery{kind, 0})
		u.networkPolling.Store(false)
		u.appLogPolling.Store(true)
		u.networkPage = traffic.Page{Kind: kind}
		u.notify()
		return
	}
	u.networkQuery.Store(logQuery{kind, page})
	u.appLogPolling.Store(false)
	u.networkPolling.Store(page == 0)
	var result traffic.Page
	u.savingSettings = true
	u.submit(func() error { var err error; result, err = u.backend.NetworkLogs(kind, page); return err }, func() { u.networkPage = result; u.logQueryResult() })
}
func (u *UI) logQueryResult() {
	u.networkQuery.Store(logQuery{u.networkPage.Kind, u.networkPage.Number})
	u.notify()
}
func (u *UI) confirmClearLogs() {
	u.confirm = &deletion{kind: "logs", name: "清空日志"}
	u.notify()
}
func (v *nativeView) networkLogPage() {
	u := v.ui
	tab := func(kind, title string) *widget {
		b := v.button(title, "backup-tab no-icon", false, false, func() { u.fetchNetworkLogs(kind, 0) })
		b.update = func() {
			class := "button backup-tab no-icon"
			if u.networkQuery.Load().(logQuery).Kind == kind {
				class += " active"
			}
			b.setClasses(class)
		}
		return b
	}
	refresh := v.button("刷新", "link icon-only icon-refresh", false, false, func() { u.fetchNetworkLogs(u.networkPage.Kind, u.networkPage.Number) })
	v.pack(v.body, v.box("", true, 10, false, tab("proxy", "代理日志"), tab("dns", "DNS日志"), tab("app", "应用日志"), v.spacer(), refresh, v.button("清空日志", "link icon-only icon-delete", false, false, u.confirmClearLogs)))
	v.logContainer = v.box("", false, 0, false)
	v.listViewport(v.logContainer)
	v.logPager = v.box("", true, 8, false)
	v.pack(v.body, v.logPager)
	v.refreshNetworkLogs()
}

// trafficSummary is the one-line description under a proxy or DNS record.
func (u *UI) trafficSummary(e traffic.Entry) string {
	var parts []string
	add := func(s string) {
		if s != "" {
			parts = append(parts, s)
		}
	}
	switch {
	case e.Kind == "dns":
		add(e.QueryType)
		add(e.Resolver)
		add(strconv.FormatInt(e.DurationMS, 10) + "ms")
		if rcode, _, _ := strings.Cut(e.Result, "\n"); rcode != "" {
			add(rcode)
		}
	case e.Method != "":
		add("HTTP " + e.Method)
		if e.Incomplete {
			add(u.view.translate("请求未完整发送"))
		}
	default:
		add(strings.ToUpper(e.Network))
		policy := u.policyText(e.Policy)
		if e.Outbound != "" {
			policy += " → " + u.policyText(e.Outbound)
		}
		add(policy)
		add(e.Rule)
		if e.Error == "" {
			add(logDuration(e.DurationMS))
			add("↑" + formatBytes(e.Upload) + " ↓" + formatBytes(e.Download))
		}
		add(e.Process)
	}
	return strings.Join(parts, " · ")
}

func logDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return strconv.FormatInt(ms, 10) + "ms"
	case d < time.Minute:
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

func (v *nativeView) refreshNetworkLogs() {
	u := v.ui
	if u.busy {
		return
	}
	if u.networkQuery.Load().(logQuery).Kind == "app" {
		v.refreshAppLogs()
		return
	}
	p := u.networkPage
	signature := fmt.Sprintf("%s:%d:%d:%d", p.Kind, p.Number, p.Revision, p.Total)
	if signature == v.logSignature {
		return
	}
	v.logSignature = signature
	v.clear(v.logContainer)
	v.clear(v.logPager)
	for i, e := range p.Entries {
		e := e
		if i > 0 {
			v.pack(v.logContainer, v.create("separator", props{}))
		}
		target := e.URL
		if e.Kind == "dns" {
			target = strings.TrimSuffix(e.Domain, ".")
		}
		lines := []*widget{v.dataLabel(target, "node-name node-title"), v.dataLabel(logTime(e.Time)+"  ·  "+u.trafficSummary(e), "help node-title")}
		if e.Error != "" {
			lines = append(lines, v.dataLabel(e.Error, "log-error node-title"))
		}
		row := v.box("node-row", true, 10, false, v.box("", false, 6, true, lines...), v.button("日志详情", "link icon-only icon-info", false, false, func() { u.popup = &popup{kind: "network-log", title: "日志详情", networkLog: &e}; u.notify() }))
		v.pack(v.logContainer, row)
	}
	if len(p.Entries) == 0 {
		v.pack(v.logContainer, v.box("empty", false, 0, false, v.label("暂无日志", "help")))
	}
	v.logPager.setVisible(p.Total > traffic.PageSize)
	if p.Total <= traffic.PageSize {
		return
	}
	pages := (p.Total + traffic.PageSize - 1) / traffic.PageSize
	v.pack(v.logPager, v.pageBar(p.Number, pages, p.Total, func(next int) {
		v.logSignature = ""
		u.fetchNetworkLogs(p.Kind, next)
	}))
}

// refreshAppLogs lists the app's own events, newest first.
func (v *nativeView) refreshAppLogs() {
	logs := v.ui.backend.GetLogs()
	signature := "app:" + strconv.Itoa(len(logs))
	if n := len(logs); n > 0 {
		signature += ":" + logs[n-1].Time + logs[n-1].Message
	}
	if signature == v.logSignature {
		return
	}
	v.logSignature = signature
	v.clear(v.logContainer)
	v.clear(v.logPager)
	v.logPager.setVisible(false)
	for i := len(logs) - 1; i >= 0; i-- {
		e := logs[i]
		if i < len(logs)-1 {
			v.pack(v.logContainer, v.create("separator", props{}))
		}
		level := v.dataLabel(strings.ToUpper(e.Level), "log-level level-"+e.Level)
		head := v.box("", true, 10, false, level, v.dataLabel(logTime(e.Time), "help"))
		v.pack(v.logContainer, v.box("node-row", false, 6, false, head, v.dataLabel(e.Message, "log-message")))
	}
	if len(logs) == 0 {
		v.pack(v.logContainer, v.box("empty", false, 0, false, v.label("暂无日志", "help")))
	}
}

func (v *nativeView) networkLogDetail(box *widget, e *traffic.Entry) {
	content := v.create("scroll", props{expand: true})
	fields := v.box("", false, 14, false)
	row := func(title, value string) {
		if value != "" {
			v.pack(fields, v.box("", false, 5, false, v.label(title, "field-label"), v.dataLabel(value, "node-name")))
		}
	}
	row("时间", e.Time)
	if e.Kind == "proxy" {
		row("请求地址", e.URL)
		row("请求方法", e.Method)
		if e.Incomplete {
			row("日志状态", v.translate("请求未完整发送"))
		} else if e.Error != "" {
			row("日志状态", v.translate("连接尝试失败"))
		} else if e.Method == "" {
			row("日志状态", v.translate("连接已结束"))
		}
		if strings.Contains(e.Error, "HY2 connect:") && strings.Contains(e.Error, "tcp6") && strings.Contains(e.Error, "network is unreachable") {
			row("说明", v.translate("HY2出口无法到达这个IPv6地址。同一网站的其他连接可能已成功。"))
		}
		row("传输协议", e.Network)
		row("匹配规则", e.Rule)
		row("策略", v.ui.policyText(e.Policy))
		if e.Outbound != "" {
			row("出站", v.ui.policyText(e.Outbound))
		}
		if e.Upload > 0 || e.Download > 0 {
			row("流量", "↑"+formatBytes(e.Upload)+"  ↓"+formatBytes(e.Download))
		}
		row("进程", e.Process)
		if e.DurationMS > 0 {
			row("持续时间", logDuration(e.DurationMS))
		}
	} else {
		row("请求域名", e.Domain)
		row("查询类型", e.QueryType)
		row("查询结果", e.Result)
		row("响应时间", strconv.FormatInt(e.DurationMS, 10)+"ms")
		row("DNS服务器", e.Resolver)
	}
	row("错误", e.Error)
	v.pack(content, fields)
	v.pack(box, content, v.box("", true, 8, false, v.spacer(), v.button("复制", "secondary icon-copy", false, false, func() {
		values := []string{e.Time, e.URL, e.Network, e.Rule, e.Policy, e.Outbound, e.Domain, e.Process, e.QueryType, e.Result, e.Resolver, e.Error}
		copyClipboard(strings.Join(values, "\n"))
	})))
}

func logTime(raw string) string {
	// Traffic records use RFC 3339; zap's ISO 8601 encoder omits the zone colon.
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z0700"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Local().Format("2006-01-02 15:04:05.000")
		}
	}
	return raw
}
