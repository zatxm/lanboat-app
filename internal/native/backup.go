package native

import (
	"fmt"
	"slices"
	"time"

	"github.com/zatxm/lanboat-app/internal/desktop"
)

const backupLimitHint = "备份已满，请删除一份后再创建"

func (u *UI) openBackups() { u.openDetail("backups") }
func (v *nativeView) backupsPage() {
	u := v.ui
	imp := v.button("导入备份", "link icon-only icon-upload backup-action", false, false, func() {
		path := chooseFile("导入备份", "*")
		if path == "" {
			return
		}
		u.confirm = &deletion{kind: "backup-restore", prompt: "导入将替换全部设置，是否继续？", run: func() {
			u.submit(func() error { return u.backend.ImportBackup(path) }, func() { u.render() })
		}}
		u.notify()
	})
	v.backupCreate = v.button("立即备份", "link icon-only icon-add backup-action", false, false, u.createBackup)
	v.backupList = v.box("panel group", false, 0, false)
	v.pack(v.body, v.box("", true, 8, false, v.spacer(), imp, v.backupCreate), v.backupList)
	v.renderBackupRows()
}

func (v *nativeView) renderBackupRows() {
	v.backupRows = v.ui.state.Backups
	v.clear(v.backupList)
	tip := "立即备份"
	if len(v.backupRows) >= desktop.MaxBackups {
		tip = backupLimitHint
	}
	v.backupCreate.setTooltip(tip)
	if len(v.backupRows) == 0 {
		v.pack(v.backupList, v.box("empty", false, 0, false, v.label("暂无备份", "help")))
	}
	for i, b := range v.backupRows {
		if i > 0 {
			v.pack(v.backupList, v.create("separator", props{}))
		}
		v.pack(v.backupList, v.backupRow(b))
	}
}
func (v *nativeView) backupRow(b desktop.BackupInfo) *widget {
	u := v.ui
	created := b.CreatedAt
	if t, err := time.Parse(time.RFC3339Nano, b.CreatedAt); err == nil {
		created = t.Local().Format("2006-01-02 15:04:05")
	}
	restore := v.button("还原备份", "link icon-only icon-refresh backup-action", false, false, func() {
		u.confirm = &deletion{kind: "backup-restore", prompt: "导入将替换全部设置，是否继续？", run: func() { u.restoreBackup(b.ID) }}
		u.notify()
	})
	download := v.button("下载备份", "link icon-only icon-download backup-action", false, false, func() { u.downloadBackup(b.ID) })
	remove := v.button("删除备份", "link icon-only icon-delete backup-action", false, false, func() {
		u.confirm = &deletion{kind: "backup", prompt: fmt.Sprintf("删除备份「%s」？", created), run: func() { u.deleteBackup(b.ID) }}
		u.notify()
	})
	return v.box("group-row", true, 8, false, v.icon("layers"), v.box("", false, 0, true, v.dataLabel(created, "node-name")), restore, download, remove)
}
func (u *UI) createBackup() {
	if u.backingUp || len(u.state.Backups) >= desktop.MaxBackups {
		return
	}
	u.backingUp = true
	u.notify()
	go func() {
		_, err := u.backend.CreateBackup()
		list := u.backend.Backups()
		u.post(update{background: true, apply: func() {
			u.backingUp = false
			u.state.Backups = list
			if err != nil {
				u.err = err.Error()
			}
		}})
	}()
}
func (u *UI) restoreBackup(id string) {
	u.submit(func() error { return u.backend.RestoreBackup(id) }, func() { u.notice = "备份已还原"; u.render() })
}
func (u *UI) deleteBackup(id string) {
	u.submit(func() error { return u.backend.DeleteBackup(id) }, func() {
		u.state.Backups = slices.DeleteFunc(slices.Clone(u.state.Backups), func(b desktop.BackupInfo) bool { return b.ID == id })
	})
}
func (u *UI) downloadBackup(id string) {
	directory := chooseDirectory("选择保存目录")
	if directory == "" {
		return
	}
	var saved string
	u.submit(func() error { var err error; saved, err = u.backend.ExportBackupTo(id, directory); return err }, func() { u.notice = fmt.Sprintf("已导出到：\n%s", saved) })
}
