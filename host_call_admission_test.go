package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHostCallAdmissionRejectsAfterShutdownClose(t *testing.T) {
	for hostCallInflight() > 0 {
		releaseHostCall()
	}
	rearmHostCallAdmissionForTest()
	t.Cleanup(func() {
		for hostCallInflight() > 0 {
			releaseHostCall()
		}
		rearmHostCallAdmissionForTest()
		beforeHostCallAcquireHook = nil
	})

	enteredHook := make(chan struct{})
	releaseHook := make(chan struct{})
	beforeHostCallAcquireHook = func() {
		select {
		case <-enteredHook:
		default:
			close(enteredHook)
		}
		<-releaseHook
	}

	var childErr error
	childDone := make(chan struct{})
	go func() {
		defer close(childDone)
		childErr = tryAcquireHostCall()
		if childErr == nil {
			releaseHostCall()
		}
	}()

	select {
	case <-enteredHook:
	case <-time.After(2 * time.Second):
		t.Fatal("child did not reach acquire hook")
	}

	waitDone := make(chan struct{})
	go func() {
		waitHostCallsForShutdown(20 * time.Millisecond)
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("waitHostCallsForShutdown hung with no admitted calls")
	}
	if !hostCallAdmissionClosedForTest() {
		t.Fatal("admission must stay closed after shutdown wait")
	}

	close(releaseHook)
	select {
	case <-childDone:
	case <-time.After(2 * time.Second):
		t.Fatal("child hung after release")
	}
	if !errors.Is(childErr, errHostCallsShuttingDown) {
		t.Fatalf("child err=%v want errHostCallsShuttingDown", childErr)
	}
	if hostCallInflight() != 0 {
		t.Fatalf("inflight=%d want 0 after rejected acquire", hostCallInflight())
	}

	done2 := make(chan struct{})
	go func() {
		waitHostCallsForShutdown(20 * time.Millisecond)
		close(done2)
	}()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("second shutdown wait hung")
	}
}

func TestHostCallAdmissionAllowsInflightThroughShutdownWait(t *testing.T) {
	for hostCallInflight() > 0 {
		releaseHostCall()
	}
	rearmHostCallAdmissionForTest()
	t.Cleanup(func() {
		for hostCallInflight() > 0 {
			releaseHostCall()
		}
		rearmHostCallAdmissionForTest()
		beforeHostCallAcquireHook = nil
	})

	if err := tryAcquireHostCall(); err != nil {
		t.Fatal(err)
	}

	waitDone := make(chan struct{})
	go func() {
		waitHostCallsForShutdown(30 * time.Millisecond)
		close(waitDone)
	}()
	select {
	case <-waitDone:
		t.Fatal("shutdown wait returned while admitted host call inflight")
	case <-time.After(80 * time.Millisecond):
	}
	if err := tryAcquireHostCall(); !errors.Is(err, errHostCallsShuttingDown) {
		t.Fatalf("late acquire err=%v", err)
	}
	releaseHostCall()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown wait did not finish after release")
	}
}

func TestHostCallAdmissionConcurrentStressNoAddAfterWaitPanic(t *testing.T) {
	for hostCallInflight() > 0 {
		releaseHostCall()
	}
	rearmHostCallAdmissionForTest()
	t.Cleanup(func() {
		for hostCallInflight() > 0 {
			releaseHostCall()
		}
		rearmHostCallAdmissionForTest()
		beforeHostCallAcquireHook = nil
	})

	const n = 64
	var (
		wg       sync.WaitGroup
		rejected atomic.Int32
		acquired atomic.Int32
	)
	start := make(chan struct{})
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			err := tryAcquireHostCall()
			if err != nil {
				if errors.Is(err, errHostCallsShuttingDown) {
					rejected.Add(1)
				}
				return
			}
			acquired.Add(1)
			time.Sleep(2 * time.Millisecond)
			releaseHostCall()
		}()
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		time.Sleep(1 * time.Millisecond)
		waitHostCallsForShutdown(15 * time.Millisecond)
	}()
	close(start)
	wg.Wait()

	// Join the shutdown Wait before further Wait/rearm/Add so a leftover
	// hostCallWG.Wait cannot race the next test's hostCallWG.Add.
	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("stress shutdown goroutine did not finish")
	}
	if hostCallInflight() != 0 {
		t.Fatalf("leftover inflight=%d", hostCallInflight())
	}
	done := make(chan struct{})
	go func() {
		waitHostCallsForShutdown(10 * time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("final shutdown wait hung after stress")
	}
	if acquired.Load()+rejected.Load() != n {
		t.Fatalf("acquired=%d rejected=%d want sum %d", acquired.Load(), rejected.Load(), n)
	}
	if err := tryAcquireHostCall(); !errors.Is(err, errHostCallsShuttingDown) {
		t.Fatalf("post-stress acquire err=%v", err)
	}
}

func TestCallHostRespectsAdmissionClosed(t *testing.T) {
	for hostCallInflight() > 0 {
		releaseHostCall()
	}
	rearmHostCallAdmissionForTest()
	t.Cleanup(func() {
		rearmHostCallAdmissionForTest()
	})
	closeHostCallAdmission()
	_, err := callHost("host.http.do", map[string]any{"method": "GET", "url": "http://127.0.0.1/"})
	if !errors.Is(err, errHostCallsShuttingDown) {
		t.Fatalf("callHost err=%v want shutting down", err)
	}
	if hostCallInflight() != 0 {
		t.Fatalf("callHost must not take a gate slot when rejected: inflight=%d", hostCallInflight())
	}
}
