package main

import (
	"sync"
	"testing"
	"time"
)

func TestHostCallGateCapacityMatchesMaxWorkers(t *testing.T) {
	if hostCallCapacity() != maxWorkers {
		t.Fatalf("host call capacity = %d, want %d", hostCallCapacity(), maxWorkers)
	}
}

func TestHostCallGateBoundsConcurrentAcquires(t *testing.T) {
	// Drain leftovers then re-open admission; previous shutdown tests leave
	// hostCallShuttingDown=true and may still have Wait in flight until joined.
	for hostCallInflight() > 0 {
		releaseHostCall()
	}
	rearmHostCallAdmissionForTest()
	t.Cleanup(func() {
		for hostCallInflight() > 0 {
			releaseHostCall()
		}
		rearmHostCallAdmissionForTest()
	})

	var started sync.WaitGroup
	var release sync.WaitGroup
	started.Add(maxWorkers)
	release.Add(1)

	for i := 0; i < maxWorkers; i++ {
		go func() {
			acquireHostCall()
			started.Done()
			release.Wait()
			releaseHostCall()
		}()
	}

	done := make(chan struct{})
	go func() {
		started.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for maxWorkers acquires")
	}

	if hostCallInflight() != maxWorkers {
		t.Fatalf("inflight = %d, want %d", hostCallInflight(), maxWorkers)
	}

	blocked := make(chan struct{})
	go func() {
		acquireHostCall()
		close(blocked)
		releaseHostCall()
	}()

	select {
	case <-blocked:
		t.Fatal("extra acquire should block while gate is full")
	case <-time.After(150 * time.Millisecond):
	}

	release.Done()
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked acquire after release")
	}
}
