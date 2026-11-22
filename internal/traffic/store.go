package traffic

import (
	"bufio"
	"cmp"
	"fmt"
	"github.com/goccy/go-json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const PageSize = 40

// Entry stores network observations. Request headers and body require explicit opt-in.
type Entry struct {
	Method        string              `json:"method,omitempty"`
	Headers       map[string][]string `json:"headers,omitempty"`
	Body          []byte              `json:"body,omitempty"`
	BodyTruncated bool                `json:"bodyTruncated,omitempty"`
	Incomplete    bool                `json:"incomplete,omitempty"`
	Time          string              `json:"time"`
	Kind          string              `json:"kind"`
	URL           string              `json:"url,omitempty"`
	Rule          string              `json:"rule,omitempty"`
	Policy        string              `json:"policy,omitempty"`
	Network       string              `json:"network,omitempty"`
	Domain        string              `json:"domain,omitempty"`
	QueryType     string              `json:"queryType,omitempty"`
	Result        string              `json:"result,omitempty"`
	Resolver      string              `json:"resolver,omitempty"`
	DurationMS    int64               `json:"durationMS,omitempty"`
	// Upload and Download are the bytes a closed connection carried.
	Upload   int64 `json:"upload,omitempty"`
	Download int64 `json:"download,omitempty"`
	// Outbound is the group member that carried the connection when Policy is a group.
	Outbound string `json:"outbound,omitempty"`
	Process  string `json:"process,omitempty"`
	Error    string `json:"error,omitempty"`
}
type Page struct {
	Kind          string
	Number, Total int
	Revision      uint64
	Entries       []Entry
}
type fileIndex struct {
	size    int64
	count   int
	offsets []int64
}
type Store struct {
	mu         sync.Mutex
	dir        string
	indexes    map[string]*fileIndex
	proxy, dns atomic.Bool
	revision   [2]atomic.Uint64
	// files keeps today's append handle per kind (0 proxy, 1 dns) instead of
	// reopening the file for every record; checked marks the last time the
	// handle was confirmed to still be the file at its path.
	files   [2]*os.File
	checked [2]time.Time
}

func New(dir string) *Store { return &Store{dir: dir, indexes: map[string]*fileIndex{}} }

func kindIndex(kind string) int {
	if kind == "proxy" {
		return 0
	}
	return 1
}
func (s *Store) SetEnabled(proxy, dns bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proxy.Store(proxy)
	s.dns.Store(dns)
}
func (s *Store) Enabled(kind string) bool {
	return kind == "proxy" && s.proxy.Load() || kind == "dns" && s.dns.Load()
}
func (s *Store) Active() bool { return s.proxy.Load() || s.dns.Load() }
func (s *Store) Revision(kind string) uint64 {
	if kind == "proxy" {
		return s.revision[0].Load()
	}
	return s.revision[1].Load()
}
func (s *Store) Record(e Entry) error {
	if !s.Enabled(e.Kind) {
		return nil
	}
	return s.RecordBatch([]Entry{e})
}

// RecordBatch appends entries in order, writing each kind's lines with one
// system call. Entries of disabled kinds are skipped.
func (s *Store) RecordBatch(entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var lines [2][]byte
	var counts [2]uint64
	var first error
	for _, e := range entries {
		if !s.Enabled(e.Kind) {
			continue
		}
		if e.Time == "" {
			e.Time = time.Now().Format(time.RFC3339Nano)
		}
		data, err := json.Marshal(e)
		if err != nil {
			first = cmp.Or(first, err)
			continue
		}
		k := kindIndex(e.Kind)
		lines[k] = append(append(lines[k], data...), '\n')
		counts[k]++
	}
	day := time.Now().Format("2006-01-02")
	for k, kind := range [2]string{"proxy", "dns"} {
		if counts[k] == 0 {
			continue
		}
		f, err := s.file(k, filepath.Join(s.dir, kind+"-"+day+".jsonl"))
		if err == nil {
			_, err = f.Write(lines[k])
		}
		if err != nil {
			// Reopen on the next record instead of reusing a failed handle.
			s.closeFile(k)
			first = cmp.Or(first, err)
			continue
		}
		s.revision[k].Add(counts[k])
	}
	return first
}

// file returns an append handle for name, reopening it after a day change or
// when the path no longer refers to the open file (checked at most once a second).
func (s *Store) file(k int, name string) (*os.File, error) {
	if f := s.files[k]; f != nil && f.Name() == name {
		if time.Since(s.checked[k]) < time.Second {
			return f, nil
		}
		path, err := os.Stat(name)
		open, openErr := f.Stat()
		if err == nil && openErr == nil && os.SameFile(path, open) {
			s.checked[k] = time.Now()
			return f, nil
		}
	}
	s.closeFile(k)
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	s.files[k], s.checked[k] = f, time.Now()
	return f, nil
}

func (s *Store) closeFile(k int) {
	if s.files[k] != nil {
		_ = s.files[k].Close()
		s.files[k] = nil
	}
}
func (s *Store) index(name string) (*fileIndex, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	idx := s.indexes[name]
	if idx == nil || stat.Size() < idx.size {
		idx = &fileIndex{}
		s.indexes[name] = idx
	}
	if stat.Size() == idx.size {
		return idx, nil
	}
	if _, err = f.Seek(idx.size, io.SeekStart); err != nil {
		return nil, err
	}
	scan := bufio.NewReaderSize(f, 64*1024)
	for {
		start := idx.size
		line, err := scan.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if idx.count%PageSize == 0 {
			idx.offsets = append(idx.offsets, start)
		}
		idx.size += int64(len(line))
		idx.count++
	}
	return idx, nil
}
func (s *Store) Page(kind string, number int) (Page, error) {
	p := Page{Kind: kind, Number: max(0, number)}
	if kind != "proxy" && kind != "dns" {
		return p, fmt.Errorf("invalid log kind")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Revision = s.Revision(kind)
	names, err := filepath.Glob(filepath.Join(s.dir, kind+"-*.jsonl"))
	if err != nil {
		return p, err
	}
	slices.Sort(names)
	slices.Reverse(names)
	for _, name := range names {
		idx, err := s.index(name)
		if err != nil {
			return p, err
		}
		p.Total += idx.count
	}
	p.Number = min(p.Number, max(0, (p.Total-1)/PageSize))
	start, end := p.Number*PageSize, min(p.Total, (p.Number+1)*PageSize)
	base := 0
	for _, name := range names {
		idx := s.indexes[name]
		from, to := max(0, start-base), min(idx.count, end-base)
		base += idx.count
		if from >= to {
			continue
		}
		lo, hi := idx.count-to, idx.count-from
		f, err := os.Open(name)
		if err != nil {
			return p, err
		}
		_, err = f.Seek(idx.offsets[lo/PageSize], io.SeekStart)
		if err != nil {
			f.Close()
			return p, err
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4096), 512*1024)
		rows := []Entry{}
		for n := lo / PageSize * PageSize; n < hi && scanner.Scan(); n++ {
			if n < lo {
				continue
			}
			var e Entry
			if err = json.Unmarshal(scanner.Bytes(), &e); err != nil {
				f.Close()
				return p, err
			}
			rows = append(rows, e)
		}
		err = scanner.Err()
		f.Close()
		if err != nil {
			return p, err
		}
		slices.Reverse(rows)
		p.Entries = append(p.Entries, rows...)
	}
	return p, nil
}
func (s *Store) Clear() error { return s.prune(0, true) }
func (s *Store) Prune(days int) error {
	if days == 0 {
		return nil
	}
	return s.prune(days, false)
}
func (s *Store) prune(days int, all bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Removed files must not keep receiving records through open handles.
	s.closeFile(0)
	s.closeFile(1)
	names, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	for _, f := range names {
		name := f.Name()
		if f.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		kind, date, ok := strings.Cut(strings.TrimSuffix(name, ".jsonl"), "-")
		if !ok || kind != "proxy" && kind != "dns" {
			continue
		}
		if !all && date >= cutoff {
			continue
		}
		path := filepath.Join(s.dir, name)
		if err := os.Remove(path); err != nil {
			return err
		}
		delete(s.indexes, path)
	}
	s.revision[0].Add(1)
	s.revision[1].Add(1)
	return nil
}
