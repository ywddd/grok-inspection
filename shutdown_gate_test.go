package main

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// rearmEngineAfterShutdownForTest clears permanent unload gate after tests that
// call engine.shutdown(). Production never clears shuttingDown.
func rearmEngineAfterShutdownForTest() {
	engine.mu.Lock()
	engine.shuttingDown = false
	engine.stopped = false
	engine.applyDraining = false
	engine.running = false
	engine.applying = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	rearmBanDisposeWorkersForTest()
	stopBanPersistWorkerForTest()
	// unban job may have been stopped; allow fresh claims
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.current = ""
	unbanJob.mu.Unlock()
}

// TestShutdownGateRejectsStartsDuringWait proves that after shutdown sets the
// permanent gate and blocks in runWG.Wait, concurrent start/startApply/
// startAction/claimUnbanSlot are refused, cannot clear stopped, and cannot Add
// workers. Deterministic without relying on the race detector.
func TestShutdownGateRejectsStartsDuringWait(t *testing.T) {
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	// Hold runWG so shutdown blocks in Wait after the gate is set.
	releaseProducer := make(chan struct{})
	engine.runWG.Add(1)
	go func() {
		defer engine.runWG.Done()
		<-releaseProducer
	}()

	shutdownDone := make(chan struct{})
	go func() {
		engine.shutdown()
		close(shutdownDone)
	}()

	// Wait until permanent gate is visible (not a sleep race on Wait itself).
	deadline := time.Now().Add(2 * time.Second)
	for {
		engine.mu.Lock()
		gated := engine.shuttingDown && engine.stopped
		engine.mu.Unlock()
		if gated {
			break
		}
		if time.Now().After(deadline) {
			close(releaseProducer)
			t.Fatal("shutdown did not set shuttingDown in time")
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Snapshot stopped; concurrent starts must not clear it.
	engine.mu.Lock()
	stoppedBefore := engine.stopped
	shuttingBefore := engine.shuttingDown
	engine.mu.Unlock()
	if !stoppedBefore || !shuttingBefore {
		close(releaseProducer)
		t.Fatalf("expected stopped+shuttingDown, got stopped=%v shutting=%v", stoppedBefore, shuttingBefore)
	}

	var startAdds int32
	// Patch is behavioral: rejected starts must not leave running=true or add work.
	errStart := engine.start(startRequest{Lang: "en", Workers: 1})
	if errStart == nil {
		atomic.AddInt32(&startAdds, 1)
		close(releaseProducer)
		t.Fatal("start must refuse after shutdown gate")
	}

	errApply := engine.startApply(applyRequest{Lang: "en", ForceAction: "disable", AuthIndexes: []string{"x"}}, "", http.Header{})
	if errApply == nil {
		atomic.AddInt32(&startAdds, 1)
		close(releaseProducer)
		t.Fatal("startApply must refuse after shutdown gate")
	}

	_, _, errAction := engine.startAction(actionRequest{Lang: "en", Name: "a1", Disabled: true}, "", http.Header{})
	if errAction == nil {
		atomic.AddInt32(&startAdds, 1)
		close(releaseProducer)
		t.Fatal("startAction must refuse after shutdown gate")
	}

	if _, errClaim := claimUnbanSlot(1, "x", true); errClaim == nil {
		// If claim wrongly succeeded, release the slot to avoid hang.
		releaseUnbanSlot(0)
		unbanJob.wg.Done()
		close(releaseProducer)
		t.Fatal("claimUnbanSlot must refuse after shutdown gate")
	}

	engine.mu.Lock()
	if !engine.stopped {
		engine.mu.Unlock()
		close(releaseProducer)
		t.Fatal("concurrent start must not clear stopped during shutdown")
	}
	if !engine.shuttingDown {
		engine.mu.Unlock()
		close(releaseProducer)
		t.Fatal("shuttingDown must remain set")
	}
	if engine.running || engine.applying || engine.actionInFlight != 0 {
		running, applying, inflight := engine.running, engine.applying, engine.actionInFlight
		engine.mu.Unlock()
		close(releaseProducer)
		t.Fatalf("no new work: running=%v applying=%v actionInFlight=%d", running, applying, inflight)
	}
	engine.mu.Unlock()

	unbanJob.mu.Lock()
	unbanRunning := unbanJob.running
	unbanJob.mu.Unlock()
	if unbanRunning {
		close(releaseProducer)
		t.Fatal("claimUnbanSlot must not start unban during shutdown")
	}

	if atomic.LoadInt32(&startAdds) != 0 {
		close(releaseProducer)
		t.Fatalf("unexpected accepted starts: %d", startAdds)
	}

	close(releaseProducer)
	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung")
	}
}

// TestNormalStopDoesNotPermanentGate ensures user cancel (stop) is not permanent
// shutdown: after stop finishes, a new start claim is still allowed once idle.
func TestNormalStopDoesNotPermanentGate(t *testing.T) {
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	engine.mu.Lock()
	if engine.shuttingDown {
		engine.mu.Unlock()
		t.Fatal("precondition: not shutting down")
	}
	engine.mu.Unlock()

	engine.stopWithLang("en")
	engine.mu.Lock()
	if engine.shuttingDown {
		engine.mu.Unlock()
		t.Fatal("stop must not set shuttingDown")
	}
	// stop sets stopped; start is allowed to clear it when not shutting down.
	engine.stopped = true
	engine.mu.Unlock()

	// claimUnbanSlot should work after normal stop (idle engine).
	runID, err := claimUnbanSlot(0, "", false)
	if err != nil {
		t.Fatalf("claim after normal stop: %v", err)
	}
	releaseUnbanSlot(runID)

	engine.mu.Lock()
	// Simulate start clearing stopped (as start does under lock when allowed).
	if engine.shuttingDown {
		engine.mu.Unlock()
		t.Fatal("still not permanent")
	}
	engine.stopped = false
	engine.mu.Unlock()
}

// TestShutdownGateIsIdempotentAndSerializesWithClaim hammers claimUnbanSlot while
// shutdown is blocked in Wait; all claims must fail and WaitGroup must not grow.
func TestShutdownGateIsIdempotentAndSerializesWithClaim(t *testing.T) {
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	releaseProducer := make(chan struct{})
	engine.runWG.Add(1)
	go func() {
		defer engine.runWG.Done()
		<-releaseProducer
	}()

	var shutdownOnce sync.WaitGroup
	shutdownOnce.Add(2)
	go func() {
		defer shutdownOnce.Done()
		engine.shutdown()
	}()
	go func() {
		defer shutdownOnce.Done()
		// Second concurrent shutdown must not panic / Add after Wait.
		time.Sleep(5 * time.Millisecond)
		engine.shutdown()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		engine.mu.Lock()
		gated := engine.shuttingDown
		engine.mu.Unlock()
		if gated {
			break
		}
		if time.Now().After(deadline) {
			close(releaseProducer)
			t.Fatal("gate not set")
		}
		time.Sleep(1 * time.Millisecond)
	}

	var wg sync.WaitGroup
	var accepted atomic.Int32
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := claimUnbanSlot(1, "race", true); err == nil {
				accepted.Add(1)
				releaseUnbanSlot(0)
				unbanJob.wg.Done()
			}
			_ = engine.start(startRequest{Lang: "en", Workers: 1})
			_, _, _ = engine.startAction(actionRequest{Lang: "en", Name: "n", Disabled: true}, "", nil)
		}()
	}
	wg.Wait()
	if accepted.Load() != 0 {
		close(releaseProducer)
		t.Fatalf("accepted %d unban claims during shutdown", accepted.Load())
	}

	close(releaseProducer)
	done := make(chan struct{})
	go func() {
		shutdownOnce.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdowns hung")
	}
}
