package native

import (
	"strings"
)

func iconName(text string) string {
	for _, r := range []struct{ match, name string }{{"代理泄漏检测", "globe"}, {"路由", "route"}, {"返回", "back"}, {"取消", "close"}, {"删除", "delete"}, {"清空", "delete"}, {"导入", "import"}, {"载入", "import"}, {"导出", "export"}, {"下载", "download"}, {"上传", "upload"}, {"分享", "share"}, {"恢复", "refresh"}, {"更新", "refresh"}, {"刷新", "refresh"}, {"保存", "save"}, {"完成", "check"}, {"使用", "check"}, {"添加", "add"}, {"下移", "down"}, {"上移", "up"}, {"上一页", "back"}, {"下一页", "forward"}, {"TCP", "tcp"}, {"UDP", "udp"}, {"主题", "moon"}, {"日志", "logs"}, {"DNS", "dns"}, {"代理", "layers"}, {"订阅", "link"}, {"配置", "folder"}, {"模块", "layers"}, {"测试", "test"}, {"诊断", "info"}, {"详情", "info"}, {"查看", "info"}, {"关于", "info"}, {"备份", "folder"}, {"重命名", "edit"}, {"编辑", "edit"}, {"节点", "server"}} {
		if strings.Contains(text, r.match) {
			return r.name
		}
	}
	return ""
}
func actionIcon(text, kind string) string {
	if strings.Contains(kind, "no-icon") || strings.Contains(kind, "icon-") || strings.Contains(kind, "switch") || strings.Contains(kind, "disclosure") {
		return ""
	}
	if icon := iconName(text); icon != "" {
		return " icon-" + icon
	}
	return ""
}
func (v *nativeView) subscriptionPage(id string) {
	u := v.ui
	for _, sub := range u.state.Config.Subscriptions {
		if sub.ID != id {
			continue
		}
		s := sub
		v.group("", v.dataLabel(s.Name, "section-title"), v.row("更新于", s.UpdatedAt, false, nil), v.row("编辑订阅", "", true, func() { u.editSubscription(s) }), v.row("更新订阅", "", true, func() {
			u.submit(func() error { _, err := u.backend.RefreshSubscription(s.ID); return err }, func() { u.notice = "订阅已更新" })
		}), v.row("分享订阅", "", false, func() { u.share("subscription", s.ID, s.Name) }), v.row("删除订阅", "", true, func() { u.confirm = &deletion{kind: "subscription", id: s.ID, name: s.Name}; u.notify() }))
		return
	}
	v.pack(v.body, v.label("订阅已删除", "help"))
}

// Only comments are renamed; executable rules and URLs retain their exact meaning.
func displayConfigContent(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if !strings.HasPrefix(trim, "#") && !strings.HasPrefix(trim, "//") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "shadowrocket/config") {
			lines[i] = "# Source: historical public configuration index; see the bundled provenance report."
			continue
		}
		for {
			lower := strings.ToLower(line)
			idx := strings.Index(lower, "shadowrocket")
			if idx < 0 {
				break
			}
			line = line[:idx] + "Lanboat" + line[idx+12:]
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
