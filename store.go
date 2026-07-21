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
	Version         int             `json:"version"`
	Workers         int             `json:"workers"`
	IncludeDisabled bool            `json:"include_disabled"`
	OnlyDisabled    bool            `json:"only_disabled"`
	StartedAt       string          `json:"started_at,omitempty"`
	FinishedAt      string          `json:"finished_at,omitempty"`
	Results         []accountResult `json:"results"`
	SavedAt         string          `json:"saved_at"`
	// Schedule is optional periodic full-inspect + auto-apply settings (password is not stored).
	Schedule *persistedSchedule `json:"schedule,omitempty"`
}

// persistedSchedule is the durable part of the auto schedule (no secrets).
type persistedSchedule struct {
	Enabled           bool   `json:"enabled"`
	IntervalMinutes   int    `json:"interval_minutes"`
	AutoApply         bool   `json:"auto_apply"`
	Workers           int    `json:"workers"`
	IncludeDisabled   bool   `json:"include_disabled"`
	OnlyDisabled      bool   `json:"only_disabled"`
	LastRunAt         string `json:"last_run_at,omitempty"`
	NextRunAt         string `json:"next_run_at,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	LastAutoSummary   string `json:"last_auto_summary,omitempty"`
}

var (
	storeMu         sync.Mutex
	storePathOverride string
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

func setStoreFilePathForTest(path string) {
	storeMu.Lock()
	defer storeMu.Unlock()
	storePathOverride = path
}

func loadPersistedSnapshot() (persistedSnapshot, error) {
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

func savePersistedSnapshot(snap persistedSnapshot) error {
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
	return os.Rename(tmp, path)
}
