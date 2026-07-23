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

	release := make(chan struct{})
	var releaseOnce sync.Once
	triggerRelease := func() {
		releaseOnce.Do(func() {
			close(release)
		})
	}
	var goroutines sync.WaitGroup
	t.Cleanup(func() {
		triggerRelease()
		goroutines.Wait()
		if inflight := hostCallInflight(); inflight != 0 {
			t.Errorf("inflight = %d after joining test goroutines, want 0", inflight)
		}
		for hostCallInflight() > 0 {
			releaseHostCall()
		}
		rearmHostCallAdmissionForTest()
	})

	var started sync.WaitGroup
	started.Add(maxWorkers)
	goroutines.Add(maxWorkers)

	for i := 0; i < maxWorkers; i++ {
		go func() {
			defer goroutines.Done()
			acquireHostCall()
			started.Done()
			<-release
			releaseHostCall()
		}()
	}

	done := make(chan struct{})
	goroutines.Add(1)
	go func() {
		defer goroutines.Done()
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
	goroutines.Add(1)
	go func() {
		defer goroutines.Done()
		acquireHostCall()
		close(blocked)
		releaseHostCall()
	}()

	select {
	case <-blocked:
		t.Fatal("extra acquire should block while gate is full")
	case <-time.After(150 * time.Millisecond):
	}

	triggerRelease()
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked acquire after release")
	}
	goroutines.Wait()
}
