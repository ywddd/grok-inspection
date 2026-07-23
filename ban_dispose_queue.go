package main

import (
	"log/slog"
	"strings"
	"sync"
)

// banDisposeQueueCapacity bounds how many distinct authIDs may wait in the
// dispose queue at once (queued, not yet taken by a worker).
//
// Semantics:
//   - Capacity counts only queued work (pending map / order slice length).
//   - In-flight work (already dequeued by a worker) does NOT consume a slot.
//   - Dedup is by authID while queued: a second enqueue updates revision only.
//   - When full, enqueue returns false without blocking; caller keeps local ban
//     unsynced so restore/retry can finish later.
//   - Shutdown discards queued work (local bans stay unsynced) and waits only
//     for in-flight applies (at most workers count), then flushes ban state.
//
// CPA Usage Manager is single-threaded; usage.handle must never block on
// Management PATCH or slow disk I/O.
const banDisposeQueueCapacity = 256

type banDisposeQueue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	pending  map[string]uint64
	order    []string
	capacity int
	queued   int
	workers  int
	stopping bool
	started  bool
	wg       sync.WaitGroup
	// testHold / testNoStart are flipped only from _test.go helpers in the same package.
	testHold    bool
	testNoStart bool
	inFlight    int
}

var globalBanDispose = newBanDisposeQueue(banDisposeQueueCapacity, 2)

func newBanDisposeQueue(capacity, workers int) *banDisposeQueue {
	if capacity <= 0 {
		capacity = banDisposeQueueCapacity
	}
	if workers <= 0 {
		workers = 1
	}
	q := &banDisposeQueue{
		pending:  make(map[string]uint64),
		order:    make([]string, 0, capacity),
		capacity: capacity,
		workers:  workers,
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// startBanDisposeWorkers starts workers exactly once. WaitGroup.Add happens under
// the same mutex path that stopBanDisposeWorkers waits on, so Add cannot race Wait.
func startBanDisposeWorkers() {
	globalBanDispose.startWorkers()
}

func (q *banDisposeQueue) startWorkers() {
	q.mu.Lock()
	if q.stopping || q.started || q.testNoStart {
		q.mu.Unlock()
		return
	}
	q.started = true
	n := q.workers
	for i := 0; i < n; i++ {
		q.wg.Add(1)
		go q.worker()
	}
	q.mu.Unlock()
}

// stopBanDisposeWorkers discards queued pending work (local bans remain unsynced),
// waits for in-flight applies only, then persists ban state. Safe under concurrent start.
func stopBanDisposeWorkers() {
	q := globalBanDispose
	q.mu.Lock()
	if q.stopping {
		// Already stopping/stopped: wait for workers if any were started.
		started := q.started
		q.mu.Unlock()
		if started {
			q.wg.Wait()
		}
		return
	}
	q.stopping = true
	q.testHold = false
	// Drop pending queue; do not run up to 256 network disables on unload.
	q.pending = make(map[string]uint64)
	q.order = q.order[:0]
	q.queued = 0
	started := q.started
	q.cond.Broadcast()
	q.mu.Unlock()
	if started {
		q.wg.Wait()
	}
	// Persist unsynced local bans so restore/retry can finish after reload.
	if err := saveActiveStoreErr(); err != nil {
		slog.Warn("grok-inspection: failed to persist ban state on dispose shutdown", "error", err)
	}
}

// stopAndWait is used by tests against a private queue instance.
func (q *banDisposeQueue) stopAndWait() {
	q.mu.Lock()
	if q.stopping {
		started := q.started
		q.mu.Unlock()
		if started {
			q.wg.Wait()
		}
		return
	}
	q.stopping = true
	q.testHold = false
	q.pending = make(map[string]uint64)
	q.order = q.order[:0]
	q.queued = 0
	started := q.started
	q.cond.Broadcast()
	q.mu.Unlock()
	if started {
		q.wg.Wait()
	}
}

func (q *banDisposeQueue) worker() {
	defer q.wg.Done()
	for {
		q.mu.Lock()
		for !q.stopping && (q.testHold || len(q.order) == 0) {
			q.cond.Wait()
		}
		// On stop: exit without processing remaining queue (already discarded by stop).
		if q.stopping {
			// Finish any item we already own only if we dequeued before stop set;
			// after stop, order is empty.
			if len(q.order) == 0 {
				q.mu.Unlock()
				return
			}
		}
		if len(q.order) == 0 {
			q.mu.Unlock()
			return
		}
		authID := strings.TrimSpace(q.order[0])
		q.order = q.order[1:]
		rev, exists := q.pending[authID]
		if exists {
			delete(q.pending, authID)
		}
		q.queued = len(q.order)
		q.inFlight++
		q.mu.Unlock()

		if exists && authID != "" {
			applyBanDispose(authID, rev)
		}

		q.mu.Lock()
		q.inFlight--
		if q.stopping && len(q.order) == 0 {
			q.cond.Broadcast()
		}
		q.mu.Unlock()
	}
}

func enqueueBanDispose(authID string, revision uint64) bool {
	q := globalBanDispose
	q.mu.Lock()
	noStart := q.testNoStart
	stopping := q.stopping
	q.mu.Unlock()
	if stopping {
		return false
	}
	if !noStart {
		startBanDisposeWorkers()
	}
	return q.enqueue(authID, revision)
}

// enqueue is the capacity/dedupe core; unit-testable on a local queue with no workers.
// Capacity is queued-only (len(order)), never including in-flight work.
func (q *banDisposeQueue) enqueue(authID string, revision uint64) bool {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopping {
		return false
	}
	if _, exists := q.pending[authID]; exists {
		if revision > q.pending[authID] {
			q.pending[authID] = revision
		}
		return true
	}
	if q.queued >= q.capacity {
		return false
	}
	q.pending[authID] = revision
	q.order = append(q.order, authID)
	q.queued = len(q.order)
	q.cond.Signal()
	return true
}

func (q *banDisposeQueue) queuedCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.queued
}

func (q *banDisposeQueue) pendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func applyBanDispose(authID string, expectedRev uint64) {
	_ = applyBanDisposeWithStore(activeStore, authID, expectedRev)
}

func applyBanDisposeWithStore(store *banStore, authID string, expectedRev uint64) error {
	authID = strings.TrimSpace(authID)
	if authID == "" || store == nil {
		return nil
	}
	return withAuthOp(authID, func() error {
		entry, ok := store.Get(authID)
		if !ok {
			return nil
		}
		if expectedRev != 0 && entry.Revision != expectedRev && entry.CpaSynced {
			return nil
		}
		errDisable := disableAuthInCPA(authID)
		current, still := store.Get(authID)
		if !still {
			return errDisable
		}
		if expectedRev != 0 && current.Revision != expectedRev && current.CpaSynced {
			return errDisable
		}
		if errDisable != nil {
			store.UpdateCpaSyncState(authID, false, sanitizeCPASyncError(errDisable))
			slog.Warn("grok-inspection: background disable failed", "auth_id", authID, "error", errDisable)
		} else if expectedRev == 0 || current.Revision == expectedRev || !current.CpaSynced {
			store.UpdateCpaSyncState(authID, true, "")
		}
		// Coalesced dirty persist (not a free-running goroutine per event).
		markBanStoreDirty()
		return errDisable
	})
}

func sanitizeCPASyncError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	if i := strings.Index(lower, "bearer "); i >= 0 {
		msg = msg[:i+len("bearer ")] + "[REDACTED]"
	}
	if len(msg) > 300 {
		msg = msg[:300] + "..."
	}
	return msg
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
