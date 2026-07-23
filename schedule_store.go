package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// scheduleIOMu serializes all schedule.json reads/writes so concurrent POST and
// runtime status flushes cannot clobber each other on Windows/SMB.
var scheduleIOMu sync.Mutex

// scheduleIOTestHook is optional; tests may block inside the schedule IO critical
// section (already under scheduleTxnMu for user/runtime writers) to prove ordering.
var scheduleIOTestHook func()

func scheduleFilePath() string {
	return filepath.Join(filepath.Dir(storeFilePath()), "schedule.json")
}

// saveInspectionScheduleSync durably writes schedule settings to a small file.
// POST /schedule must only report success after this returns nil.
func saveInspectionScheduleSync(cfg persistedInspectionSchedule) error {
	cfg = normalizePersistedInspectionSchedule(cfg)
	scheduleIOMu.Lock()
	defer scheduleIOMu.Unlock()
	if scheduleIOTestHook != nil {
		scheduleIOTestHook()
	}
	path := scheduleFilePath()
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".grok-inspection-schedule-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replaceFileWithRetry(tempName, path)
}

func loadScheduleJSONUnlocked() (persistedInspectionSchedule, error) {
	path := scheduleFilePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return persistedInspectionSchedule{}, err
	}
	var cfg persistedInspectionSchedule
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return persistedInspectionSchedule{}, err
	}
	return normalizePersistedInspectionSchedule(cfg), nil
}

// loadInspectionScheduleFromDisk prefers schedule.json; falls back to results.json
// schedule field for migration from older installs.
func loadInspectionScheduleFromDisk() (persistedInspectionSchedule, error) {
	scheduleIOMu.Lock()
	cfg, err := loadScheduleJSONUnlocked()
	scheduleIOMu.Unlock()
	if err == nil {
		return cfg, nil
	}
	if err != nil && !os.IsNotExist(err) {
		// Corrupt schedule.json: still try migration, but surface if both fail.
		schedErr := err
		snap, errSnap := loadPersistedSnapshot()
		if errSnap == nil {
			return normalizePersistedInspectionSchedule(snap.Schedule), nil
		}
		return persistedInspectionSchedule{}, schedErr
	}
	// Missing schedule.json: migrate from results.json when present.
	snap, errSnap := loadPersistedSnapshot()
	if errSnap != nil {
		if os.IsNotExist(errSnap) {
			return defaultInspectionSchedule(), nil
		}
		if _, statErr := os.Stat(storeFilePath()); os.IsNotExist(statErr) {
			return defaultInspectionSchedule(), nil
		}
		// results unreadable — schedule still defaults so plugin can boot
		return defaultInspectionSchedule(), nil
	}
	return normalizePersistedInspectionSchedule(snap.Schedule), nil
}
