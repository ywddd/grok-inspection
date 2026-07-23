package main

import (
	"log/slog"
	"strings"
	"sync"
	"time"
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
//
// CPA Usage Manager is single-threaded; usage.handle must never block on
// Management PATCH or slow disk I/O.
const banDisposeQueueCapacity = 256

type banDisposeQueue struct {
	mu          sync.Mutex
	cond        *sync.Cond
	pending     map[string]uint64
	order       []string
	capacity    int
	queued      int
	workers     int
	stopping    bool
	wg          sync.WaitGroup
	stopOnce    sync.Once
	startOnce   sync.Once
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

func startBanDisposeWorkers() {
	globalBanDispose.startOnce.Do(func() {
		for i := 0; i < globalBanDispose.workers; i++ {
			globalBanDispose.wg.Add(1)
			go globalBanDispose.worker()
		}
	})
}

func stopBanDisposeWorkers() {
	globalBanDispose.stopOnce.Do(func() {
		q := globalBanDispose
		q.mu.Lock()
		q.stopping = true
		q.testHold = false
		q.cond.Broadcast()
		q.mu.Unlock()
		q.wg.Wait()
	})
}

func (q *banDisposeQueue) worker() {
	defer q.wg.Done()
	for {
		q.mu.Lock()
		for !q.stopping && (q.testHold || len(q.order) == 0) {
			q.cond.Wait()
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
		if err := saveActiveStoreErr(); err != nil {
			slog.Warn("grok-inspection: failed to save ban state after dispose", "error", err)
		}
		return errDisable
	})
}

func applyBanDisposeForTest(authID string, expectedRev uint64) error {
	return applyBanDisposeWithStore(activeStore, authID, expectedRev)
}

func resetBanDisposeQueueForTest(t interface {
	Helper()
	Cleanup(func())
}) {
	t.Helper()
	q := globalBanDispose
	q.mu.Lock()
	wasHold := q.testHold
	wasNoStart := q.testNoStart
	q.testHold = true
	q.pending = make(map[string]uint64)
	q.order = q.order[:0]
	q.queued = 0
	q.cond.Broadcast()
	q.mu.Unlock()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		n := q.inFlight
		q.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	q.mu.Lock()
	q.testHold = wasHold
	q.testNoStart = wasNoStart
	if !wasHold {
		q.cond.Broadcast()
	}
	q.mu.Unlock()
	t.Cleanup(func() {
		q.mu.Lock()
		hold := q.testHold
		noStart := q.testNoStart
		q.testHold = true
		q.pending = make(map[string]uint64)
		q.order = q.order[:0]
		q.queued = 0
		q.cond.Broadcast()
		q.mu.Unlock()
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			q.mu.Lock()
			n := q.inFlight
			q.mu.Unlock()
			if n == 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		q.mu.Lock()
		q.testHold = hold
		q.testNoStart = noStart
		if !hold {
			q.cond.Broadcast()
		}
		q.mu.Unlock()
	})
}

func pauseBanDisposeWorkersForTest(t interface {
	Helper()
	Cleanup(func())
}) {
	t.Helper()
	q := globalBanDispose
	q.mu.Lock()
	q.testHold = true
	q.testNoStart = true
	q.cond.Broadcast()
	q.mu.Unlock()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		n := q.inFlight
		q.mu.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Cleanup(func() {
		q.mu.Lock()
		q.testHold = false
		q.testNoStart = false
		q.cond.Broadcast()
		q.mu.Unlock()
	})
}

// markBanDisposeQueueFullForTest forces queued capacity full under lock.
func markBanDisposeQueueFullForTest(t interface {
	Helper()
}) {
	t.Helper()
	q := globalBanDispose
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := 0; i < q.capacity; i++ {
		if q.queued >= q.capacity {
			break
		}
		id := "cap-fill-" + itoa(i)
		if _, ok := q.pending[id]; ok {
			continue
		}
		q.pending[id] = 1
		q.order = append(q.order, id)
		q.queued = len(q.order)
	}
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


// freezeAndWaitBanDisposeIdleForTest freezes workers and waits until no apply is
// in flight. Call before restoring global management dial settings so cleanup
// cannot race background disableAuthInCPA. Deterministic: waits on inFlight, no sleep-only gates.
func freezeAndWaitBanDisposeIdleForTest(t interface {
	Helper()
	Fatalf(string, ...any)
}) {
	t.Helper()
	q := globalBanDispose
	q.mu.Lock()
	q.testHold = true
	q.testNoStart = true
	q.cond.Broadcast()
	q.mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for {
		q.mu.Lock()
		n := q.inFlight
		q.mu.Unlock()
		if n == 0 {
			// Drop queued work so unfreeze cannot resurrect stale disables against restored dial.
			q.mu.Lock()
			q.pending = make(map[string]uint64)
			q.order = q.order[:0]
			q.queued = 0
			q.mu.Unlock()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ban dispose still in flight after freeze: inFlight=%d", n)
		}
		// Park only while an in-flight apply is still inside disableAuthInCPA/save.
		time.Sleep(1 * time.Millisecond)
	}
}

// unfreezeBanDisposeWorkersForTest releases a freeze from freezeAndWait...
func unfreezeBanDisposeWorkersForTest() {
	q := globalBanDispose
	q.mu.Lock()
	q.testHold = false
	q.testNoStart = false
	q.cond.Broadcast()
	q.mu.Unlock()
}

func banDisposePendingCountForTest() int { return globalBanDispose.pendingCount() }
func banDisposeQueuedCountForTest() int  { return globalBanDispose.queuedCount() }

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