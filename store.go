package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const storeVersion = 1

// persistedSnapshot is the on-disk form of the last inspection results.
// JSON file is used instead of SQLite for minimal deps and fast full-list read/write.
type persistedSnapshot struct {
	Version         int                         `json:"version"`
	Workers         int                         `json:"workers"`
	IncludeDisabled bool                        `json:"include_disabled"`
	OnlyDisabled    bool                        `json:"only_disabled"`
	StartedAt       string                      `json:"started_at,omitempty"`
	FinishedAt      string                      `json:"finished_at,omitempty"`
	Results         []accountResult             `json:"results"`
	Schedule        persistedInspectionSchedule `json:"schedule,omitempty"`
	SavedAt         string                      `json:"saved_at"`
	// seq is assigned when the snapshot is taken (not when save starts).
	// Stale async flushes must not overwrite a newer final snapshot.
	seq uint64 `json:"-"`
}

var (
	storeMu           sync.Mutex
	storePathOverride string
	// testStorePathDefault is set by TestMain so clearing override never falls
	// back to the repo-relative data/grok-inspection path during tests.
	testStorePathDefault string

	// Serialize disk IO and coalesce concurrent flushes so only the newest
	// snapshot is kept when persistLocked/stop/finish race.
	storeIOMu       sync.Mutex
	storeIOCond     = sync.NewCond(&storeIOMu)
	storePending    *persistedSnapshot
	storeSaveGen    uint64
	storeWrittenGen uint64
	storeWriting    bool
)

func storeFilePath() string {
	storeMu.Lock()
	defer storeMu.Unlock()
	if storePathOverride != "" {
		return storePathOverride
	}
	if dir := firstNonEmpty(os.Getenv("GROK_INSPECTION_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "results.json")
	}
	// Prefer a stable data dir under the process working directory (CPA cwd).
	return filepath.Join("data", "grok-inspection", "results.json")
}

func loadPersistedSnapshot() (persistedSnapshot, error) {
	storeIOMu.Lock()
	defer storeIOMu.Unlock()
	// Wait until any in-flight writer finishes so Windows does not open a
	// half-replaced file (sharing violation during rename/copy).
	for storeWriting {
		storeIOCond.Wait()
	}
	var last error
	for i := 0; i < 8; i++ {
		snap, err := readPersistedSnapshotUnlocked()
		if err == nil {
			return snap, nil
		}
		last = err
		// Brief backoff for antivirus / SMB / Windows file locks.
		storeIOMu.Unlock()
		time.Sleep(time.Duration(5*(i+1)) * time.Millisecond)
		storeIOMu.Lock()
		for storeWriting {
			storeIOCond.Wait()
		}
	}
	return persistedSnapshot{}, last
}

func readPersistedSnapshotUnlocked() (persistedSnapshot, error) {
	path := storeFilePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return persistedSnapshot{}, err
	}
	var snap persistedSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return persistedSnapshot{}, err
	}
	if snap.Results == nil {
		snap.Results = []accountResult{}
	}
	return snap, nil
}

// savePersistedSnapshot writes snap to disk. Concurrent callers are serialized.
// Snapshots carry a seq assigned at creation time; older seq values are discarded
// so a delayed persistLocked goroutine cannot overwrite finish()'s final write.
// The call blocks until this snapshot or a newer one has been written (or fails).
func savePersistedSnapshot(snap persistedSnapshot) error {
	pending := snap

	storeIOMu.Lock()
	seq := pending.seq
	if seq == 0 {
		// Callers outside the engine (tests) still get a monotonic generation.
		storeSaveGen++
		seq = storeSaveGen
		pending.seq = seq
	} else if seq < storeSaveGen {
		// A newer snapshot was already queued or written after this one was taken.
		storeIOMu.Unlock()
		return nil
	} else {
		storeSaveGen = seq
	}

	// Keep only the newest pending payload.
	if storePending != nil && storePending.seq > pending.seq {
		myGen := pending.seq
		for storeWrittenGen < myGen {
			if storeWriting || (storePending != nil && storePending.seq > myGen) {
				storeIOCond.Wait()
				continue
			}
			break
		}
		storeIOMu.Unlock()
		return nil
	}
	storePending = &pending
	myGen := seq

	for storeWrittenGen < myGen {
		if storeWriting {
			storeIOCond.Wait()
			continue
		}

		storeWriting = true
		var writeErr error
		for storePending != nil {
			current := *storePending
			writeGen := current.seq
			if writeGen == 0 {
				writeGen = storeSaveGen
			}
			storePending = nil
			storeIOMu.Unlock()

			writeErr = writePersistedSnapshot(current)

			storeIOMu.Lock()
			if writeErr != nil {
				// Re-queue the failed snapshot only when nothing newer arrived.
				if storePending == nil {
					failed := current
					storePending = &failed
				}
				break
			}
			if writeGen > storeWrittenGen {
				storeWrittenGen = writeGen
			}
		}
		storeWriting = false
		storeIOCond.Broadcast()
		if writeErr != nil && storeWrittenGen < myGen {
			storeIOMu.Unlock()
			return writeErr
		}
	}
	storeIOMu.Unlock()
	return nil
}

func writePersistedSnapshot(snap persistedSnapshot) error {
	path := storeFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	snap.Version = storeVersion
	snap.SavedAt = time.Now().Format(time.RFC3339)
	// Compact JSON: with 1000+ accounts, Indent costs CPU and multiplies disk size.
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := replaceFileWithRetry(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func replaceFileWithRetry(tmp, path string) error {
	var last error
	for i := 0; i < 12; i++ {
		last = os.Rename(tmp, path)
		if last == nil {
			return nil
		}
		// Windows can deny rename/open while another handle still reads the target.
		// Fallback: copy contents over destination then remove temp.
		if data, errRead := os.ReadFile(tmp); errRead == nil {
			if errWrite := os.WriteFile(path, data, 0o644); errWrite == nil {
				_ = os.Remove(tmp)
				return nil
			} else {
				last = errWrite
			}
		}
		time.Sleep(time.Duration(8*(i+1)) * time.Millisecond)
	}
	return last
}
