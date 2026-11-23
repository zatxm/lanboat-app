package desktop

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/ruledata"
	"go.uber.org/zap"
)

const MaxBackups = 3

type BackupInfo struct {
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
}

func backupDir(path string) string { return filepath.Join(filepath.Dir(path), "backups") }
func (a *App) backupPath(id string) (string, error) {
	if filepath.Base(id) != id || strings.ContainsAny(id, "/\\") || !strings.HasPrefix(id, "backup-settings-") || !(strings.HasSuffix(id, ".json") || strings.HasSuffix(id, ".tar.gz")) {
		return "", errors.New("备份不存在")
	}
	return filepath.Join(backupDir(a.path), id), nil
}
func (a *App) Backups() []BackupInfo {
	entries, _ := os.ReadDir(backupDir(a.path))
	var list []BackupInfo
	for _, e := range entries {
		if _, err := a.backupPath(e.Name()); err != nil || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err == nil {
			list = append(list, BackupInfo{e.Name(), info.ModTime().Format(time.RFC3339Nano)})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID > list[j].ID })
	return list
}
func (a *App) CreateBackup() (_ BackupInfo, err error) {
	started := time.Now()
	a.log.Info("backup started")
	defer func() {
		duration := zap.Duration("duration", time.Since(started))
		if err != nil {
			a.log.Warn("backup failed", duration, zap.Error(err))
		} else {
			a.log.Info("backup completed", duration)
		}
	}()
	a.ruleUpdateMu.Lock()
	defer a.ruleUpdateMu.Unlock()
	if len(a.Backups()) >= MaxBackups {
		return BackupInfo{}, errors.New("备份已满，请删除一份后再创建")
	}
	dir := backupDir(a.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return BackupInfo{}, err
	}
	now := time.Now()
	name := "backup-settings-" + now.UTC().Format("20060102-150405.000000000")
	a.mu.Lock()
	c := a.config
	a.mu.Unlock()
	root := a.ruleDataDir()
	if _, err := os.Stat(filepath.Join(root, "current")); os.IsNotExist(err) {
		name += ".json"
		if err := profile.Save(filepath.Join(dir, name), c); err != nil {
			return BackupInfo{}, err
		}
	} else {
		name += ".tar.gz"
		if err := ruledata.CompactInactive(root); err != nil {
			return BackupInfo{}, err
		}
		if err := writeRoutingBackup(filepath.Join(dir, name), c, root); err != nil {
			return BackupInfo{}, err
		}
	}
	return BackupInfo{name, now.Format(time.RFC3339Nano)}, nil
}
func (a *App) RestoreBackup(id string) error {
	path, err := a.backupPath(id)
	if err != nil {
		return err
	}
	return a.ImportBackup(path)
}
func (a *App) importArchive(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > maxRoutingBackup {
		return errors.New("备份文件过大")
	}
	a.ruleUpdateMu.Lock()
	defer a.ruleUpdateMu.Unlock()
	root := a.ruleDataDir()
	c, stage, active, err := stageRoutingBackup(path, root)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err = ruledata.Ensure(root); err != nil {
		return err
	}
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	profile.EnsureFiles(&c)
	previous, err := ruledata.Current(root)
	if err != nil {
		return err
	}
	newDir, created, err := promoteRoutingBackup(root, stage, active)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = ruledata.Activate(root, previous)
			if current, e := ruledata.Current(root); e == nil && current == previous {
				for _, p := range created {
					_ = os.RemoveAll(p)
				}
			}
		}
	}()
	if err = ruledata.Activate(root, newDir); err != nil {
		return err
	}
	if err = a.commitWithRuleData(c, true, func() error { return ruledata.Activate(root, previous) }); err != nil {
		return err
	}
	committed = true
	if err = ruledata.CompactInactive(root); err != nil {
		return fmt.Errorf("规则已还原，但压缩旧数据失败：%w", err)
	}
	return nil
}
func (a *App) DeleteBackup(id string) error {
	path, err := a.backupPath(id)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		return errors.New("备份不存在")
	}
	a.log.Info("backup deleted")
	return nil
}

func (a *App) ExportBackupTo(id, directory string) (string, error) {
	path, err := a.backupPath(id)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("请选择有效的本地目录")
	}
	src, err := os.Open(path)
	if err != nil {
		return "", errors.New("备份不存在")
	}
	defer src.Close()
	ext := filepath.Ext(id)
	if strings.HasSuffix(id, ".tar.gz") {
		ext = ".tar.gz"
	}
	for i := 1; ; i++ {
		name := id
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(id, ext), i, ext)
		}
		target := filepath.Join(directory, name)
		out, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		_, e = io.Copy(out, src)
		if e == nil {
			e = out.Sync()
		}
		if closeErr := out.Close(); e == nil {
			e = closeErr
		}
		if e != nil {
			_ = os.Remove(target)
			return "", e
		}
		return target, nil
	}
}
