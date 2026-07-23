package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// packageTestDataDir is the process-wide temp data root for this package's tests.
// All default bans.json / results.json / schedule.json paths resolve under here
// so go test never writes into the repo's data/grok-inspection/.
var packageTestDataDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "grok-inspection-test-*")
	if err != nil {
		panic(err)
	}
	packageTestDataDir = dir
	results := filepath.Join(dir, "results.json")
	bans := filepath.Join(dir, "bans.json")

	// Env drives defaultBanStateFile / storeFilePath when override is empty.
	_ = os.Setenv("GROK_INSPECTION_DATA_DIR", dir)
	setTestStorePathDefaultForTest(results)

	cfg := defaultPluginConfig()
	cfg.StateFile = bans
	cfg.PersistState = true
	currentConfig.Store(cfg)

	// package init() may already have loaded repo data/grok-inspection via
	// engine.loadFromDisk() before TestMain ran. Reset and reload from the
	// temp dir so tests never observe repo results/schedule.
	resetEngineAndStoresForTestIsolation()

	code := m.Run()

	// Drop process defaults before removing the temp tree.
	setTestStorePathDefaultForTest("")
	storeMu.Lock()
	storePathOverride = ""
	testStorePathDefault = ""
	storeMu.Unlock()
	_ = os.Unsetenv("GROK_INSPECTION_DATA_DIR")
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// restorePackageTestDataEnv re-applies TestMain isolation after a test that
// temporarily changes GROK_INSPECTION_DATA_DIR or store paths.
func restorePackageTestDataEnv() {
	if packageTestDataDir == "" {
		return
	}
	_ = os.Setenv("GROK_INSPECTION_DATA_DIR", packageTestDataDir)
	setTestStorePathDefaultForTest(filepath.Join(packageTestDataDir, "results.json"))
	cfg := loadedConfig()
	cfg.StateFile = filepath.Join(packageTestDataDir, "bans.json")
	currentConfig.Store(cfg)
}

func TestPackageDataIsolationUsesTempDir(t *testing.T) {
	if packageTestDataDir == "" {
		t.Fatal("TestMain did not set packageTestDataDir")
	}
	ban := defaultBanStateFile()
	res := storeFilePath()
	sched := scheduleFilePath()
	root := filepath.Clean(packageTestDataDir)
	for _, p := range []string{ban, res, sched} {
		if p == "" || !strings.HasPrefix(filepath.Clean(p), root+string(os.PathSeparator)) && filepath.Clean(p) != root {
			// allow exact root; usually files are under root
			if !strings.HasPrefix(filepath.Clean(p), root) {
				t.Fatalf("path %q not under package temp %q", p, packageTestDataDir)
			}
		}
		slash := filepath.ToSlash(p)
		if slash == "data/grok-inspection/bans.json" || slash == "data/grok-inspection/results.json" || slash == "data/grok-inspection/schedule.json" {
			t.Fatalf("still using repo-relative path: %s", p)
		}
	}
	cfg := loadedConfig()
	if !strings.HasPrefix(filepath.Clean(cfg.StateFile), root) {
		t.Fatalf("currentConfig.StateFile=%q not under temp", cfg.StateFile)
	}
}

func TestPackageDataIsolationWriteStaysInTemp(t *testing.T) {
	if packageTestDataDir == "" {
		t.Fatal("TestMain did not set packageTestDataDir")
	}
	cfg := defaultInspectionSchedule()
	cfg.Enabled = true
	cfg.IntervalMinutes = 4
	if err := saveInspectionScheduleSync(cfg); err != nil {
		t.Fatal(err)
	}
	activeStore.Clear()
	t.Cleanup(func() { activeStore.Clear() })
	now := time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "iso-1", Provider: "xai", ErrorCode: unauthorizedErrorCode,
		BannedAt: now, ResetAt: now.AddDate(100, 0, 0),
		ResetSource: "manual_unban", CpaSynced: false,
	})
	path := filepath.Join(packageTestDataDir, "bans.json")
	if err := activeStore.Save(path); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bans.json", "schedule.json"} {
		want := filepath.Join(packageTestDataDir, name)
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("expected %s: %v", want, err)
		}
	}
}

// resetEngineAndStoresForTestIsolation clears process globals that package init()
// may have populated from the repo data directory, then reloads from the temp path.
func resetEngineAndStoresForTestIsolation() {
	resetStoreIOForTest()
	activeStore.Clear()
	engine.mu.Lock()
	engine.results = nil
	engine.schedule = defaultInspectionSchedule()
	engine.workers = defaultWorkers
	engine.includeDisabled = false
	engine.onlyDisabled = false
	engine.total = 0
	engine.probeDone = 0
	engine.startedAt = time.Time{}
	engine.finishedAt = time.Time{}
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.stopped = false
	engine.shuttingDown = false
	engine.mu.Unlock()
	// loadFromDisk now resolves under GROK_INSPECTION_DATA_DIR (empty temp).
	engine.loadFromDisk()
}
