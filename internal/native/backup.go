package native

import (
	"fmt"
	"github.com/zatxm/lanboat-app/internal/desktop"
	"time"
)

var backupKinds = []option{{"nodes", "节点"}, {"routing", "路由规则"}, {"dns", "DNS设置"}}

func backupTitle(kind string) string {
	for _, k := range backupKinds {
		if k.value == kind {
			return k.label
		}
	}
	return "备份"
}
func (u *UI) openBackups() { u.field("backupKind").SetText("nodes"); u.openDetail("backups") }
func (v *nativeView) backupsPage() {
	u := v.ui
	kind := u.field("backupKind").Text()
	if kind == "" {
		kind = "nodes"
		u.field("backupKind").SetText(kind)
	}
	tabs := []*widget{}
	for _, item := range backupKinds {
		k := item
		class := "backup-tab icon-" + map[string]string{"nodes": "layers", "routing": "route", "dns": "globe"}[k.value]
		if k.value == kind {
			class += " active"
		}
		tabs = append(tabs, v.button(k.label, class, false, false, func() { u.field("backupKind").SetText(k.value); u.render() }))
	}
	tabs = append(tabs, v.spacer())
	v.pack(v.body, v.box("", true, 10, false, tabs...))
	description := map[string]string{"nodes": "保存节点及订阅，含连接凭据。", "routing": "只保存当前启用的路由规则及自定义规则。", "dns": "保存当前DNS设置，不包含节点及路由规则。"}[kind]
	count := 0
	for _, b := range u.state.Backups {
		if b.Kind == kind {
			count++
		}
	}
	v.pack(v.body, v.label(description, "help"), v.box("", true, 8, false, v.label("备份记录", "section-title"), v.spacer(), v.label(fmt.Sprintf("%d / %d", count, desktop.MaxBackups), "help"), v.button("立即备份", "primary icon-add", false, false, u.createBackup)))
	list := v.box("panel group", false, 0, false)
	added := 0
	for _, b := range u.state.Backups {
		if b.Kind != kind {
			continue
		}
		if added > 0 {
			v.pack(list, v.create("separator", props{}))
		}
		v.pack(list, v.backupRow(b))
		added++
	}
	if count == 0 {
		v.pack(list, v.box("empty", false, 12, false, v.label("暂无备份", "node-name"), v.label("创建此分类的第一份备份。", "help")))
	}
	v.pack(v.body, list, v.label("每类最多保留5份；还原仅替换当前分类的数据。", "help"))
}
func (v *nativeView) backupRow(b desktop.BackupInfo) *widget {
	u := v.ui
	created := b.CreatedAt
	if t, err := time.Parse(time.RFC3339Nano, b.CreatedAt); err == nil {
		created = t.Local().Format("2006-01-02 15:04:05")
	}
	detail := backupTitle(b.Kind)
	if b.Kind == "nodes" {
		detail = fmt.Sprintf("%d 个节点", b.Nodes)
	}
	if b.Kind == "routing" {
		detail = b.Routing
	}
	restore := v.button("还原备份", "link icon-only icon-refresh", false, false, func() {
		u.confirm = &deletion{kind: "backup-restore", prompt: fmt.Sprintf("还原此备份将替换当前%s，是否继续？", backupTitle(b.Kind)), run: func() { u.restoreBackup(b.ID) }}
		u.notify()
	})
	download := v.button("下载备份", "link icon-only icon-download", false, false, func() { u.downloadBackup(b.ID) })
	remove := v.button("删除备份", "link icon-only icon-delete", false, false, func() {
		u.confirm = &deletion{kind: "backup", prompt: fmt.Sprintf("删除备份「%s」？", created), run: func() { u.deleteBackup(b.ID) }}
		u.notify()
	})
	detailLabel := v.label(detail, "help")
	if b.Kind == "routing" {
		detailLabel.raw = true
		detailLabel.localize()
	}
	return v.box("group-row", true, 8, false, v.icon(map[string]string{"nodes": "layers", "routing": "route", "dns": "globe"}[b.Kind]), v.box("", false, 5, true, v.dataLabel(created, "node-name node-title"), detailLabel), restore, download, remove)
}
func (u *UI) createBackup() {
	if u.busy {
		return
	}
	kind := u.field("backupKind").Text()
	if kind == "" {
		kind = "nodes"
		u.field("backupKind").SetText(kind)
	}
	count := 0
	for _, b := range u.state.Backups {
		if b.Kind == kind {
			count++
		}
	}
	if count >= desktop.MaxBackups {
		u.confirm = &deletion{kind: "backup-overwrite", prompt: fmt.Sprintf("此类备份已达%d份，继续将覆盖最早的备份，是否继续？", desktop.MaxBackups), run: func() { u.backupNow(kind) }}
		u.notify()
		return
	}
	u.backupNow(kind)
}
func (u *UI) backupNow(kind string) {
	u.submit(func() error { _, err := u.backend.CreateBackup(kind); return err }, func() { u.notice = ""; u.render() })
}
func (u *UI) restoreBackup(id string) {
	u.submit(func() error { return u.backend.RestoreBackup(id) }, func() { u.notice = "备份已还原"; u.render() })
}
func (u *UI) deleteBackup(id string) {
	u.submit(func() error { return u.backend.DeleteBackup(id) }, func() { u.notice = ""; u.render() })
}
func (u *UI) downloadBackup(id string) {
	directory := chooseDirectory("选择保存目录")
	if directory == "" {
		return
	}
	var saved string
	u.submit(func() error { var err error; saved, err = u.backend.ExportBackupTo(id, directory); return err }, func() { u.notice = fmt.Sprintf("已导出到：%s", saved) })
}
