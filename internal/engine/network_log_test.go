package engine

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	allinone "github.com/zatxm/lanboat-allinone/v26"
	"github.com/zatxm/lanboat-app/internal/corebackend"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestConnectionRecordsAsynchronously(t *testing.T) {
	store := traffic.New(t.TempDir())
	store.SetEnabled(true, false)
	q := newTrafficQueue(store, zap.NewNop())
	now := time.Now()
	for _, dest := range []string{"a.test:443", "b.test:443", "c.test:443"} {
		// Opened is followed by Closed for routed connections; only the close is kept.
		recordConnection(q, allinone.ConnectionLog{Event: allinone.ConnectionOpened, Time: now, Network: "tcp", Destination: dest, RouteOutbound: "proxy"})
		recordConnection(q, allinone.ConnectionLog{Event: allinone.ConnectionClosed, Time: now, Network: "tcp", Destination: dest, RouteOutbound: "policy-group-HK", Outbound: "policy-node-1", Rule: "domain_suffix=test", Elapsed: 4200 * time.Millisecond, Upload: 10, Download: 20, ProcessPath: "/usr/bin/curl"})
	}
	recordConnection(q, allinone.ConnectionLog{Event: allinone.ConnectionFailed, Time: now, Destination: "d.test:443", RouteOutbound: "REJECT", Outbound: "REJECT", Rule: "domain=d.test", Error: errors.New("connection rejected by routing policy")})
	recordConnection(q, allinone.ConnectionLog{Event: allinone.ConnectionFailed, Time: now, Destination: "e.test:443", RouteOutbound: "proxy", Error: context.Canceled})
	// Closing waits for queued records, as engine cleanup does after the runtime stops.
	q.close()
	p, err := store.Page("proxy", 0)
	if err != nil || p.Total != 4 || p.Entries[1].URL != "c.test:443" || p.Entries[3].URL != "a.test:443" {
		t.Fatal("connection records lost or reordered", p, err)
	}
	if e := p.Entries[3]; e.Policy != "policy-group-HK" || e.Outbound != "policy-node-1" || e.Rule != "domain_suffix=test" || e.DurationMS != 4200 || e.Upload != 10 || e.Download != 20 || e.Process != "curl" {
		t.Fatal(e)
	}
	if e := p.Entries[0]; e.Policy != "reject" || e.Outbound != "" || e.Rule != "domain=d.test" || e.Error == "" {
		t.Fatal(e)
	}
	q.push(traffic.Entry{Kind: "proxy", URL: "late"})
	if q.dropped.Load() != 1 {
		t.Fatal("record after close was not counted as dropped")
	}

	full := &trafficQueue{entries: make(chan traffic.Entry, 1), stop: make(chan struct{})}
	full.push(traffic.Entry{URL: "kept"})
	full.push(traffic.Entry{URL: "overflow"})
	if full.dropped.Load() != 1 || len(full.entries) != 1 {
		t.Fatal("full queue blocked or did not count the drop")
	}
}

func TestTrafficQueueReportsWriteFailures(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	// A regular file where the log directory should be makes every write fail.
	store := traffic.New(filepath.Join(blocked, "logs"))
	store.SetEnabled(true, false)
	core, logs := observer.New(zapcore.WarnLevel)
	q := newTrafficQueue(store, zap.New(core))
	for range 5 {
		q.push(traffic.Entry{Kind: "proxy", URL: "x"})
	}
	q.close()
	if logs.Len() != 1 || logs.All()[0].Message != "write connection log" {
		t.Fatal("write failures were not reported once", logs.Len())
	}
}

func TestRuntimeConnectionRecordsCarryTraffic(t *testing.T) {
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	raw := []byte(`{"log":{"disabled":true},"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct","rules":[{"ip_cidr":["127.0.0.0/8"],"outbound":"direct"}]}}`)
	runtime, err := allinone.NewWithLogger(context.Background(), raw, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Start(); err != nil {
		t.Fatal(err)
	}
	store := traffic.New(t.TempDir())
	store.SetEnabled(true, false)
	q := newTrafficQueue(store, zap.NewNop())
	runtime.SetConnectionLogger(func(c allinone.ConnectionLog) {
		if entry, ok := corebackend.ConnectionEntry(c); ok {
			q.push(entry)
		}
	}, true)
	conn, err := runtime.Dial(context.Background(), "tcp", echo.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, make([]byte, 5)); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	runtime.Close()
	q.close()
	p, err := store.Page("proxy", 0)
	if err != nil || p.Total != 1 {
		t.Fatal("expected one record per connection", p, err)
	}
	if e := p.Entries[0]; e.URL != echo.Addr().String() || e.Upload != 5 || e.Download != 5 || e.Rule != "ip_cidr=127.0.0.0/8" || e.Policy != "direct" {
		t.Fatal(e)
	}
}

func recordConnection(q *trafficQueue, c allinone.ConnectionLog) {
	if q.store.Enabled("proxy") {
		if entry, ok := corebackend.ConnectionEntry(c); ok {
			q.push(entry)
		}
	}
}
