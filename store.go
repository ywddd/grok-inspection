package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	// Follow the configured state_file directory so results.json lives in the
	// same (mounted) volume as bans.json and survives container recreation.
	if dir := stateFileDir(); dir != "" {
		return filepath.Join(dir, "results.json")
	}
	// Prefer a stable data dir under the process working directory (CPA cwd).
	return filepath.Join(legacyInspectionDataDir(), "results.json")
}

// stateFileDir returns the directory of the configured ban state_file, or "".
// Both results.json and schedule.json follow it so all persisted plugin state
// shares one durable location instead of splitting across mounted/unmounted dirs.
func stateFileDir() string {
	if p := loadedConfig().StateFile; p != "" {
		return filepath.Dir(p)
	}
	return ""
}

// legacyInspectionDataDir is the historical CWD-relative data dir used before
// state persisted alongside state_file. Kept for read/migration fallback.
func legacyInspectionDataDir() string {
	return filepath.Join("data", "grok-inspection")
}

// legacyResultsPathFor returns the old CWD-relative results.json when it differs
// from the given (current) path, so upgrades migrating to the state_file dir can
// still read pre-existing history exactly once.
func legacyResultsPathFor(current string) string {
	legacy := filepath.Join(legacyInspectionDataDir(), "results.json")
	if filepath.Clean(legacy) == filepath.Clean(current) {
		return ""
	}
	return legacy
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

// writeFileIfAbsent seeds path with raw only when the destination does not
// already exist. Used for one-shot legacy -> state_file migrations.
//
// Writes a same-directory temp file first, then renames into place so a crash
// cannot leave a half-written durable target that would block later O_EXCL
// retries. Never overwrites an existing destination. Best-effort: callers
// ignore errors so a successful legacy read still returns data.
//
// Must not call store/schedule save helpers (those re-take IO mutexes). Callers
// are expected to already hold the relevant IO mutex when seeding.
func writeFileIfAbsent(path string, raw []byte, perm os.FileMode) {
	if strings.TrimSpace(path) == "" || raw == nil {
		return
	}
	if perm == 0 {
		perm = 0o644
	}
	if _, err := os.Stat(path); err == nil {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	if _, err := os.Stat(path); err == nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".grok-inspection-seed-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	promoted := false
	defer func() {
		_ = tmp.Close()
		if !promoted {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		// Windows may reject Chmod on some volumes; contents still matter more.
	}
	if _, err := tmp.Write(raw); err != nil {
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	// Re-check under the caller's IO mutex before promoting.
	if _, err := os.Stat(path); err == nil {
		return
	}
	// rename is atomic on the same volume. Windows fails if dest exists; Unix
	// would replace, so the Stat guard above is required (held under IO mutex).
	if err := os.Rename(tmpName, path); err == nil {
		promoted = true
		return
	}
	// Windows/SMB fallback: only write when dest is still missing (same spirit
	// as replaceFileWithRetry, but never overwrite an existing durable file).
	if _, err := os.Stat(path); err == nil {
		return
	}
	data, errRead := os.ReadFile(tmpName)
	if errRead != nil {
		return
	}
	f, errOpen := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if errOpen != nil {
		return
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(path) // allow a later migration retry
		return
	}
}

func readPersistedSnapshotUnlocked() (persistedSnapshot, error) {
	path := storeFilePath()
	raw, err := os.ReadFile(path)
	fromLegacy := false
	if err != nil {
		if os.IsNotExist(err) {
			if legacy := legacyResultsPathFor(path); legacy != "" {
				if legacyRaw, legacyErr := os.ReadFile(legacy); legacyErr == nil {
					raw, err = legacyRaw, nil
					fromLegacy = true
				}
			}
		}
		if err != nil {
			return persistedSnapshot{}, err
		}
	}
	var snap persistedSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return persistedSnapshot{}, err
	}
	if snap.Results == nil {
		snap.Results = []accountResult{}
	}
	// Seed the durable state_file directory so the next container boot does not
	// depend on the legacy CWD path still being present.
	if fromLegacy {
		writeFileIfAbsent(path, raw, 0o644)
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
