package main

import (
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
