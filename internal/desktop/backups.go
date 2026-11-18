package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/routing"
	"github.com/zatxm/lanboat-app/internal/ruledata"
)

// MaxBackups bounds local snapshots; creating another one replaces the oldest.
const MaxBackups = 5

type BackupRouting struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Each snapshot restores one category without changing the other settings.
type Backup struct {
	Version       int                    `json:"version"`
	Kind          string                 `json:"kind"`
	Subscriptions []profile.Subscription `json:"subscriptions,omitempty"`
	RuleOverlays  []profile.RuleOverlay  `json:"ruleOverlays,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	Nodes         []profile.Node         `json:"nodes,omitempty"`
	Selected      string                 `json:"selected,omitempty"`
	Rules         []profile.Rule         `json:"rules,omitempty"`
	Routing       *BackupRouting         `json:"routing,omitempty"`
	DNS           *profile.DNSSettings   `json:"dns,omitempty"`
}

type BackupInfo struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	CreatedAt string `json:"createdAt"`
	Nodes     int    `json:"nodes"`
	Routing   string `json:"routing,omitempty"`
}

func backupDir(path string) string { return filepath.Join(filepath.Dir(path), "backups") }

func backupID(id string) error {
	if !strings.HasPrefix(id, "backup-") || !(strings.HasSuffix(id, ".json") || strings.HasSuffix(id, ".tar.gz")) || len(id) > 128 || id != filepath.Base(id) || strings.ContainsAny(id, "/\\") {
		return errors.New("备份不存在")
	}
	return nil
}

func (a *App) backupPath(id string) (string, error) {
	if err := backupID(id); err != nil {
		return "", err
	}
	dir := backupDir(a.path)
	path := filepath.Join(dir, id)
	if filepath.Dir(path) != dir {
		return "", errors.New("备份不存在")
	}
	return path, nil
}

func (a *App) Backups() []BackupInfo {
	dir := backupDir(a.path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	infos := []BackupInfo{}
	for _, entry := range entries {
		if entry.IsDir() || backupID(entry.Name()) != nil {
			continue
		}
		b, err := readBackupFile(filepath.Join(dir, entry.Name()))
		if err != nil || b.validate() != nil {
			continue
		}
		infos = append(infos, BackupInfo{Kind: b.Kind, ID: entry.Name(), CreatedAt: b.CreatedAt.Format(time.RFC3339Nano), Nodes: len(b.Nodes), Routing: routingName(b)})
	}
	sort.Slice(infos, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, infos[i].CreatedAt)
		right, _ := time.Parse(time.RFC3339Nano, infos[j].CreatedAt)
		if left.Equal(right) {
			return infos[i].ID > infos[j].ID
		}
		return left.After(right)
	})
	counts := map[string]int{}
	filtered := infos[:0]
	for _, info := range infos {
		if counts[info.Kind] < MaxBackups {
			filtered = append(filtered, info)
			counts[info.Kind]++
		}
	}
	return filtered
}

func routingName(b Backup) string {
	if b.Routing == nil {
		return ""
	}
	return b.Routing.Name
}

func readBackupFile(path string) (Backup, error) {
	var b Backup
	info, err := os.Stat(path)
	if err != nil {
		return b, errors.New("备份不存在")
	}
	if strings.HasSuffix(path, ".tar.gz") {
		if info.Size() > maxRoutingBackup {
			return b, errors.New("备份文件过大")
		}
		f, gz, tr, e := openRoutingBackup(path)
		if e != nil {
			return b, e
		}
		defer f.Close()
		defer gz.Close()
		b, e = archiveMetadata(tr)
		if e != nil {
			return b, e
		}
		if b.Kind != "routing" || b.Version != 3 {
			return b, errors.New("备份格式不受支持")
		}
		return b, nil
	}
	if info.Size() > profile.MaxConfig {
		return b, errors.New("备份文件过大")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return b, errors.New("无法读取备份")
	}
	if err = json.Unmarshal(data, &b); err != nil {
		return b, errors.New("备份文件已损坏")
	}
	if b.Kind == "routing" {
		return b, errors.New("路由规则备份格式不受支持")
	}
	return b, nil
}

func (a *App) readBackup(id string) (Backup, error) {
	path, err := a.backupPath(id)
	if err != nil {
		return Backup{}, err
	}
	return readBackupFile(path)
}

func backupKind(kind string) bool { return kind == "nodes" || kind == "routing" || kind == "dns" }
func (a *App) CreateBackup(kind string) (BackupInfo, error) {
	if !backupKind(kind) {
		return BackupInfo{}, errors.New("备份类型无效")
	}
	if kind == "routing" {
		a.ruleUpdateMu.Lock()
		defer a.ruleUpdateMu.Unlock()
	}
	a.mu.Lock()
	c := a.config
	a.mu.Unlock()
	now := time.Now()
	b := Backup{Version: 2, Kind: kind, CreatedAt: now}
	switch kind {
	case "nodes":
		b.Nodes = append([]profile.Node{}, c.Nodes...)
		b.Subscriptions = append([]profile.Subscription{}, c.Subscriptions...)
		b.RuleOverlays = append([]profile.RuleOverlay{}, c.RuleOverlays...)
		b.Selected = c.Selected
	case "routing":
		b.Version = 3
		b.Rules = append([]profile.Rule{}, c.Rules...)
		for _, f := range c.Files {
			if f.ID == c.ActiveFile {
				b.Routing = &BackupRouting{Name: f.Name, Content: f.Content}
				break
			}
		}
		if b.Routing == nil {
			return BackupInfo{}, errors.New("当前启用的路由规则不存在")
		}
	case "dns":
		d := profile.EffectiveDNS(c, c.Settings.Mode == "tun")
		b.DNS = &d
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return BackupInfo{}, err
	}
	dir := backupDir(a.path)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return BackupInfo{}, err
	}
	ext := ".json"
	if kind == "routing" {
		ext = ".tar.gz"
	}
	name := "backup-" + kind + "-" + now.Format("20060102-150405") + ext
	for i := 2; ; i++ {
		if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
			break
		}
		name = fmt.Sprintf("backup-%s-%s-%d%s", kind, now.Format("20060102-150405"), i, ext)
	}
	if kind == "routing" {
		root := a.ruleDataDir()
		if err = ruledata.Ensure(root); err == nil {
			err = ruledata.CompactInactive(root)
		}
		if err == nil {
			err = writeRoutingBackup(filepath.Join(dir, name), b, root)
		}
	} else {
		err = writePrivateFile(filepath.Join(dir, name), data)
	}
	if err != nil {
		return BackupInfo{}, err
	}
	a.pruneBackups(kind)
	a.log.Info("backup created")
	info := BackupInfo{Kind: kind, ID: name, CreatedAt: now.Format(time.RFC3339Nano), Nodes: len(b.Nodes), Routing: routingName(b)}
	return info, nil
}

func (a *App) pruneBackups(kind string) {
	dir := backupDir(a.path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type candidate struct {
		name string
		at   time.Time
	}
	var files []candidate
	for _, entry := range entries {
		if entry.IsDir() || backupID(entry.Name()) != nil {
			continue
		}
		// Unreadable snapshots are left untouched and never displace usable ones.
		b, err := readBackupFile(filepath.Join(dir, entry.Name()))
		if err != nil || b.Kind != kind || b.validate() != nil {
			continue
		}
		files = append(files, candidate{entry.Name(), b.CreatedAt})
	}
	if len(files) <= MaxBackups {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].at.Equal(files[j].at) {
			return files[i].name > files[j].name
		}
		return files[i].at.After(files[j].at)
	})
	for _, old := range files[MaxBackups:] {
		_ = os.Remove(filepath.Join(dir, old.name))
	}
}

func (b Backup) validate() error {
	if !backupKind(b.Kind) || (b.Kind == "routing" && b.Version != 3) || (b.Kind != "routing" && b.Version != 2) {
		return errors.New("备份版本不受支持")
	}
	switch b.Kind {
	case "nodes":
		if b.Routing != nil || b.DNS != nil || len(b.Rules) > 0 {
			return errors.New("节点备份包含其他类型的数据")
		}
	case "routing":
		if b.Routing == nil || b.DNS != nil || len(b.Nodes) > 0 || len(b.Subscriptions) > 0 || len(b.RuleOverlays) > 0 {
			return errors.New("路由规则备份内容无效")
		}
		if _, err := routing.Parse(b.Routing.Content); err != nil {
			return err
		}
	case "dns":
		if b.DNS == nil || b.Routing != nil || len(b.Nodes) > 0 || len(b.Rules) > 0 || len(b.Subscriptions) > 0 || len(b.RuleOverlays) > 0 {
			return errors.New("DNS备份内容无效")
		}
		if err := b.DNS.Validate(); err != nil {
			return err
		}
	}
	if len(b.Subscriptions) > 30 {
		return errors.New("备份中的订阅数量超过限制")
	}
	subs := map[string]bool{}
	for _, sub := range b.Subscriptions {
		if sub.ID == "" || subs[sub.ID] {
			return errors.New("备份中的订阅标识无效或重复")
		}
		subs[sub.ID] = true
		if err := sub.Validate(); err != nil {
			return err
		}
	}
	if len(b.Nodes) > profile.MaxNodes {
		return errors.New("备份中的节点数量超过限制")
	}
	ids := map[string]bool{}
	for _, n := range b.Nodes {
		if n.ID == "" || ids[n.ID] {
			return errors.New("备份中的节点标识无效或重复")
		}
		ids[n.ID] = true
		if err := n.Validate(); err != nil {
			return err
		}
	}
	if err := profile.ValidateRuleOverlays(b.RuleOverlays); err != nil {
		return err
	}
	if len(b.Rules) > 500 {
		return errors.New("备份中的规则数量超过限制")
	}
	for _, r := range b.Rules {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// RestoreBackup applies only the category recorded in the snapshot.
func (a *App) RestoreBackup(id string) error {
	b, err := a.readBackup(id)
	if err != nil {
		return err
	}
	if b.Kind != "routing" {
		return a.restoreSnapshot(b, "", "")
	}
	a.ruleUpdateMu.Lock()
	defer a.ruleUpdateMu.Unlock()
	root := a.ruleDataDir()
	if err = ruledata.Ensure(root); err != nil {
		return err
	}
	path, err := a.backupPath(id)
	if err != nil {
		return err
	}
	b, stage, active, err := stageRoutingBackup(path, root)
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	return a.restoreSnapshot(b, stage, active)
}
func (a *App) restoreSnapshot(b Backup, stage, active string) error {
	var err error
	if err = b.validate(); err != nil {
		return err
	}
	a.connectionMu.Lock()
	defer a.connectionMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	switch b.Kind {
	case "nodes":
		c.Nodes = append([]profile.Node{}, b.Nodes...)
		c.Subscriptions = append([]profile.Subscription{}, b.Subscriptions...)
		c.RuleOverlays = append([]profile.RuleOverlay{}, b.RuleOverlays...)
		c.Selected = ""
		for _, n := range c.Nodes {
			if n.ID == b.Selected {
				c.Selected = n.ID
				break
			}
		}
		if c.Selected == "" && len(c.Nodes) > 0 {
			c.Selected = c.Nodes[0].ID
		}
	case "routing":
		c.Rules = append([]profile.Rule{}, b.Rules...)
		for i := range c.Rules {
			if c.Rules[i].ID == "" {
				c.Rules[i].ID = profile.ID()
			}
		}
		id, err := restoreRoutingFile(&c, *b.Routing)
		if err != nil {
			return err
		}
		c.ActiveFile = id
	case "dns":
		d := *b.DNS
		c.Settings.DNS = &d
	}
	if d := c.Settings.DNS; d != nil && d.ProxyRoute == "node" {
		found := false
		for _, n := range c.Nodes {
			if n.ID == d.ProxyNode {
				found = true
				break
			}
		}
		if !found {
			return errors.New("还原后DNS出站节点不存在，请先调整DNS出口或还原对应节点")
		}
	}
	if stage == "" {
		if err = a.commit(c); err != nil {
			return err
		}
	} else {
		root := a.ruleDataDir()
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
	}
	a.log.Info("backup restored")
	return nil
}

func restoreRoutingFile(c *profile.Config, r BackupRouting) (string, error) {
	if _, err := routing.Parse(r.Content); err != nil {
		return "", fmt.Errorf("备份中的路由规则无效：%w", err)
	}
	if r.Name == "default.conf" {
		if r.Content == routing.DefaultConfig {
			return "default", nil
		}
		r.Name = "default-restored.conf"
	}
	files := append([]profile.ConfigFile{}, c.Files...)
	for i, f := range files {
		if f.ID != "default" && f.Name == r.Name {
			files[i].Content = r.Content
			files[i].UpdatedAt = time.Now().Format(time.RFC3339)
			c.Files = files
			return f.ID, nil
		}
	}
	name := r.Name
	if !profile.ValidFileName(name) || name == "default.conf" {
		name = "restored.conf"
	}
	if !strings.HasSuffix(name, ".conf") {
		name += ".conf"
	}
	names := map[string]bool{}
	for _, f := range files {
		names[f.Name] = true
	}
	if names[name] {
		base := strings.TrimSuffix(name, ".conf")
		for i := 2; ; i++ {
			candidate := fmt.Sprintf("%s-%d.conf", base, i)
			if !names[candidate] {
				name = candidate
				break
			}
		}
	}
	if len(files) >= 30 {
		return "", errors.New("最多保存 30 个配置文件，无法还原路由规则")
	}
	id := profile.ID()
	c.Files = append(files, profile.ConfigFile{ID: id, Name: name, Content: r.Content, UpdatedAt: time.Now().Format(time.RFC3339), Source: "本地备份还原"})
	return id, nil
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
	for i := 1; ; i++ {
		name := id
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(id, filepath.Ext(id)), i, filepath.Ext(id))
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
