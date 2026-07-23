package main

import (
	"net/http"
	"testing"
	"time"
)

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
		time.Sleep(1 * time.Millisecond)
	}
}

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

// installCPAManagementDialForTest swaps management dial under lock. Cleanup freezes
// ban dispose workers until idle, then restores dial — safe for Usage/async disable tests.
func installCPAManagementDialForTest(t interface {
	Helper()
	Cleanup(func())
	Fatalf(string, ...any)
}, baseURL string, do func(*http.Request) (*http.Response, error)) {
	t.Helper()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementDial(baseURL, do)
	t.Cleanup(func() {
		freezeAndWaitBanDisposeIdleForTest(t)
		setCPAManagementDial(oldBase, oldDo)
		unfreezeBanDisposeWorkersForTest()
	})
}

// Ensure testing import used when type-asserting TB helpers in some call sites.
var _ = testing.Short

// ---- test-only queue helpers (not compiled into production) ----

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
