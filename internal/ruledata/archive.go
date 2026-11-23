package ruledata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// VerifyDir checks the original bundle manifest before preserving or restoring a generation.
func VerifyDir(dir string) error {
	manifest, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return err
	}
	expected := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		p := strings.SplitN(line, "  ", 2)
		if len(p) != 2 || len(p[0]) != 64 || p[1] == "" || filepath.Clean(p[1]) != p[1] || filepath.IsAbs(p[1]) || strings.HasPrefix(p[1], "../") || strings.Contains(p[1], "\\") {
			return errors.New("规则库校验清单无效")
		}
		if _, ok := expected[p[1]]; ok {
			return errors.New("规则库校验清单重复")
		}
		expected[p[1]] = p[0]
	}
	found := 0
	var total int64
	buf := make([]byte, 32<<10)
	h := sha256.New()
	err = filepath.WalkDir(dir, func(name string, e fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e.IsDir() {
			return nil
		}
		if !e.Type().IsRegular() {
			return errors.New("规则库包含非普通文件")
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "SHA256SUMS" {
			return nil
		}
		expectedSum, ok := expected[rel]
		if !ok {
			return fmt.Errorf("规则库包含未校验文件：%s", rel)
		}
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		h.Reset()
		n, err := io.CopyBuffer(h, io.LimitReader(f, (256<<20)+1), buf)
		total += n
		if err != nil {
			return err
		}
		if total > 256<<20 || hex.EncodeToString(h.Sum(nil)) != expectedSum {
			return fmt.Errorf("规则库校验失败：%s", rel)
		}
		found++
		return nil
	})
	if err != nil {
		return err
	}
	if found != len(expected) {
		return errors.New("规则库文件不完整")
	}
	entries, err := Index(dir)
	if err != nil || len(entries) == 0 {
		return errors.New("规则库索引无效")
	}
	for _, entry := range entries {
		if entry.Path != "srs/"+entry.Kind+"/"+entry.Category+".srs" || expected[entry.Path] == "" {
			return errors.New("规则库索引无效")
		}
	}
	return nil
}

// WriteGeneration writes a generation in the same validated format as the published bundle.
func WriteGeneration(dir string, out io.Writer) error {
	if err := VerifyDir(dir); err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	buf := make([]byte, 32<<10)
	err := filepath.WalkDir(dir, func(name string, e fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e.IsDir() {
			return nil
		}
		if !e.Type().IsRegular() {
			return errors.New("规则库包含非普通文件")
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0600, Size: info.Size(), Typeflag: tar.TypeReg}
		if err = tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		_, err = io.CopyBuffer(tw, io.LimitReader(f, info.Size()), buf)
		ce := f.Close()
		if err != nil {
			return err
		}
		return ce
	})
	if e := tw.Close(); err == nil {
		err = e
	}
	if e := gz.Close(); err == nil {
		err = e
	}
	return err
}

// CompactInactive archives every non-current generation after verifying its contents.
func CompactInactive(root string) error {
	current, err := Current(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range dirs {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "generation-") {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if dir == current {
			continue
		}
		archive := dir + ".tar.gz"
		if _, err = os.Stat(archive); os.IsNotExist(err) {
			temp, err := os.CreateTemp(root, ".generation-*.tar.gz")
			if err != nil {
				return err
			}
			name := temp.Name()
			err = WriteGeneration(dir, temp)
			if ce := temp.Close(); err == nil {
				err = ce
			}
			if err != nil {
				os.Remove(name)
				return err
			}
			if err = os.Rename(name, archive); err != nil {
				os.Remove(name)
				return err
			}
		} else if err != nil {
			return err
		} else if err = VerifyDir(dir); err != nil {
			return err
		}
		f, err := os.Open(archive)
		if err != nil {
			return err
		}
		got, err := scanGeneration(f, "")
		f.Close()
		if err != nil {
			return err
		}
		want, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
		if err != nil {
			return err
		}
		if !bytes.Equal(want, got) {
			return errors.New("规则库压缩包与原数据不一致")
		}
		if err = os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}
