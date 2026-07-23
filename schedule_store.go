package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func scheduleFilePath() string {
	return filepath.Join(filepath.Dir(storeFilePath()), "schedule.json")
}

// saveInspectionScheduleSync durably writes schedule settings to a small file.
// POST /schedule must only report success after this returns nil.
func saveInspectionScheduleSync(cfg persistedInspectionSchedule) error {
	cfg = normalizePersistedInspectionSchedule(cfg)
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
	return os.Rename(tempName, path)
}

// loadInspectionScheduleFromDisk prefers schedule.json; falls back to results.json
// schedule field for migration from older installs.
func loadInspectionScheduleFromDisk() (persistedInspectionSchedule, error) {
	path := scheduleFilePath()
	raw, err := os.ReadFile(path)
	if err == nil {
		var cfg persistedInspectionSchedule
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return persistedInspectionSchedule{}, err
		}
		return normalizePersistedInspectionSchedule(cfg), nil
	}
	if !os.IsNotExist(err) {
		return persistedInspectionSchedule{}, err
	}
	// Migration: read schedule embedded in results.json.
	snap, errSnap := loadPersistedSnapshot()
	if errSnap != nil {
		if os.IsNotExist(errSnap) {
			return defaultInspectionSchedule(), nil
		}
		// results may be missing on first boot.
		if _, statErr := os.Stat(storeFilePath()); os.IsNotExist(statErr) {
			return defaultInspectionSchedule(), nil
		}
		return persistedInspectionSchedule{}, errSnap
	}
	return normalizePersistedInspectionSchedule(snap.Schedule), nil
}
