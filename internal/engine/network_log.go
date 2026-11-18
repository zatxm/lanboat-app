package engine

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	allinone "github.com/zatxm/lanboat-allinone"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"go.uber.org/zap"
)

func (e *Engine) SetRequestLogging(enabled, full bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.configureRequestLog(enabled, full)
}
func (e *Engine) configureRequestLog(enabled, full bool) {
	if e.runtime == nil {
		return
	}
	queue := e.trafficQueue
	if queue == nil {
		e.runtime.SetConnectionLogger(nil, false)
		e.runtime.SetRequestLogger(nil, false)
		e.runtime.SetDNSLogger(nil)
		return
	}
	// Connection records are written when a connection ends so they carry its traffic.
	e.runtime.SetConnectionLogger(queue.recordConnection, true)
	if !enabled {
		e.runtime.SetRequestLogger(nil, false)
	} else {
		e.runtime.SetRequestLogger(func(q allinone.RequestLog) {
			queue.push(traffic.Entry{Time: time.Now().Format(time.RFC3339Nano), Kind: "proxy", URL: q.URL, Method: q.Method, Network: "http", Headers: q.Headers, Body: q.Body, BodyTruncated: q.BodyTruncated, Incomplete: q.Incomplete})
		}, full)
	}
	if !queue.store.Enabled("dns") {
		e.runtime.SetDNSLogger(nil)
		return
	}
	e.runtime.SetDNSLogger(func(q allinone.DNSLog) {
		if !queue.store.Enabled("dns") {
			return
		}
		var question, response dns.Msg
		if question.Unpack(q.Query) != nil || len(question.Question) == 0 {
			return
		}
		entry := traffic.Entry{Kind: "dns", Domain: question.Question[0].Name, QueryType: dns.TypeToString[question.Question[0].Qtype], DurationMS: q.Duration.Milliseconds()}
		if q.Error != nil {
			entry.Error = q.Error.Error()
		} else if response.Unpack(q.Response) == nil {
			entry.Result = dns.RcodeToString[response.Rcode]
			for _, answer := range response.Answer {
				entry.Result += "\n" + answer.String()
			}
		}
		entry.Time = time.Now().Format(time.RFC3339Nano)
		queue.push(entry)
	})
}

// trafficQueueSize bounds connection records waiting for disk; beyond it new
// records are dropped and counted rather than slowing connections.
const trafficQueueSize = 1024

// trafficBatch caps how many queued records are written together.
const trafficBatch = 256

// trafficQueue batches connection records for disk. Store.Record does file
// I/O under a lock that page reads also take.
type trafficQueue struct {
	store   *traffic.Store
	log     *zap.Logger
	entries chan traffic.Entry
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
	dropped atomic.Uint64
	// failures since the last report; reports are limited to one a minute.
	failures   int
	reportedAt time.Time
}

// newTrafficQueue reports write failures to log, which must not route back
// into the traffic core.
func newTrafficQueue(store *traffic.Store, log *zap.Logger) *trafficQueue {
	q := &trafficQueue{store: store, log: log, entries: make(chan traffic.Entry, trafficQueueSize), stop: make(chan struct{}), done: make(chan struct{})}
	go q.run()
	return q
}

func (q *trafficQueue) run() {
	defer close(q.done)
	batch := make([]traffic.Entry, 0, trafficBatch)
	for {
		select {
		case entry := <-q.entries:
			q.write(q.collect(append(batch[:0], entry)))
		case <-q.stop:
			// Persist what was accepted before stopping.
			for {
				next := q.collect(batch[:0])
				if len(next) == 0 {
					return
				}
				q.write(next)
			}
		}
	}
}

// collect adds already queued records to batch without waiting.
func (q *trafficQueue) collect(batch []traffic.Entry) []traffic.Entry {
	for len(batch) < trafficBatch {
		select {
		case entry := <-q.entries:
			batch = append(batch, entry)
		default:
			return batch
		}
	}
	return batch
}

func (q *trafficQueue) write(batch []traffic.Entry) {
	err := q.store.RecordBatch(batch)
	clear(batch)
	if err == nil {
		return
	}
	q.failures++
	if now := time.Now(); now.Sub(q.reportedAt) >= time.Minute {
		q.log.Warn("write connection log", zap.Error(err), zap.Int("failed_batches", q.failures))
		q.failures, q.reportedAt = 0, now
	}
}

func (q *trafficQueue) push(entry traffic.Entry) {
	select {
	case <-q.stop:
		q.dropped.Add(1)
		return
	default:
	}
	select {
	case q.entries <- entry:
	default:
		q.dropped.Add(1)
	}
}

// close stops accepting records and waits until queued ones are written.
func (q *trafficQueue) close() {
	q.once.Do(func() { close(q.stop) })
	<-q.done
}

// recordConnection writes one record per connection: when it closes, with its
// traffic and lifetime, or when it fails. It runs on the runtime's log
// goroutine and only enqueues, so disk writes stay batched.
func (q *trafficQueue) recordConnection(c allinone.ConnectionLog) {
	if c.Event == allinone.ConnectionOpened || !q.store.Enabled("proxy") {
		return
	}
	// A client abandoning its own dial is not a network event.
	if errors.Is(c.Error, context.Canceled) || errors.Is(c.Error, net.ErrClosed) {
		return
	}
	entry := traffic.Entry{Time: c.Time.Format(time.RFC3339Nano), Kind: "proxy", URL: c.Destination, Network: c.Network, Domain: c.Domain, Policy: c.RouteOutbound, Rule: c.Rule, DurationMS: c.Elapsed.Milliseconds(), Upload: c.Upload, Download: c.Download, Process: c.ProcessName}
	if entry.Process == "" && c.ProcessPath != "" {
		entry.Process = filepath.Base(c.ProcessPath)
	}
	if c.Outbound != c.RouteOutbound {
		entry.Outbound = c.Outbound
	}
	if entry.Policy == "REJECT" {
		entry.Policy, entry.Outbound = "reject", ""
	}
	if c.Error != nil {
		entry.Error = c.Error.Error()
	}
	q.push(entry)
}

func (e *Engine) SetTrafficStore(store *traffic.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.traffic = store
}
