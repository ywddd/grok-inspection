package main

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

// errHostCallsShuttingDown is returned when a host callback is attempted after
// shutdown closed host-call admission. Soft-timeout child goroutines that race
// past outer Wait must not register on hostCallWG or invoke C.call_host_api.
var errHostCallsShuttingDown = errors.New("host callbacks unavailable: plugin is shutting down")

// hostCallGate bounds concurrent CGO host callbacks (host.http.do / host.auth.*).
// Soft timeouts in probe.go may return early, but abandoned calls still hold a
// slot until the host returns. That prevents timed-out workers from spawning
// unbounded OS threads when upstream hangs.
//
// Admission + WaitGroup lifecycle (same mutex):
//   - tryAcquireHostCall checks hostCallShuttingDown then hostCallWG.Add under
//     hostCallMu. After admission is closed, no new Add can race Wait.
//   - Already-admitted calls (including those blocked on hostCallGate) stay on
//     the WaitGroup until releaseHostCall; shutdown waits for them.
//   - hostCallShuttingDown is permanent in production; tests may rearm.
var (
	hostCallMu           sync.Mutex
	hostCallShuttingDown bool
	hostCallGate         = make(chan struct{}, maxWorkers)
	hostCallWG           sync.WaitGroup
)

// beforeHostCallAcquireHook is test-only; production keeps nil.
// Invoked before the admission check so tests can delay a soft-timeout child
// until after shutdown closes admission.
var beforeHostCallAcquireHook func()

func tryAcquireHostCall() error {
	if beforeHostCallAcquireHook != nil {
		beforeHostCallAcquireHook()
	}
	hostCallMu.Lock()
	if hostCallShuttingDown {
		hostCallMu.Unlock()
		return errHostCallsShuttingDown
	}
	// Add while still holding the admission lock so Wait cannot observe a zero
	// counter between a successful check and Add.
	hostCallWG.Add(1)
	hostCallMu.Unlock()
	hostCallGate <- struct{}{}
	return nil
}

// acquireHostCall is the historical helper used by gate capacity tests.
// Panics if admission is closed (tests must rearm after shutdown).
func acquireHostCall() {
	if err := tryAcquireHostCall(); err != nil {
		panic(err)
	}
}

func releaseHostCall() {
	<-hostCallGate
	hostCallWG.Done()
}

func hostCallInflight() int {
	return len(hostCallGate)
}

func hostCallCapacity() int {
	return cap(hostCallGate)
}

// closeHostCallAdmission permanently rejects new host-call registration.
// Must run before hostCallWG.Wait during plugin shutdown.
func closeHostCallAdmission() {
	hostCallMu.Lock()
	hostCallShuttingDown = true
	hostCallMu.Unlock()
}

// rearmHostCallAdmissionForTest reopens admission after shutdown tests.
func rearmHostCallAdmissionForTest() {
	hostCallMu.Lock()
	hostCallShuttingDown = false
	hostCallMu.Unlock()
}

func hostCallAdmissionClosedForTest() bool {
	hostCallMu.Lock()
	defer hostCallMu.Unlock()
	return hostCallShuttingDown
}

// waitHostCalls waits for abandoned soft-timeout host callbacks before unload.
// Prefer waitHostCallsForShutdown which also closes admission first.
func waitHostCalls(timeout time.Duration) {
	if timeout <= 0 {
		hostCallWG.Wait()
		return
	}
	done := make(chan struct{})
	go func() {
		hostCallWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// shutdownWaitLogger is overridable in tests for diagnostic assertions.
var shutdownWaitLogger = func(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// waitHostCallsForShutdown waits for abandoned soft-timeout host callbacks before
// the host may dlclose this plugin. Host callbacks (host.http.do) are not cancelable
// under the current CPA ABI; returning early would free the shared library under
// still-running Go/CGO stacks and risk process crashes.
//
// HOST ABI LIMITATION (not fully solved in-plugin):
// Unix CPA unload calls plugin Shutdown then immediately dlclose. Soft-timeout
// probes abandon host.http.do with no cancel RPC, so Shutdown must wait for
// hostCallWG or risk dlclose under live Go/CGO stacks (process crash).
//
// What this plugin does safely today:
//  1. Close host-call admission under hostCallMu (no new WG.Add after this).
//  2. Wait until hostCallWG drains (unbounded -- required for crash-freedom).
//  3. Emit periodic diagnostics while waiting (does NOT make wait bounded).
//  4. tryCancelAbandonedHostCalls is the future host-cancel collaboration hook (no-op).
//
// What remains blocked on host/ABI:
//   - True bounded shutdown without crash needs host-side cancel or unload that
//     waits for in-flight host callbacks before dlclose. Plugin-only timeout
//     return is intentionally rejected as unsafe.
func waitHostCallsForShutdown(diagEvery time.Duration) {
	tryCancelAbandonedHostCalls()
	// Close admission BEFORE Wait so a soft-timeout child that lost the race
	// cannot Add after Wait begins (Add/Wait rule + dlclose safety).
	closeHostCallAdmission()
	if diagEvery <= 0 {
		diagEvery = 5 * time.Second
	}
	start := time.Now()
	done := make(chan struct{})
	go func() {
		hostCallWG.Wait()
		close(done)
	}()
	ticker := time.NewTicker(diagEvery)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			shutdownWaitLogger("grok-inspection: shutdown waiting for abandoned host callbacks",
				"inflight", hostCallInflight(),
				"waited", time.Since(start).String(),
				"hint", "host.http.do is not cancelable; host must finish before dlclose is safe",
			)
		}
	}
}

// tryCancelAbandonedHostCalls is a collaboration point for a future CPA host
// cancel method. Current ABI has no cancel; intentionally a no-op.
func tryCancelAbandonedHostCalls() {
	// Future: callHost("host.http.cancel_all", ...) when CPA exposes it.
	// Must not call callHost after admission is closed; keep as no-op / pre-close.
}
