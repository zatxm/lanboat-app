package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zatxm/lanboat-app/internal/coreipc"
	"github.com/zatxm/lanboat-app/internal/traffic"
	"go.uber.org/zap"
)

func (e *Engine) SetRequestLogging(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.configureRequestLog(enabled)
}
func (e *Engine) logging(enabled bool) coreipc.Logging {
	if e.trafficQueue == nil {
		return coreipc.Logging{}
	}
	proxy := e.trafficQueue.store.Enabled("proxy")
	return coreipc.Logging{Proxy: proxy, Request: proxy && enabled, DNS: e.trafficQueue.store.Enabled("dns")}
}

func (e *Engine) configureRequestLog(enabled bool) {
	if e.runtime == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := e.runtime.Call(ctx, coreipc.Request{Op: "logging", Logging: e.logging(enabled)}); err != nil {
		e.log.Warn("change network logging", zap.Error(err))
	}
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

func (e *Engine) SetTrafficStore(store *traffic.Store) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.traffic = store
}
