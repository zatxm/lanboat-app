package desktop

import (
	"context"
	"fmt"
	"github.com/zatxm/lanboat-app/internal/profile"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRuleUpdateSchedule(t *testing.T) {
	loc := time.FixedZone("test", 8*3600)
	at := func(day, hour, min int) time.Time { return time.Date(2026, 10, day, hour, min, 0, 0, loc) }
	for _, tc := range []struct {
		now, last time.Time
		days      int
		want      bool
	}{
		{at(28, 10, 9), time.Time{}, 1, false}, {at(28, 10, 10), time.Time{}, 1, true},
		{at(29, 10, 9), at(28, 15, 0), 1, false}, {at(29, 10, 10), at(28, 15, 0), 1, true},
		{at(30, 10, 10), at(28, 15, 0), 3, false}, {at(31, 10, 10), at(28, 15, 0), 3, true},
	} {
		if got := DueUpdate(tc.now, tc.last, tc.days); got != tc.want {
			t.Errorf("now %v last %v: got %v", tc.now, tc.last, got)
		}
	}
}

func TestLiveRuleUpdateKeepsConnection(t *testing.T) {
	if os.Getenv("LANBOAT_RULEDATA_LIVE") != "1" {
		t.Skip("live release test disabled")
	}
	a := autoSelectApp(t)
	proxy := httptest.NewServer(http.HandlerFunc(autoProxyTunnel))
	defer proxy.Close()
	n := autoHTTPNode("active", proxy.URL)
	a.config.Nodes = []profile.Node{n}
	a.config.Selected = n.ID
	a.config.Settings.Mode = "rule"
	a.config.Settings.RuleRegion = "cn"
	if err := a.Connect(); err != nil {
		t.Fatal(err)
	}
	var peak atomic.Int64
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				m := ProcessMemory(os.Getpid()).RSS
				for {
					old := peak.Load()
					if m <= old || peak.CompareAndSwap(old, m) {
						break
					}
				}
			}
		}
	}()
	defer func() { close(stop); <-stopped; t.Logf("test process tree peak RSS: %d bytes", peak.Load()) }()
	s := a.GetState().Config.Settings
	s.RuleRegion = "us"
	if err := a.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if !a.GetStatus().Connected {
		t.Fatal("region switch disconnected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := a.UpdateRuleData(ctx); err != nil {
		t.Fatal(err)
	}
	if !a.GetStatus().Connected || a.RuleDataUpdatedAt().IsZero() {
		t.Fatal("update did not preserve connection or timestamp")
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer origin.Close()
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", s.Port))
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "ok" {
		t.Fatal("traffic failed after update")
	}
	result, err := a.TestRule("", "4.2.2.2")
	if err != nil || !strings.HasPrefix(result, "DIRECT") {
		t.Fatal("US region not applied", result, err)
	}
}
