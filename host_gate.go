package main

import (
	"log/slog"
	"sync"
	"time"
)

// hostCallGate bounds concurrent CGO host callbacks (host.http.do / host.auth.*).
// Soft timeouts in probe.go may return early, but abandoned calls still hold a
// slot until the host returns. That prevents timed-out workers from spawning
// unbounded OS threads when upstream hangs.
var (
	hostCallGate = make(chan struct{}, maxWorkers)
	hostCallWG   sync.WaitGroup
)

func acquireHostCall() {
	hostCallWG.Add(1)
	hostCallGate <- struct{}{}
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

// waitHostCalls waits for abandoned soft-timeout host callbacks before unload.
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
//  1. Wait until hostCallWG drains (unbounded -- required for crash-freedom).
//  2. Emit periodic diagnostics while waiting (does NOT make wait bounded).
//  3. tryCancelAbandonedHostCalls is the future host-cancel collaboration hook (no-op).
//
// What remains blocked on host/ABI:
//  - True bounded shutdown without crash needs host-side cancel or unload that
//    waits for in-flight host callbacks before dlclose. Plugin-only timeout
//    return is intentionally rejected as unsafe.
func waitHostCallsForShutdown(diagEvery time.Duration) {
	tryCancelAbandonedHostCalls()
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
}
