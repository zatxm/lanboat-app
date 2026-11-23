package desktop

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zatxm/lanboat-app/internal/profile"
	"github.com/zatxm/lanboat-app/internal/ruledata"
)

const maxRoutingBackup = int64(2 << 30)

var generationName = regexp.MustCompile(`^generation-[0-9]+$`)

func openRoutingBackup(name string) (*os.File, *gzip.Reader, *tar.Reader, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, nil, err
	}
	return f, gz, tar.NewReader(gz), nil
}
func archiveMetadata(tr *tar.Reader) (profile.Config, error) {
	var b profile.Config
	h, err := tr.Next()
	if err != nil || h.Name != "backup.json" || h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > profile.MaxConfig {
		return b, errors.New("备份文件已损坏")
	}
	data, err := io.ReadAll(io.LimitReader(tr, profile.MaxConfig+1))
	if err != nil {
		return b, err
	}
	if err = json.Unmarshal(data, &b); err != nil {
		return b, errors.New("备份文件已损坏")
	}
	return b, nil
}
func writeRoutingBackup(destination string, b profile.Config, root string) error {
	current, err := ruledata.Current(root)
	if err != nil {
		return err
	}
	if err = ruledata.VerifyDir(current); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "generation-") && filepath.Join(root, e.Name()) != current {
			return errors.New("非当前规则库尚未压缩")
		}
		if e.Type().IsRegular() && strings.HasPrefix(e.Name(), "generation-") && strings.HasSuffix(e.Name(), ".tar.gz") {
			f, err := os.Open(filepath.Join(root, e.Name()))
			if err != nil {
				return err
			}
			err = ruledata.VerifyArchive(f)
			f.Close()
			if err != nil {
				return fmt.Errorf("规则库压缩包无效：%w", err)
			}
		}
	}
	metadata, err := json.Marshal(b)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".backup-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	gz := gzip.NewWriter(temp)
	tw := tar.NewWriter(gz)
	buf := make([]byte, 32<<10)
	addBytes := func(name string, data []byte) error {
		if e := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), Typeflag: tar.TypeReg}); e != nil {
			return e
		}
		_, e := tw.Write(data)
		return e
	}
	err = addBytes("backup.json", metadata)
	if err == nil {
		err = addBytes("rules-data/current", []byte(filepath.Base(current)))
	}
	addFile := func(full, relative string) error {
		info, e := os.Lstat(full)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return errors.New("规则库含非普通文件")
		}
		if e = tw.WriteHeader(&tar.Header{Name: relative, Mode: 0600, Size: info.Size(), Typeflag: tar.TypeReg}); e != nil {
			return e
		}
		f, e := os.Open(full)
		if e != nil {
			return e
		}
		_, e = io.CopyBuffer(tw, io.LimitReader(f, info.Size()), buf)
		ce := f.Close()
		if e != nil {
			return e
		}
		return ce
	}
	if err == nil {
		err = filepath.WalkDir(current, func(full string, e fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if e.IsDir() {
				return nil
			}
			rel, er := filepath.Rel(root, full)
			if er != nil {
				return er
			}
			return addFile(full, "rules-data/"+filepath.ToSlash(rel))
		})
	}
	if err == nil {
		for _, e := range entries {
			if e.Type().IsRegular() && strings.HasPrefix(e.Name(), "generation-") && strings.HasSuffix(e.Name(), ".tar.gz") {
				if err = addFile(filepath.Join(root, e.Name()), "rules-data/"+e.Name()); err != nil {
					break
				}
			}
		}
	}
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	if e := temp.Sync(); err == nil {
		err = e
	}
	if e := temp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(temp.Name(), destination)
}

// stageRoutingBackup extracts and validates before touching the active data or configuration.
func stageRoutingBackup(file, root string) (profile.Config, string, string, error) {
	var b profile.Config
	if err := os.MkdirAll(root, 0700); err != nil {
		return b, "", "", err
	}
	stage, err := os.MkdirTemp(root, ".restore-")
	if err != nil {
		return b, "", "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(stage)
		}
	}()
	f, gz, tr, err := openRoutingBackup(file)
	if err != nil {
		return b, "", "", err
	}
	defer f.Close()
	defer gz.Close()
	b, err = archiveMetadata(tr)
	if err != nil {
		return b, "", "", err
	}
	if err = b.ValidateSnapshot(); err != nil {
		return b, "", "", errors.New("路由规则备份内容无效")
	}
	total := int64(0)
	count := 0
	buf := make([]byte, 32<<10)
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return b, "", "", e
		}
		name := h.Name
		if h.Typeflag != tar.TypeReg || !strings.HasPrefix(name, "rules-data/") || path.Clean(name) != name || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || h.Size < 0 {
			return b, "", "", errors.New("备份路径无效")
		}
		rel := strings.TrimPrefix(name, "rules-data/")
		parts := strings.Split(rel, "/")
		if len(parts) == 0 || !(rel == "current" || generationName.MatchString(parts[0]) && len(parts) > 1 || len(parts) == 1 && strings.HasSuffix(parts[0], ".tar.gz") && generationName.MatchString(strings.TrimSuffix(parts[0], ".tar.gz"))) {
			return b, "", "", errors.New("备份内容无效")
		}
		count++
		if h.Size > maxRoutingBackup-total || count > 100000 {
			return b, "", "", errors.New("备份内容过大")
		}
		total += h.Size
		dest := filepath.Join(stage, filepath.FromSlash(rel))
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return b, "", "", err
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return b, "", "", e
		}
		_, err = io.CopyBuffer(out, tr, buf)
		ce := out.Close()
		if err == nil {
			err = ce
		}
		if err != nil {
			return b, "", "", err
		}
	}
	if _, err = io.Copy(io.Discard, gz); err != nil {
		return b, "", "", err
	}
	name, err := os.ReadFile(filepath.Join(stage, "current"))
	if err != nil {
		return b, "", "", err
	}
	active := strings.TrimSpace(string(name))
	if !generationName.MatchString(active) {
		return b, "", "", errors.New("备份当前规则库无效")
	}
	if err = ruledata.VerifyDir(filepath.Join(stage, active)); err != nil {
		return b, "", "", fmt.Errorf("备份规则库无效：%w", err)
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return b, "", "", err
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != active {
			return b, "", "", errors.New("备份包含多个未压缩规则库")
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tar.gz") {
			f, err := os.Open(filepath.Join(stage, entry.Name()))
			if err != nil {
				return b, "", "", err
			}
			err = ruledata.VerifyArchive(f)
			f.Close()
			if err != nil {
				return b, "", "", err
			}
		}
	}
	ok = true
	return b, stage, active, nil
}

// Promote staged generations under new names so restoration never overwrites local data.
func promoteRoutingBackup(root, stage, active string) (string, []string, error) {
	var created []string
	cleanup := func() {
		for _, p := range created {
			_ = os.RemoveAll(p)
		}
	}
	newDir, err := os.MkdirTemp(root, "generation-")
	if err != nil {
		return "", nil, err
	}
	if err = os.Remove(newDir); err != nil {
		return "", nil, err
	}
	if err = os.Rename(filepath.Join(stage, active), newDir); err != nil {
		return "", nil, err
	}
	created = append(created, newDir)
	entries, err := os.ReadDir(stage)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		f, e := os.CreateTemp(root, "generation-*.tar.gz")
		if e != nil {
			cleanup()
			return "", nil, e
		}
		target := f.Name()
		if e = f.Close(); e == nil {
			e = os.Rename(filepath.Join(stage, entry.Name()), target)
		}
		if e != nil {
			_ = os.Remove(target)
			cleanup()
			return "", nil, e
		}
		created = append(created, target)
	}
	return newDir, created, nil
}
