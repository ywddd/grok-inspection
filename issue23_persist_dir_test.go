package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginabi"
)

// Issue #23: schedule.json / results.json must live in the same directory as the
// configured ban state_file so they persist on a mounted volume and survive
// container recreation (e.g. watchtower auto-update). Previously they resolved to
// a CWD-relative data/grok-inspection dir that unmounted installs lost on upgrade.
func TestPersistFilesFollowStateFileDir(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "bans.json")

	prevOverride := getStorePathOverrideForTest()
	prevEnv, hadEnv := os.LookupEnv("GROK_INSPECTION_DATA_DIR")
	prevCfg := loadedConfig()
	t.Cleanup(func() {
		setStoreFilePathForTest(prevOverride)
		if hadEnv {
			_ = os.Setenv("GROK_INSPECTION_DATA_DIR", prevEnv)
		} else {
			_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
		}
		currentConfig.Store(prevCfg)
		restorePackageTestDataEnv()
	})

	// Force resolution through state_file: no override, no env.
	_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
	setStoreFilePathForTest("")
	storeMu.Lock()
	storePathOverride = ""
	testStorePathDefault = ""
	storeMu.Unlock()

	cfg := defaultPluginConfig()
	cfg.StateFile = stateFile
	cfg.PersistState = true
	currentConfig.Store(cfg)

	if got := storeFilePath(); got != filepath.Join(dir, "results.json") {
		t.Fatalf("results path = %q, want it beside state_file", got)
	}
	if got := scheduleFilePath(); got != filepath.Join(dir, "schedule.json") {
		t.Fatalf("schedule path = %q, want it beside state_file", got)
	}
}

// Issue #23 migration: an install upgrading from the old CWD-relative layout must
// still read its previous results.json/schedule.json once, from the legacy dir,
// when the new state_file dir has none yet.
func TestLegacyPersistFilesAreReadOnceOnUpgrade(t *testing.T) {
	// Chdir into a temp CWD so the legacy CWD-relative "data/grok-inspection"
	// path resolves under the temp tree and never touches the repo working tree.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	legacyDir := filepath.Join("data", "grok-inspection")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "results.json"), mustJSONBytes(t, persistedSnapshot{
		Version: storeVersion,
		Workers: 9,
		Results: []accountResult{},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "schedule.json"), mustJSONBytes(t, persistedInspectionSchedule{
		Enabled:         true,
		IntervalMinutes: 7,
		Workers:         5,
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "bans.json")

	prevOverride := getStorePathOverrideForTest()
	prevEnv, hadEnv := os.LookupEnv("GROK_INSPECTION_DATA_DIR")
	prevCfg := loadedConfig()
	t.Cleanup(func() {
		setStoreFilePathForTest(prevOverride)
		if hadEnv {
			_ = os.Setenv("GROK_INSPECTION_DATA_DIR", prevEnv)
		} else {
			_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
		}
		currentConfig.Store(prevCfg)
		restorePackageTestDataEnv()
	})

	_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
	setStoreFilePathForTest("")
	storeMu.Lock()
	storePathOverride = ""
	testStorePathDefault = ""
	storeMu.Unlock()

	cfg := defaultPluginConfig()
	cfg.StateFile = stateFile
	cfg.PersistState = true
	currentConfig.Store(cfg)

	snap, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatalf("loadPersistedSnapshot: %v", err)
	}
	if snap.Workers != 9 {
		t.Fatalf("legacy results not migrated: workers=%d", snap.Workers)
	}
	sched, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatalf("loadInspectionScheduleFromDisk: %v", err)
	}
	if !sched.Enabled || sched.IntervalMinutes != 7 {
		t.Fatalf("legacy schedule not migrated: %+v", sched)
	}

	// Fallback must seed the durable state_file dir so a later boot without the
	// legacy CWD path still finds the data.
	newResults := filepath.Join(dir, "results.json")
	newSchedule := filepath.Join(dir, "schedule.json")
	if _, err := os.Stat(newResults); err != nil {
		t.Fatalf("expected seeded results at %s: %v", newResults, err)
	}
	if _, err := os.Stat(newSchedule); err != nil {
		t.Fatalf("expected seeded schedule at %s: %v", newSchedule, err)
	}
	// Drop legacy files and re-load from the seeded durable paths only.
	_ = os.Remove(filepath.Join(legacyDir, "results.json"))
	_ = os.Remove(filepath.Join(legacyDir, "schedule.json"))
	snap2, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatalf("reload after legacy removal: %v", err)
	}
	if snap2.Workers != 9 {
		t.Fatalf("seeded results not durable: workers=%d", snap2.Workers)
	}
	sched2, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatalf("reload schedule after legacy removal: %v", err)
	}
	if !sched2.Enabled || sched2.IntervalMinutes != 7 {
		t.Fatalf("seeded schedule not durable: %+v", sched2)
	}
}

// Issue #23: results and schedule fall back independently. A new-path results.json
// must not be displaced by legacy results, while a missing new-path schedule.json
// can still migrate from the legacy schedule-only file.
func TestLegacyScheduleFallbackWithNewResults(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	legacyDir := filepath.Join("data", "grok-inspection")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Stale legacy results must not win when the new path already has results.
	if err := os.WriteFile(filepath.Join(legacyDir, "results.json"), mustJSONBytes(t, persistedSnapshot{
		Version: storeVersion,
		Workers: 1,
		Results: []accountResult{{AuthIndex: "legacy", Name: "legacy.json"}},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	// schedule.json only exists in the legacy dir.
	if err := os.WriteFile(filepath.Join(legacyDir, "schedule.json"), mustJSONBytes(t, persistedInspectionSchedule{
		Enabled:         true,
		IntervalMinutes: 13,
		Workers:         3,
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "bans.json")
	if err := os.WriteFile(filepath.Join(dir, "results.json"), mustJSONBytes(t, persistedSnapshot{
		Version: storeVersion,
		Workers: 10,
		Results: []accountResult{{AuthIndex: "new", Name: "new.json"}},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	// Intentionally no schedule.json beside the new state_file.

	prevOverride := getStorePathOverrideForTest()
	prevEnv, hadEnv := os.LookupEnv("GROK_INSPECTION_DATA_DIR")
	prevCfg := loadedConfig()
	t.Cleanup(func() {
		setStoreFilePathForTest(prevOverride)
		if hadEnv {
			_ = os.Setenv("GROK_INSPECTION_DATA_DIR", prevEnv)
		} else {
			_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
		}
		currentConfig.Store(prevCfg)
		restorePackageTestDataEnv()
	})

	_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
	setStoreFilePathForTest("")
	storeMu.Lock()
	storePathOverride = ""
	testStorePathDefault = ""
	storeMu.Unlock()

	cfg := defaultPluginConfig()
	cfg.StateFile = stateFile
	cfg.PersistState = true
	currentConfig.Store(cfg)

	snap, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatalf("loadPersistedSnapshot: %v", err)
	}
	if snap.Workers != 10 || len(snap.Results) != 1 || snap.Results[0].AuthIndex != "new" {
		t.Fatalf("new results must win over legacy: %+v", snap)
	}
	sched, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatalf("loadInspectionScheduleFromDisk: %v", err)
	}
	if !sched.Enabled || sched.IntervalMinutes != 13 || sched.Workers != 3 {
		t.Fatalf("legacy schedule-only file should migrate: %+v", sched)
	}
	// Schedule seeds independently; results at new path stay untouched.
	if _, err := os.Stat(filepath.Join(dir, "schedule.json")); err != nil {
		t.Fatalf("expected seeded schedule.json beside state_file: %v", err)
	}
	rawResults, err := os.ReadFile(filepath.Join(dir, "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	var diskSnap persistedSnapshot
	if err := json.Unmarshal(rawResults, &diskSnap); err != nil {
		t.Fatal(err)
	}
	if diskSnap.Workers != 10 || len(diskSnap.Results) != 1 || diskSnap.Results[0].AuthIndex != "new" {
		t.Fatalf("schedule seed must not touch durable results: %+v", diskSnap)
	}
}

// Issue #23: valid files at the new state_file path must win over leftover legacy
// CWD-relative files so upgrades never re-import stale history.
func TestNewPersistPathWinsOverLegacy(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	legacyDir := filepath.Join("data", "grok-inspection")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "results.json"), mustJSONBytes(t, persistedSnapshot{
		Version: storeVersion,
		Workers: 1,
		Results: []accountResult{},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "schedule.json"), mustJSONBytes(t, persistedInspectionSchedule{
		Enabled:         true,
		IntervalMinutes: 3,
		Workers:         1,
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "bans.json")
	if err := os.WriteFile(filepath.Join(dir, "results.json"), mustJSONBytes(t, persistedSnapshot{
		Version: storeVersion,
		Workers: 12,
		Results: []accountResult{{AuthIndex: "new", Name: "new.json"}},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schedule.json"), mustJSONBytes(t, persistedInspectionSchedule{
		Enabled:         false,
		IntervalMinutes: 15,
		Workers:         4,
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	prevOverride := getStorePathOverrideForTest()
	prevEnv, hadEnv := os.LookupEnv("GROK_INSPECTION_DATA_DIR")
	prevCfg := loadedConfig()
	t.Cleanup(func() {
		setStoreFilePathForTest(prevOverride)
		if hadEnv {
			_ = os.Setenv("GROK_INSPECTION_DATA_DIR", prevEnv)
		} else {
			_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
		}
		currentConfig.Store(prevCfg)
		restorePackageTestDataEnv()
	})

	_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
	setStoreFilePathForTest("")
	storeMu.Lock()
	storePathOverride = ""
	testStorePathDefault = ""
	storeMu.Unlock()

	cfg := defaultPluginConfig()
	cfg.StateFile = stateFile
	cfg.PersistState = true
	currentConfig.Store(cfg)

	snap, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatalf("loadPersistedSnapshot: %v", err)
	}
	if snap.Workers != 12 || len(snap.Results) != 1 || snap.Results[0].AuthIndex != "new" {
		t.Fatalf("new path should win over legacy results: %+v", snap)
	}
	sched, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatalf("loadInspectionScheduleFromDisk: %v", err)
	}
	if sched.Enabled || sched.IntervalMinutes != 15 {
		t.Fatalf("new path should win over legacy schedule: %+v", sched)
	}

	// Existing durable files must not be rewritten by legacy fallback.
	rawResults, err := os.ReadFile(filepath.Join(dir, "results.json"))
	if err != nil {
		t.Fatal(err)
	}
	var diskSnap persistedSnapshot
	if err := json.Unmarshal(rawResults, &diskSnap); err != nil {
		t.Fatal(err)
	}
	if diskSnap.Workers != 12 || len(diskSnap.Results) != 1 || diskSnap.Results[0].AuthIndex != "new" {
		t.Fatalf("legacy fallback overwrote durable results: %+v", diskSnap)
	}
	rawSched, err := os.ReadFile(filepath.Join(dir, "schedule.json"))
	if err != nil {
		t.Fatal(err)
	}
	var diskSched persistedInspectionSchedule
	if err := json.Unmarshal(rawSched, &diskSched); err != nil {
		t.Fatal(err)
	}
	if diskSched.Enabled || diskSched.IntervalMinutes != 15 {
		t.Fatalf("legacy fallback overwrote durable schedule: %+v", diskSched)
	}
}

// Issue #23: package init() calls loadFromDisk before CPA PluginRegister applies the
// real state_file. register must re-read results/schedule from the configured dir
// before the schedule loop starts, so mounted volume history survives upgrades and
// init-time default-path data cannot stick as the live snapshot.
func TestRegisterReloadsPersistedInspectionFromStateFileDir(t *testing.T) {
	// Seed a boot/default path that init() would have loaded first.
	bootDir := t.TempDir()
	bootState := filepath.Join(bootDir, "bans.json")
	if err := os.WriteFile(filepath.Join(bootDir, "results.json"), mustJSONBytes(t, persistedSnapshot{
		Version: storeVersion,
		Workers: 2,
		Results: []accountResult{
			{AuthIndex: "boot-only", Name: "boot.json", Classification: "other", Action: "keep"},
		},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bootDir, "schedule.json"), mustJSONBytes(t, persistedInspectionSchedule{
		Enabled:         false,
		IntervalMinutes: 3,
		Workers:         2,
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	// Real mounted state_file directory delivered later via plugin.register.
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "bans.json")
	if err := os.WriteFile(filepath.Join(dir, "results.json"), mustJSONBytes(t, persistedSnapshot{
		Version: storeVersion,
		Workers: 8,
		Results: []accountResult{
			{AuthIndex: "persist-1", Name: "persist-1.json", Classification: "healthy", Action: "keep"},
		},
	}), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schedule.json"), mustJSONBytes(t, persistedInspectionSchedule{
		Enabled:         false, // keep the process-global schedule loop idle
		IntervalMinutes: 21,
		Workers:         6,
		NextRunAt:       time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	prevOverride := getStorePathOverrideForTest()
	prevEnv, hadEnv := os.LookupEnv("GROK_INSPECTION_DATA_DIR")
	prevCfg := loadedConfig()
	pluginLifecycleMu.Lock()
	prevRegistered := pluginRegistered
	pluginRegistered = false
	pluginLifecycleMu.Unlock()

	engine.mu.Lock()
	prevResults := append([]accountResult(nil), engine.results...)
	prevSchedule := engine.schedule
	prevWorkers := engine.workers
	engine.results = nil
	engine.schedule = defaultInspectionSchedule()
	engine.workers = defaultWorkers
	engine.mu.Unlock()

	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = prevResults
		engine.schedule = prevSchedule
		engine.workers = prevWorkers
		engine.mu.Unlock()
		setPluginRegisteredForTest(prevRegistered)
		setStoreFilePathForTest(prevOverride)
		if hadEnv {
			_ = os.Setenv("GROK_INSPECTION_DATA_DIR", prevEnv)
		} else {
			_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
		}
		currentConfig.Store(prevCfg)
		restorePackageTestDataEnv()
	})

	_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
	setStoreFilePathForTest("")
	storeMu.Lock()
	storePathOverride = ""
	testStorePathDefault = ""
	storeMu.Unlock()
	resetStoreIOForTest()

	// Simulate init-before-register against the boot/default path.
	bootCfg := defaultPluginConfig()
	bootCfg.StateFile = bootState
	bootCfg.PersistState = true
	currentConfig.Store(bootCfg)
	engine.loadFromDisk()
	bootSnap := engine.snapshot(true)
	if bootSnap.Workers != 2 || len(bootSnap.Results) != 1 || bootSnap.Results[0].AuthIndex != "boot-only" {
		t.Fatalf("pre-register boot load failed: workers=%d results=%+v", bootSnap.Workers, bootSnap.Results)
	}

	raw, err := json.Marshal(lifecycleRequest{
		SchemaVersion: pluginabi.SchemaVersion,
		ConfigYAML:    []byte(lifecycleYAML(true, stateFile, 24)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registerPlugin(raw); err != nil {
		t.Fatalf("registerPlugin: %v", err)
	}

	snap := engine.snapshot(true)
	if snap.Workers != 8 {
		t.Fatalf("workers after register reload = %d, want 8 (not boot workers=%d)", snap.Workers, bootSnap.Workers)
	}
	if len(snap.Results) != 1 || snap.Results[0].AuthIndex != "persist-1" {
		t.Fatalf("results after register reload = %+v, boot data must not stick", snap.Results)
	}
	if snap.Schedule.IntervalMinutes != 21 || snap.Schedule.Workers != 6 {
		t.Fatalf("schedule after register reload = %+v", snap.Schedule)
	}
	if !pluginRegistered {
		t.Fatal("pluginRegistered should be true after first register")
	}
}

func mustJSONBytes(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
