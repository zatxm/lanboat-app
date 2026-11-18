package ruledata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBundledDataAndFailedInstall(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	current, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := Index(current)
	if err != nil || len(entries) < 2000 {
		t.Fatalf("index %d: %v", len(entries), err)
	}
	for _, name := range []string{"srs/geoip/cn.srs", "srs/geoip/us.srs", "srs/geosite/cn.srs", "srs/geosite/private.srs"} {
		if _, err = os.Stat(filepath.Join(current, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = install(root, bytes.NewReader([]byte("invalid"))); err == nil {
		t.Fatal("accepted invalid archive")
	}
	if after, err := Current(root); err != nil || after != current {
		t.Fatal("failed update changed active generation", after, err)
	}
}
func TestRegionSelection(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	if Region("") != "us" || Region("cn") != "cn" || ValidRegion("zz") {
		t.Fatal("invalid region selection")
	}
	t.Setenv("LC_ALL", "C.UTF-8")
	t.Setenv("TZ", "Asia/Shanghai")
	if Region("") != "cn" {
		t.Fatalf("time zone did not select CN: %s", Region(""))
	}
}

func TestFetchChecksReleaseChecksum(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	old, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(initial)
	good := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA256SUMS":
			if good {
				fmt.Fprintf(w, "%x  %s\n", sum, bundleName)
			} else {
				fmt.Fprintf(w, "%064d  %s\n", 0, bundleName)
			}
		case "/" + bundleName:
			_, _ = w.Write(initial)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	saved := releaseURL
	releaseURL = server.URL + "/"
	defer func() { releaseURL = saved }()
	if err = Fetch(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	current, err := Current(root)
	if err != nil || current == old {
		t.Fatal("successful update not activated", err)
	}
	good = false
	if err = Fetch(context.Background(), root); err == nil {
		t.Fatal("invalid checksum accepted")
	}
	if after, _ := Current(root); after != current {
		t.Fatal("failed update changed data")
	}
}

func TestLiveReleaseFetch(t *testing.T) {
	if os.Getenv("LANBOAT_RULEDATA_LIVE") != "1" {
		t.Skip("live release test disabled")
	}
	root := t.TempDir()
	if err := Fetch(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	dir, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := Index(dir)
	if err != nil || len(entries) < 2000 {
		t.Fatalf("release index: %d %v", len(entries), err)
	}
}

func TestCompactInactivePreservesOnBadArchive(t *testing.T) {
	root := t.TempDir()
	if err := Ensure(root); err != nil {
		t.Fatal(err)
	}
	old, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	next, err := Unpack(root, bytes.NewReader(initial))
	if err != nil {
		t.Fatal(err)
	}
	if err = Activate(root, next); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(old+".tar.gz", []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = CompactInactive(root); err == nil {
		t.Fatal("accepted corrupt archive")
	}
	if err = VerifyDir(old); err != nil {
		t.Fatal("original generation lost", err)
	}
	current, err := Current(root)
	if err != nil || current != next {
		t.Fatal(current, err)
	}
}
