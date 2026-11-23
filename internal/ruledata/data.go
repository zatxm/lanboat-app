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
	"errors"
	"fmt"
	"github.com/goccy/go-json"
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
	if _, err = scanGeneration(input, dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// VerifyArchive validates an archived generation without extracting it to disk.
func VerifyArchive(input io.Reader) error {
	_, err := scanGeneration(input, "")
	return err
}

func scanGeneration(input io.Reader, dir string) ([]byte, error) {
	gz, err := gzip.NewReader(input)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hashes := map[string]string{}
	var manifest, index bytes.Buffer
	buf := make([]byte, 32<<10)
	sum := sha256.New()
	var total int64
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(strings.TrimPrefix(h.Name, "./"), "/")
		if name == "" || name == "." {
			continue
		}
		if path.Clean(name) != name || !filepath.IsLocal(name) || strings.Contains(name, "\\") {
			return nil, errors.New("规则包路径无效")
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return nil, errors.New("规则包包含非普通文件")
		}
		if h.Size < 0 || h.Size > (256<<20)-total || len(hashes) > 5000 {
			return nil, errors.New("规则包过大")
		}
		total += h.Size
		if _, exists := hashes[name]; exists {
			return nil, errors.New("规则包文件重复")
		}
		sum.Reset()
		var out io.Writer = sum
		switch name {
		case "SHA256SUMS", "rules-index.json":
			if h.Size > 8<<20 {
				return nil, errors.New("规则包索引或校验清单过大")
			}
			metadata := &index
			if name == "SHA256SUMS" {
				metadata = &manifest
			}
			out = io.MultiWriter(out, metadata)
		}
		if dir == "" {
			_, err = io.CopyBuffer(out, tr, buf)
		} else {
			dest := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
				return nil, err
			}
			f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return nil, err
			}
			_, copyErr := io.CopyBuffer(io.MultiWriter(f, out), tr, buf)
			if err := errors.Join(copyErr, f.Close()); err != nil {
				return nil, err
			}
		}
		if err != nil {
			return nil, err
		}
		hashes[name] = hex.EncodeToString(sum.Sum(nil))
	}
	delete(hashes, "SHA256SUMS")
	for name := range hashes {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if parent == "SHA256SUMS" || hashes[parent] != "" {
				return nil, errors.New("规则包路径无效")
			}
		}
	}
	var entries []Item
	if err := json.Unmarshal(index.Bytes(), &entries); err != nil || len(entries) == 0 {
		return nil, errors.New("规则包索引无效")
	}
	for _, entry := range entries {
		if entry.Path != "srs/"+entry.Kind+"/"+entry.Category+".srs" || hashes[entry.Path] == "" {
			return nil, errors.New("规则包索引无效")
		}
	}
	for line := range strings.SplitSeq(strings.TrimSpace(manifest.String()), "\n") {
		hash, name, ok := strings.Cut(line, "  ")
		if !ok || len(hash) != 64 {
			return nil, errors.New("规则包校验清单无效")
		}
		if hashes[name] != hash {
			return nil, fmt.Errorf("规则包校验失败：%s", name)
		}
		delete(hashes, name)
	}
	if len(hashes) != 0 {
		return nil, errors.New("规则包文件数量不匹配")
	}
	if _, err := io.CopyBuffer(io.Discard, gz, buf); err != nil {
		return nil, err
	}
	return manifest.Bytes(), nil
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
