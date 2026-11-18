// Package ruledata manages the published Lanboat sing-box rule sets.
package ruledata

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

//go:embed assets/lanboat-allinone-rules.tar.gz
var initial []byte

var releaseURL = "https://github.com/zatxm/geoip/releases/latest/download/"

const bundleName = "lanboat-allinone-rules.tar.gz"

type Item struct {
	Kind     string `json:"kind"`
	Category string `json:"category"`
	Path     string `json:"path"`
}

func Current(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "current"))
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(b))
	if name == "" || filepath.Base(name) != name {
		return "", errors.New("规则库索引无效")
	}
	dir := filepath.Join(root, name)
	if _, err := os.Stat(filepath.Join(dir, "rules-index.json")); err != nil {
		return "", err
	}
	return dir, nil
}

func Ensure(root string) error {
	if _, err := Current(root); err == nil {
		return nil
	}
	_, err := install(root, bytes.NewReader(initial))
	return err
}

// Unpack verifies all entries and leaves the active version unchanged.
func Unpack(root string, input io.Reader) (string, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, "generation-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(dir)
		}
	}()
	gz, err := gzip.NewReader(input)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hashes := map[string]string{}
	var manifest []byte
	total := int64(0)
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		name := strings.TrimSuffix(strings.TrimPrefix(h.Name, "./"), "/")
		clean := path.Clean(name)
		if name == "" || name == "." {
			continue
		}
		if clean != name || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, "\\") {
			return "", errors.New("规则包路径无效")
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return "", errors.New("规则包包含非普通文件")
		}
		total += h.Size
		if h.Size < 0 || total > 256<<20 || len(hashes) > 5000 {
			return "", errors.New("规则包过大")
		}
		dest := filepath.Join(dir, filepath.FromSlash(clean))
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return "", err
		}
		f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return "", e
		}
		sum := sha256.New()
		_, e = io.Copy(io.MultiWriter(f, sum), tr)
		ce := f.Close()
		if e != nil {
			return "", e
		}
		if ce != nil {
			return "", ce
		}
		if clean == "SHA256SUMS" {
			manifest, e = os.ReadFile(dest)
			if e != nil {
				return "", e
			}
		} else {
			hashes[clean] = hex.EncodeToString(sum.Sum(nil))
		}
	}
	expected := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(manifest)), "\n") {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 {
			return "", errors.New("规则包校验清单无效")
		}
		if _, exists := expected[parts[1]]; exists {
			return "", errors.New("规则包校验清单重复")
		}
		expected[parts[1]] = parts[0]
	}
	if len(expected) != len(hashes) {
		return "", errors.New("规则包文件数量不匹配")
	}
	for name, sum := range hashes {
		if expected[name] != sum {
			return "", fmt.Errorf("规则包校验失败：%s", name)
		}
	}
	entries, err := Index(dir)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", errors.New("规则包索引为空")
	}
	for _, entry := range entries {
		if entry.Path != "srs/"+entry.Kind+"/"+entry.Category+".srs" || hashes[entry.Path] == "" {
			return "", errors.New("规则包索引无效")
		}
	}
	if _, err = io.Copy(io.Discard, gz); err != nil {
		return "", err
	}
	ok = true
	return dir, nil
}

func install(root string, input io.Reader) (string, error) {
	dir, err := Unpack(root, input)
	if err != nil {
		return "", err
	}
	if err = Activate(root, dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func Activate(root, dir string) error {
	if filepath.Dir(dir) != root {
		return errors.New("规则库路径无效")
	}
	f, err := os.CreateTemp(root, ".current-")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	_, err = f.WriteString(filepath.Base(dir))
	if ce := f.Close(); err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	return os.Rename(temp, filepath.Join(root, "current"))
}

func Index(dir string) ([]Item, error) {
	b, err := os.ReadFile(filepath.Join(dir, "rules-index.json"))
	if err != nil {
		return nil, err
	}
	var v []Item
	err = json.Unmarshal(b, &v)
	return v, err
}

func Fetch(ctx context.Context, root string) error {
	client := &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || (len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https") {
			return errors.New("规则库重定向无效")
		}
		return nil
	}}
	get := func(name string, limit int64) ([]byte, error) {
		req, e := http.NewRequestWithContext(ctx, "GET", releaseURL+name, nil)
		if e != nil {
			return nil, e
		}
		res, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", res.StatusCode)
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, limit+1))
		if e == nil && int64(len(b)) > limit {
			e = errors.New("下载文件过大")
		}
		return b, e
	}
	sums, err := get("SHA256SUMS", 1<<20)
	if err != nil {
		return err
	}
	var expected string
	for _, line := range strings.Split(string(sums), "\n") {
		p := strings.Fields(line)
		if len(p) == 2 && p[1] == bundleName {
			expected = p[0]
			break
		}
	}
	if len(expected) != 64 {
		return errors.New("发布校验值缺失")
	}
	data, err := get(bundleName, 32<<20)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expected {
		return errors.New("规则包SHA-256校验失败")
	}
	_, err = install(root, bytes.NewReader(data))
	return err
}
