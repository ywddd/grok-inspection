package main

import (
	"time"
)

func (e *inspectionEngine) loadFromDisk() {
	// Load schedule.json independently so a missing/corrupt results.json cannot
	// prevent schedule settings from applying on restart.
	sched, schedErr := loadInspectionScheduleFromDisk()
	snap, err := loadPersistedSnapshot()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running || e.applying {
		return
	}
	if schedErr == nil {
		e.schedule = sched
	}
	if err != nil {
		// results missing/corrupt: keep schedule if loaded; leave results empty.
		return
	}
	e.results = append([]accountResult(nil), snap.Results...)
	if schedErr != nil {
		e.schedule = normalizePersistedInspectionSchedule(snap.Schedule)
	}
	e.bumpResultsLocked()
	if snap.Workers >= minWorkers && snap.Workers <= maxWorkers {
		e.workers = snap.Workers
	}
	e.includeDisabled = snap.IncludeDisabled
	e.onlyDisabled = snap.OnlyDisabled
	e.total = len(snap.Results)
	if snap.StartedAt != "" {
		if t, errParse := time.Parse(time.RFC3339, snap.StartedAt); errParse == nil {
			e.startedAt = t
		}
	}
	if snap.FinishedAt != "" {
		if t, errParse := time.Parse(time.RFC3339, snap.FinishedAt); errParse == nil {
			e.finishedAt = t
		}
	}
}

// copyPersistedLocked builds a disk snapshot while the caller holds e.mu.
// Each snapshot gets a monotonic seq so delayed async saves cannot overwrite
// a newer finish/stop flush.
func (e *inspectionEngine) copyPersistedLocked() persistedSnapshot {
	e.persistSeq++
	snap := persistedSnapshot{
		Workers:         e.workers,
		IncludeDisabled: e.includeDisabled,
		OnlyDisabled:    e.onlyDisabled,
		Results:         append([]accountResult(nil), e.results...),
		Schedule:        e.schedule,
		seq:             e.persistSeq,
	}
	if !e.startedAt.IsZero() {
		snap.StartedAt = e.startedAt.Format(time.RFC3339)
	}
	if !e.finishedAt.IsZero() {
		snap.FinishedAt = e.finishedAt.Format(time.RFC3339)
	}
	return snap
}

// persistAsyncBeforeSave is an optional test hook invoked inside async
// persistLocked goroutines before disk I/O (nil in production).
var persistAsyncBeforeSave func()

// persistLocked copies under the caller lock, then writes asynchronously so
// status/snapshot callers are not blocked on disk I/O for large result lists.
// Caller must hold e.mu. The writer is tracked on persistWG so shutdown can wait.
func (e *inspectionEngine) persistLocked() {
	e.persistWG.Add(1)
	snap := e.copyPersistedLocked()
	go func(s persistedSnapshot) {
		defer e.persistWG.Done()
		if hook := persistAsyncBeforeSave; hook != nil {
			hook()
		}
		err := savePersistedSnapshot(s)
		e.mu.Lock()
		e.applyPersistResultLocked(s.seq, err)
		e.mu.Unlock()
	}(snap)
}

// waitAsyncPersist blocks until all persistLocked writers finish.
func (e *inspectionEngine) waitAsyncPersist() {
	e.persistWG.Wait()
}

// persist copies under lock and writes outside the critical section.
func (e *inspectionEngine) persist() {
	_ = e.persistSync()
}

// persistSync writes the current results snapshot and returns THIS write's error.
// Callers that must attribute success/failure to a specific job should use the
// returned error rather than reading e.persistError (which can be overwritten by
// a concurrent newer persist).
func (e *inspectionEngine) persistSync() error {
	e.mu.Lock()
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	return e.saveSnapshotAndRecord(snap)
}

// persistSaveHook, when non-nil, wraps the disk save for tests (seq + real save).
// Production leaves it nil. The hook's returned error is what persistSync reports
// and what applyPersistResultLocked records for this seq.
var persistSaveHook func(seq uint64, save func() error) error

func (e *inspectionEngine) saveSnapshotAndRecord(snap persistedSnapshot) error {
	save := func() error { return savePersistedSnapshot(snap) }
	var err error
	if hook := persistSaveHook; hook != nil {
		err = hook(snap.seq, save)
	} else {
		err = save()
	}
	e.mu.Lock()
	e.applyPersistResultLocked(snap.seq, err)
	e.mu.Unlock()
	return err
}

// applyPersistResultLocked updates persistError only for snapshots that are not
// older than the last reported generation. A delayed stale save that returns
// nil must not clear a newer failure.
func (e *inspectionEngine) applyPersistResultLocked(seq uint64, err error) {
	if seq != 0 && seq < e.persistStatusSeq {
		return
	}
	if seq > e.persistStatusSeq {
		e.persistStatusSeq = seq
	}
	if err != nil {
		e.persistError = err.Error()
	} else {
		e.persistError = ""
	}
}
