package main

import (
	"log/slog"
	"sync"
	"time"
)

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
		return
	}
	w.stop = true
	// Final flush intent: keep dirty so loop writes before exit.
	w.cond.Broadcast()
	started := w.started
	w.mu.Unlock()
	if started {
		w.wg.Wait()
	} else {
		// Never started: best-effort sync flush.
		_ = flushBanPersistLocked()
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
	for {
		w.mu.Lock()
		for !w.dirty && !w.stop {
			w.cond.Wait()
		}
		if w.stop && !w.dirty {
			w.mu.Unlock()
			return
		}
		path := w.path
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
		if err := activeStore.Save(path); err != nil {
			slog.Warn("grok-inspection: failed to save ban state", "error", err)
		}
		// Brief coalesce window: absorb a burst of dirty marks without one save each.
		time.Sleep(5 * time.Millisecond)
		w.mu.Lock()
		if w.stop && !w.dirty {
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
	}
}

func flushBanPersistLocked() error {
	cfg := loadedConfig()
	if !(cfg.PersistState && cfg.StateFile != "") {
		return nil
	}
	return activeStore.Save(cfg.StateFile)
}

// flushBanPersistWorker forces a best-effort flush of the dirty flag for restore/shutdown.
func flushBanPersistWorker() error {
	startBanPersistWorker()
	w := globalBanPersist
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		dirty := w.dirty
		w.mu.Unlock()
		if !dirty {
			// One more direct save to ensure disk has latest.
			return flushBanPersistLocked()
		}
		w.mu.Lock()
		w.cond.Signal()
		w.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	return flushBanPersistLocked()
}
