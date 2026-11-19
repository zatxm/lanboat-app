package native

import (
	"fmt"
	"time"

	"github.com/zatxm/lanboat-app/internal/desktop"
)

const backupLimitHint = "备份已满，请删除一份后再创建"

func (u *UI) openBackups() { u.openDetail("backups") }
func (v *nativeView) backupsPage() {
	u := v.ui
	imp := v.button("导入备份", "link icon-only icon-upload", false, false, func() {
		path := chooseFile("导入备份", "*")
		if path == "" {
			return
		}
		u.confirm = &deletion{kind: "backup-restore", prompt: "导入将替换全部设置，是否继续？", run: func() {
			u.submit(func() error { return u.backend.ImportBackup(path) }, func() { u.render() })
		}}
		u.notify()
	})
	full := len(u.state.Backups) >= desktop.MaxBackups
	create := v.button("立即备份", "link icon-only icon-add", false, full, u.createBackup)
	if full {
		create.setTooltip(backupLimitHint)
	}
	v.pack(v.body, v.box("", true, 8, false, v.spacer(), imp, create))
	var rows []*widget
	for _, b := range u.state.Backups {
		rows = append(rows, v.backupRow(b))
	}
	if len(rows) == 0 {
		rows = append(rows, v.box("empty", false, 0, false, v.label("暂无备份", "help")))
	}
	v.group("", rows...)
}
func (v *nativeView) backupRow(b desktop.BackupInfo) *widget {
	u := v.ui
	created := b.CreatedAt
	if t, err := time.Parse(time.RFC3339Nano, b.CreatedAt); err == nil {
		created = t.Local().Format("2006-01-02 15:04:05")
	}
	restore := v.button("还原备份", "link icon-only icon-refresh", false, false, func() {
		u.confirm = &deletion{kind: "backup-restore", prompt: "导入将替换全部设置，是否继续？", run: func() { u.restoreBackup(b.ID) }}
		u.notify()
	})
	download := v.button("下载备份", "link icon-only icon-download", false, false, func() { u.downloadBackup(b.ID) })
	remove := v.button("删除备份", "link icon-only icon-delete", false, false, func() {
		u.confirm = &deletion{kind: "backup", prompt: fmt.Sprintf("删除备份「%s」？", created), run: func() { u.deleteBackup(b.ID) }}
		u.notify()
	})
	return v.box("group-row", true, 8, false, v.icon("layers"), v.box("", false, 0, true, v.dataLabel(created, "node-name")), restore, download, remove)
}
func (u *UI) createBackup() {
	if len(u.state.Backups) >= desktop.MaxBackups {
		return
	}
	u.submit(func() error { _, err := u.backend.CreateBackup(); return err }, func() { u.notice = ""; u.render() })
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
	u.submit(func() error { var err error; saved, err = u.backend.ExportBackupTo(id, directory); return err }, func() { u.notice = fmt.Sprintf("已导出到：\n%s", saved) })
}
