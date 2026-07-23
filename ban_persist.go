package main

import (
	"log/slog"
	"sync"
	"time"
)

// banStoreSaveFn is the disk write used by the persist worker / final flush.
// Tests may inject a failing implementation; production defaults to activeStore.Save.
var banStoreSaveFn = func(path string) error {
	return activeStore.Save(path)
}

// banPersistWorker coalesces ban-state Save calls from usage/dispose/restore.
// Usage callbacks only mark dirty; a single worker flushes and can stop/flush on shutdown.
type banPersistWorker struct {
	mu      sync.Mutex
	cond    *sync.Cond
	dirty   bool
	path    string
	stop    bool
	started bool
	wg      sync.WaitGroup
}

var globalBanPersist = func() *banPersistWorker {
	w := &banPersistWorker{}
	w.cond = sync.NewCond(&w.mu)
	return w
}()

func startBanPersistWorker() {
	w := globalBanPersist
	w.mu.Lock()
	if w.started || w.stop {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.wg.Add(1)
	w.mu.Unlock()
	go w.loop()
}

func stopBanPersistWorker() {
	w := globalBanPersist
	w.mu.Lock()
	if w.stop {
		started := w.started
		w.mu.Unlock()
		if started {
			w.wg.Wait()
		}
		// Still attempt a final sync save (covers prior failed writes).
		if err := flushBanPersistLocked(); err != nil {
			slog.Warn("grok-inspection: final ban persist after stop failed", "error", err)
		}
		return
	}
	w.stop = true
	w.cond.Broadcast()
	started := w.started
	w.mu.Unlock()
	if started {
		w.wg.Wait()
	}
	// Explicit final synchronous save after the worker has exited. Bounded: one attempt.
	if err := flushBanPersistLocked(); err != nil {
		slog.Warn("grok-inspection: final ban persist on stop failed", "error", err)
	}
}

func markBanStoreDirty() {
	cfg := loadedConfig()
	if !(cfg.PersistState && cfg.StateFile != "") {
		return
	}
	w := globalBanPersist
	w.mu.Lock()
	w.path = cfg.StateFile
	w.dirty = true
	needStart := !w.started && !w.stop
	w.mu.Unlock()
	if needStart {
		startBanPersistWorker()
	}
	w.mu.Lock()
	w.cond.Signal()
	w.mu.Unlock()
}

func (w *banPersistWorker) loop() {
	defer w.wg.Done()
	backoff := 5 * time.Millisecond
	const maxBackoff = 200 * time.Millisecond
	for {
		w.mu.Lock()
		for !w.dirty && !w.stop {
			w.cond.Wait()
		}
		// On stop: exit promptly without retry loops. Final flush runs after Wait.
		if w.stop {
			w.mu.Unlock()
			return
		}
		path := w.path
		// Clear dirty before save; restore on failure so last change is not lost.
		w.dirty = false
		w.mu.Unlock()

		if path == "" {
			cfg := loadedConfig()
			if cfg.PersistState {
				path = cfg.StateFile
			}
		}
		if path == "" {
			continue
		}
		if err := banStoreSaveFn(path); err != nil {
			slog.Warn("grok-inspection: failed to save ban state", "error", err)
			w.mu.Lock()
			w.dirty = true
			stop := w.stop
			w.mu.Unlock()
			if stop {
				return
			}
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			continue
		}
		backoff = 5 * time.Millisecond
		// Brief coalesce window: absorb a burst of dirty marks without one save each.
		time.Sleep(5 * time.Millisecond)
	}
}

func flushBanPersistLocked() error {
	cfg := loadedConfig()
	if !(cfg.PersistState && cfg.StateFile != "") {
		return nil
	}
	path := cfg.StateFile
	w := globalBanPersist
	w.mu.Lock()
	if w.path != "" {
		path = w.path
	}
	w.mu.Unlock()
	if path == "" {
		return nil
	}
	return banStoreSaveFn(path)
}

// flushBanPersistWorker forces a best-effort flush of the dirty flag for restore/shutdown.
func flushBanPersistWorker() error {
	startBanPersistWorker()
	w := globalBanPersist
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		dirty := w.dirty
		stop := w.stop
		w.mu.Unlock()
		if stop {
			break
		}
		if !dirty {
			return flushBanPersistLocked()
		}
		w.mu.Lock()
		w.cond.Signal()
		w.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	return flushBanPersistLocked()
}
