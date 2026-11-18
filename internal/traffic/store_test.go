package traffic

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPagedLogsPersistenceAndSwitches(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	if err := s.Record(Entry{Kind: "proxy", URL: "disabled"}); err != nil {
		t.Fatal(err)
	}
	if files, _ := os.ReadDir(dir); len(files) != 0 {
		t.Fatal("disabled logging wrote files")
	}
	s.SetEnabled(true, false)
	for i := 0; i < 617; i++ {
		if err := s.Record(Entry{Kind: "proxy", URL: fmt.Sprintf("request-%03d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Record(Entry{Kind: "dns", Domain: "disabled.invalid"})
	p, err := s.Page("proxy", 0)
	if err != nil || p.Total != 617 || len(p.Entries) != 40 || p.Entries[0].URL != "request-616" {
		t.Fatal(p, err)
	}
	// Reopening scans once, then uses a sparse index and incrementally indexes appends.
	s = New(dir)
	s.SetEnabled(true, true)
	p, err = s.Page("proxy", 15)
	if err != nil || p.Number != 15 || len(p.Entries) != 17 || p.Entries[16].URL != "request-000" {
		t.Fatal(p, err)
	}
	if err = s.Record(Entry{Kind: "proxy", URL: "appended"}); err != nil {
		t.Fatal(err)
	}
	p, err = s.Page("proxy", 0)
	if err != nil || p.Total != 618 || p.Entries[0].URL != "appended" {
		t.Fatal(p, err)
	}
	_ = s.Record(Entry{Kind: "dns", Domain: "example.invalid", Result: "192.0.2.1", Resolver: "udp://127.0.0.1:5353", DurationMS: 14})
	dns, _ := s.Page("dns", 0)
	if dns.Total != 1 || dns.Entries[0].DurationMS != 14 {
		t.Fatal(dns)
	}
	for _, f := range mustFiles(t, dir) {
		info, err := os.Stat(f)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("private log permissions", err)
		}
	}
	stale := filepath.Join(dir, "dns-2000-01-01.jsonl")
	_ = os.WriteFile(stale, []byte("{}\n"), 0600)
	if err = s.Prune(1); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("retention did not prune")
	}
	if err = s.Clear(); err != nil {
		t.Fatal(err)
	}
	p, _ = s.Page("proxy", 0)
	dns, _ = s.Page("dns", 0)
	if p.Total != 0 || dns.Total != 0 {
		t.Fatal("clear left indexed logs")
	}
	if _, err = s.Page("../app", 0); err == nil {
		t.Fatal("invalid log kind accepted")
	}
}
func mustFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}
func TestConcurrentLogsAndPaging(t *testing.T) {
	s := New(t.TempDir())
	s.SetEnabled(true, true)
	var wg sync.WaitGroup
	for k := 0; k < 4; k++ {
		wg.Go(func() {
			for i := 0; i < 80; i++ {
				if err := s.Record(Entry{Kind: "proxy", Time: time.Now().Format(time.RFC3339Nano), URL: "fixture"}); err != nil {
					t.Error(err)
				}
				if _, err := s.Page("proxy", 0); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	p, err := s.Page("proxy", 0)
	if err != nil || p.Total != 320 {
		t.Fatal(p, err)
	}
}
func TestRecordBatchAndReopen(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.SetEnabled(true, false)
	if err := s.RecordBatch([]Entry{{Kind: "proxy", URL: "one"}, {Kind: "dns", Domain: "disabled"}, {Kind: "proxy", URL: "two"}}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Page("proxy", 0)
	if err != nil || p.Total != 2 || p.Entries[0].URL != "two" || p.Revision != 2 {
		t.Fatal(p, err)
	}
	if files := mustFiles(t, dir); len(files) != 1 {
		t.Fatal("disabled kind created a file", files)
	}
	// A file removed behind the store's back is recreated instead of
	// silently receiving records through the stale handle.
	for _, f := range mustFiles(t, dir) {
		os.Remove(f)
	}
	time.Sleep(1100 * time.Millisecond)
	if err = s.Record(Entry{Kind: "proxy", URL: "after"}); err != nil {
		t.Fatal(err)
	}
	p, err = s.Page("proxy", 0)
	if err != nil || p.Total != 1 || p.Entries[0].URL != "after" {
		t.Fatal("record after external removal was lost", p, err)
	}
}
