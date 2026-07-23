package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginabi"
)

func lifecycleRequestJSON(t *testing.T, yaml string) []byte {
	t.Helper()
	raw, err := json.Marshal(lifecycleRequest{
		SchemaVersion: pluginabi.SchemaVersion,
		ConfigYAML:    []byte(yaml),
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func setPluginRegisteredForTest(registered bool) {
	pluginLifecycleMu.Lock()
	pluginRegistered = registered
	pluginLifecycleMu.Unlock()
}

func isolatePluginLifecycle(t *testing.T, cfg pluginConfig, registered bool) {
	t.Helper()
	isolateActiveStore(t)
	stopBanPersistWorkerForTest()

	oldCfg := loadedConfig()
	pluginLifecycleMu.Lock()
	oldRegistered := pluginRegistered
	pluginRegistered = registered
	pluginLifecycleMu.Unlock()
	currentConfig.Store(cfg)

	oldSave := banStoreSaveFn
	banStoreSaveFn = func(path string) error {
		return activeStore.Save(path)
	}
	t.Cleanup(func() {
		stopBanPersistWorkerForTest()
		banStoreSaveFn = oldSave
		currentConfig.Store(oldCfg)
		setPluginRegisteredForTest(oldRegistered)
	})
}

func lifecycleYAML(persist bool, path string, fallback int) string {
	return "autoban_enabled: true\n" +
		"fallback_hours: " + itoa(fallback) + "\n" +
		"persist_state: " + map[bool]string{true: "true", false: "false"}[persist] + "\n" +
		"state_file: " + path + "\n"
}

func reconfigureForTest(t *testing.T, persist bool, path string, fallback int) {
	t.Helper()
	_, err := handleMethod(
		pluginabi.MethodPluginReconfigure,
		lifecycleRequestJSON(t, lifecycleYAML(persist, path, fallback)),
	)
	if err != nil {
		t.Fatalf("plugin.reconfigure: %v", err)
	}
}

func testBanEntry(id string, reset time.Time) banEntry {
	return banEntry{
		AuthID:      id,
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    reset.Add(-time.Hour),
		ResetAt:     reset,
		ResetSource: "local_plus_fallback",
		CpaSynced:   false,
	}
}

func TestReconfigureSamePathDoesNotLoadStaleDiskOverLiveBans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bans.json")
	now := time.Now()

	disk := newBanStore()
	disk.Set(testBanEntry("stale-disk", now.Add(time.Hour)))
	if err := disk.Save(path); err != nil {
		t.Fatal(err)
	}

	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = path
	isolatePluginLifecycle(t, cfg, true)
	activeStore.Set(testBanEntry("fresh-memory", now.Add(2*time.Hour)))

	reconfigureForTest(t, true, path, 48)

	if _, ok := activeStore.Get("fresh-memory"); !ok {
		t.Fatal("reconfigure loaded stale disk and dropped fresh in-memory ban")
	}
	if _, ok := activeStore.Get("stale-disk"); ok {
		t.Fatal("reconfigure must not import stale disk state")
	}
	if got := loadedConfig(); got.StateFile != path || got.FallbackHours != 48 {
		t.Fatalf("config not updated: %+v", got)
	}
}

func TestReconfigureNewPathMigratesCompleteLiveState(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old", "bans.json")
	newPath := filepath.Join(dir, "new", "bans.json")
	now := time.Now()

	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = oldPath
	isolatePluginLifecycle(t, cfg, true)
	activeStore.Set(testBanEntry("live-a", now.Add(time.Hour)))
	activeStore.Set(testBanEntry("live-b", now.Add(2*time.Hour)))

	reconfigureForTest(t, true, newPath, 24)

	reloaded := newBanStore()
	if err := reloaded.Load(newPath, now); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"live-a", "live-b"} {
		if _, ok := reloaded.Get(id); !ok {
			t.Fatalf("new state_file missing %s", id)
		}
	}
	if got := loadedConfig(); got.StateFile != newPath || !got.PersistState {
		t.Fatalf("new config not committed: %+v", got)
	}
}

func TestReconfigureDisablesPersistenceWithoutClearingLiveState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bans.json")
	now := time.Now()

	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = path
	isolatePluginLifecycle(t, cfg, true)
	activeStore.Set(testBanEntry("keep-memory", now.Add(time.Hour)))

	reconfigureForTest(t, false, path, 24)

	if _, ok := activeStore.Get("keep-memory"); !ok {
		t.Fatal("disabling persistence cleared the live ban pool")
	}
	if loadedConfig().PersistState {
		t.Fatal("persist_state remained enabled")
	}
}

func TestReconfigureBeforeFirstRegisterDoesNotLoadDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bans.json")
	now := time.Now()

	disk := newBanStore()
	disk.Set(testBanEntry("disk-only", now.Add(time.Hour)))
	if err := disk.Save(path); err != nil {
		t.Fatal(err)
	}

	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = filepath.Join(dir, "before.json")
	isolatePluginLifecycle(t, cfg, false)
	activeStore.Set(testBanEntry("live-before-register", now.Add(2*time.Hour)))

	reconfigureForTest(t, true, path, 24)

	if _, ok := activeStore.Get("live-before-register"); !ok {
		t.Fatal("reconfigure before register replaced live state")
	}
	if _, ok := activeStore.Get("disk-only"); ok {
		t.Fatal("only plugin.register may load ban state from disk")
	}
	pluginLifecycleMu.Lock()
	registered := pluginRegistered
	pluginLifecycleMu.Unlock()
	if registered {
		t.Fatal("reconfigure must not consume the first-register lifecycle")
	}
}

func TestReconfigurePathSwitchCapturesConcurrentUsageMutation(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "new.json")
	now := time.Now()

	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = oldPath
	isolatePluginLifecycle(t, cfg, true)
	activeStore.Set(testBanEntry("before-switch", now.Add(time.Hour)))

	firstSaveDone := make(chan struct{})
	releaseFirstSave := make(chan struct{})
	var once, releaseOnce sync.Once
	releaseMigration := func() {
		releaseOnce.Do(func() {
			close(releaseFirstSave)
		})
	}
	defer releaseMigration()
	banStoreSaveFn = func(path string) error {
		err := activeStore.Save(path)
		if path == newPath {
			once.Do(func() {
				close(firstSaveDone)
				<-releaseFirstSave
			})
		}
		return err
	}

	request := lifecycleRequestJSON(t, lifecycleYAML(true, newPath, 24))
	errCh := make(chan error, 1)
	go func() {
		_, err := handleMethod(pluginabi.MethodPluginReconfigure, request)
		errCh <- err
	}()

	select {
	case <-firstSaveDone:
	case <-time.After(3 * time.Second):
		t.Fatal("new-path migration save did not start")
	}
	if got := loadedConfig().StateFile; got != oldPath {
		t.Fatalf("new config published before migration completed: got %q want %q", got, oldPath)
	}
	activeStore.Set(testBanEntry("during-switch", now.Add(2*time.Hour)))
	markBanStoreDirty()
	releaseMigration()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reconfigure hung")
	}

	if err := flushBanPersistWorker(); err != nil {
		t.Fatal(err)
	}
	reloaded := newBanStore()
	if err := reloaded.Load(newPath, now); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"before-switch", "during-switch"} {
		if _, ok := reloaded.Get(id); !ok {
			t.Fatalf("new state_file lost concurrent mutation %s", id)
		}
	}
}

func TestReconfigureMigrationFailureKeepsPreviousConfig(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	newPath := filepath.Join(dir, "new.json")
	now := time.Now()

	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = oldPath
	isolatePluginLifecycle(t, cfg, true)
	activeStore.Set(testBanEntry("live-on-failure", now.Add(time.Hour)))

	saveErr := errors.New("injected migration failure")
	banStoreSaveFn = func(path string) error {
		if path == newPath {
			return saveErr
		}
		return activeStore.Save(path)
	}

	_, err := handleMethod(
		pluginabi.MethodPluginReconfigure,
		lifecycleRequestJSON(t, lifecycleYAML(true, newPath, 48)),
	)
	if !errors.Is(err, saveErr) {
		t.Fatalf("plugin.reconfigure error = %v, want migration failure", err)
	}
	if got := loadedConfig(); got.StateFile != oldPath || got.FallbackHours != cfg.FallbackHours {
		t.Fatalf("failed migration committed new config: %+v", got)
	}
	if _, ok := activeStore.Get("live-on-failure"); !ok {
		t.Fatal("failed migration changed the live ban pool")
	}
}

func TestConcurrentReconfigureCallsSerializeWithoutRollback(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.json")
	pathA := filepath.Join(dir, "a.json")
	pathB := filepath.Join(dir, "b.json")
	now := time.Now()

	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = oldPath
	isolatePluginLifecycle(t, cfg, true)
	activeStore.Set(testBanEntry("serial-live", now.Add(time.Hour)))

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var once sync.Once
	banStoreSaveFn = func(path string) error {
		if path == pathA {
			once.Do(func() {
				close(firstEntered)
				<-releaseFirst
			})
		}
		return activeStore.Save(path)
	}

	requestA := lifecycleRequestJSON(t, lifecycleYAML(true, pathA, 36))
	errA := make(chan error, 1)
	go func() {
		_, err := handleMethod(pluginabi.MethodPluginReconfigure, requestA)
		errA <- err
	}()
	select {
	case <-firstEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("first reconfigure did not reach migration")
	}

	requestB := lifecycleRequestJSON(t, lifecycleYAML(true, pathB, 72))
	errB := make(chan error, 1)
	go func() {
		_, err := handleMethod(pluginabi.MethodPluginReconfigure, requestB)
		errB <- err
	}()
	select {
	case err := <-errB:
		t.Fatalf("second reconfigure bypassed lifecycle serialization: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-errA; err != nil {
		t.Fatal(err)
	}
	if err := <-errB; err != nil {
		t.Fatal(err)
	}

	got := loadedConfig()
	if got.StateFile != pathB || got.FallbackHours != 72 {
		t.Fatalf("later serialized reconfigure rolled back: %+v", got)
	}
	reloaded := newBanStore()
	if err := reloaded.Load(pathB, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get("serial-live"); !ok {
		t.Fatal("final state_file missing live ban")
	}
}
