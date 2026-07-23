package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

// ---------- 1) schedule transaction serialization ----------

func TestScheduleConcurrentUserUpdatesAreTransactional(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	resetStoreIOForTest()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		resetStoreIOForTest()
		scheduleIOTestHook = nil
	})
	engine.mu.Lock()
	engine.schedule = defaultInspectionSchedule()
	engine.mu.Unlock()

	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			enabled := i%2 == 0
			interval := 1 + i
			_, err := updateInspectionSchedule(inspectionScheduleUpdate{
				Enabled:         &enabled,
				IntervalMinutes: &interval,
			})
			if err != nil {
				t.Errorf("update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	mem := inspectionScheduleSnapshot()
	disk, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if mem.Enabled != disk.Enabled || mem.IntervalMinutes != disk.IntervalMinutes {
		t.Fatalf("memory/disk diverge: mem=%+v disk=%+v", mem, disk)
	}
	// Full transaction: enabled and interval must be a coherent pair from one writer.
	// Even i => enabled=true, interval=1+i (odd). Odd i => enabled=false, interval even.
	if mem.Enabled && mem.IntervalMinutes%2 == 0 {
		t.Fatalf("incoherent enabled=true with even interval: %+v", mem)
	}
	if !mem.Enabled && mem.IntervalMinutes%2 != 0 {
		t.Fatalf("incoherent enabled=false with odd interval: %+v", mem)
	}
}

func TestScheduleRuntimeStatusDoesNotClobberConcurrentUserSettings(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	resetStoreIOForTest()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		resetStoreIOForTest()
		scheduleIOTestHook = nil
	})
	engine.mu.Lock()
	engine.schedule = defaultInspectionSchedule()
	engine.schedule.Enabled = false
	engine.schedule.IntervalMinutes = 3
	engine.mu.Unlock()

	// Force user update to hold the txn (via save hook) while runtime is attempted.
	userEntered := make(chan struct{})
	releaseUser := make(chan struct{})
	var once sync.Once
	scheduleIOTestHook = func() {
		once.Do(func() {
			close(userEntered)
			<-releaseUser
		})
	}

	doneUser := make(chan error, 1)
	go func() {
		en := true
		iv := 17
		_, err := updateInspectionSchedule(inspectionScheduleUpdate{Enabled: &en, IntervalMinutes: &iv})
		doneUser <- err
	}()
	select {
	case <-userEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("user update never reached save")
	}

	// Runtime status must wait for user txn; start it while user is mid-save.
	doneRT := make(chan struct{})
	go func() {
		setInspectionScheduleRuntimeStatus("ok", "", time.Date(2026, 7, 23, 1, 0, 0, 0, time.UTC), scheduleRunStats{matched403: 1})
		close(doneRT)
	}()

	// Give runtime a chance to race if unlocked (must still be blocked).
	time.Sleep(30 * time.Millisecond)
	select {
	case <-doneRT:
		t.Fatal("runtime status completed while user txn still open")
	default:
	}

	close(releaseUser)
	if err := <-doneUser; err != nil {
		t.Fatal(err)
	}
	select {
	case <-doneRT:
	case <-time.After(3 * time.Second):
		t.Fatal("runtime status hung")
	}

	got := inspectionScheduleSnapshot()
	if !got.Enabled || got.IntervalMinutes != 17 {
		t.Fatalf("user settings lost: %+v", got)
	}
	if got.LastStatus != "ok" || got.LastMatched403 != 1 {
		t.Fatalf("runtime fields missing: %+v", got)
	}
	disk, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if !disk.Enabled || disk.IntervalMinutes != 17 || disk.LastStatus != "ok" {
		t.Fatalf("disk incoherent: %+v", disk)
	}
}

// ---------- 2+3) enable CAS result + no engine.mu during re-disable HTTP ----------

func TestEnableCASUpdatesResultDisabledAfterRedisable(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	redisableEntered := make(chan struct{})
	releaseRedisable := make(chan struct{})
	var redisableOK atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "status") {
			var body struct {
				Name     string `json:"name"`
				Disabled bool   `json:"disabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body.Disabled {
				select {
				case <-enableEntered:
				default:
					close(enableEntered)
				}
				<-releaseEnable
			} else {
				redisableOK.Store(true)
				select {
				case <-redisableEntered:
				default:
					close(redisableEntered)
				}
				<-releaseRedisable
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "cas2", Name: "cas2.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "cas2.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})

	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex: "cas2", Name: "cas2.json", FileName: "cas2.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	errCh := make(chan error, 1)
	go func() { errCh <- setAuthDisabled("cas2.json", false, "test-pass", nil, false) }()
	select {
	case <-enableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("enable not entered")
	}
	// Concurrent ban during enable.
	activeStore.Set(banEntry{
		AuthID: "cas2.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(30 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: false,
	})
	close(releaseEnable)

	// While re-disable is blocked, engine.snapshot must still take the lock quickly.
	select {
	case <-redisableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("re-disable not entered")
	}
	snapDone := make(chan struct{})
	go func() {
		_ = engine.snapshot(false)
		close(snapDone)
	}()
	select {
	case <-snapDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("engine.snapshot blocked while re-disable HTTP in flight (CAS held engine.mu)")
	}

	close(releaseRedisable)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("enable: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("enable hung")
	}
	if !redisableOK.Load() {
		t.Fatal("expected successful re-disable")
	}
	engine.mu.Lock()
	var row *accountResult
	for i := range engine.results {
		if engine.results[i].FileName == "cas2.json" {
			row = &engine.results[i]
			break
		}
	}
	engine.mu.Unlock()
	if row == nil {
		t.Fatal("result row missing")
	}
	if !row.Disabled {
		t.Fatalf("result must stay disabled after CAS re-disable: %+v", row)
	}
	if _, ok := activeStore.Get("cas2.json"); !ok {
		t.Fatal("local ban must remain")
	}
}

// ---------- 4+5) ban persist dirty restore + final flush ----------

func TestBanPersistRetainsDirtyOnSaveFailureAndFinalFlush(t *testing.T) {
	isolateActiveStore(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(stopBanPersistWorkerForTest)

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = state
	currentConfig.Store(cfg)
	t.Cleanup(func() {
		currentConfig.Store(oldCfg)
		banStoreSaveFn = func(path string) error { return activeStore.Save(path) }
	})

	// --- Part 1: transient failures then worker retry succeeds ---
	var calls atomic.Int32
	failUntil := int32(3)
	banStoreSaveFn = func(path string) error {
		n := calls.Add(1)
		if n <= failUntil {
			return errors.New("transient save failure")
		}
		return activeStore.Save(path)
	}
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "pf1", Provider: "xai", ErrorCode: unauthorizedErrorCode,
		BannedAt: now, ResetAt: now.AddDate(100, 0, 0),
		ResetSource: "manual_unban", CpaSynced: false,
	})
	markBanStoreDirty()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(state); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatalf("expected eventual successful save after retries: calls=%d err=%v", calls.Load(), err)
	}
	if calls.Load() <= failUntil {
		t.Fatalf("expected retries beyond failures: calls=%d", calls.Load())
	}

	// --- Part 2: deterministic final-flush-only success after stop ---
	// Worker save always fails after first entry; beforeBanPersistFinalFlush flips
	// allowSuccess only after worker Wait, so only final flush can succeed.
	stopBanPersistWorkerForTest()
	t.Cleanup(func() { beforeBanPersistFinalFlush = nil })
	activeStore.Set(banEntry{
		AuthID: "pf2", Provider: "xai", ErrorCode: unauthorizedErrorCode,
		BannedAt: now, ResetAt: now.AddDate(100, 0, 0),
		ResetSource: "manual_unban", CpaSynced: false,
	})
	firstSaveEntered := make(chan struct{})
	var allowSuccess atomic.Bool
	var workerCalls atomic.Int32
	var finalCalls atomic.Int32
	calls.Store(0)
	banStoreSaveFn = func(path string) error {
		n := calls.Add(1)
		if !allowSuccess.Load() {
			workerCalls.Add(1)
			if n == 1 {
				close(firstSaveEntered)
			}
			return errors.New("worker save blocked")
		}
		finalCalls.Add(1)
		return activeStore.Save(path)
	}
	beforeBanPersistFinalFlush = func() {
		// Runs after worker exited, before final Save — prove this path is required.
		if workerCalls.Load() < 1 {
			t.Errorf("expected worker save attempts before final flush")
		}
		allowSuccess.Store(true)
	}
	markBanStoreDirty()
	select {
	case <-firstSaveEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker never attempted save")
	}
	stopBanPersistWorker()
	if finalCalls.Load() < 1 {
		t.Fatalf("final flush never succeeded; calls=%d worker=%d final=%d", calls.Load(), workerCalls.Load(), finalCalls.Load())
	}
	loaded := newBanStore()
	if err := loaded.Load(state, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("pf2"); !ok {
		t.Fatalf("final flush missing pf2; calls=%d", calls.Load())
	}
	beforeBanPersistFinalFlush = nil
	stopBanPersistWorkerForTest()
}

// ---------- 9) CAS sync-state changes persist even when removed==0 ----------

func TestCASSyncStateMarkedDirtyWhenOnlyRedisable(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(stopBanPersistWorkerForTest)

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = state
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body struct {
				Disabled bool `json:"disabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body.Disabled {
				select {
				case <-enableEntered:
				default:
					close(enableEntered)
				}
				<-releaseEnable
			}
			// re-disable succeeds
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "cs1", Name: "cs1.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "cs1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	// findAuthFile prefers engine.results (avoids real host list).
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex: "cs1", Name: "cs1.json", FileName: "cs1.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	// persist=false: memory/sync state updates only; explicit final save for disk assert.
	errCh := make(chan error, 1)
	go func() { errCh <- setAuthDisabled("cs1.json", false, "test-pass", nil, false) }()
	select {
	case <-enableEntered:
	case err := <-errCh:
		t.Fatalf("enable finished early: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("enable never entered management")
	}
	activeStore.Set(banEntry{
		AuthID: "cs1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(20 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: false, CpaSyncError: "stale",
	})
	close(releaseEnable)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("enable hung after release")
	}
	if err := saveActiveStoreErr(); err != nil {
		t.Fatal(err)
	}
	loaded := newBanStore()
	if err := loaded.Load(state, now); err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get("cs1.json")
	if !ok {
		t.Fatal("ban missing on disk")
	}
	if !got.CpaSynced {
		t.Fatalf("re-disable success must persist CpaSynced=true: %#v", got)
	}
	if got.CpaSyncError != "" {
		t.Fatalf("CpaSyncError should clear: %#v", got)
	}
}

// ---------- 10) restore dirty on sync-state-only mutations ----------

func TestRestoreUnsyncedDisableFailurePersistsSyncError(t *testing.T) {
	isolateActiveStore(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(stopBanPersistWorkerForTest)

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = state
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "disable boom", http.StatusBadGateway)
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "ru1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: false,
	})
	_, _ = restoreExpiredBans(activeStore, now)
	if err := flushBanPersistWorker(); err != nil {
		t.Fatal(err)
	}
	loaded := newBanStore()
	if err := loaded.Load(state, now); err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get("ru1")
	if !ok {
		t.Fatal("ban missing")
	}
	if got.CpaSynced || strings.TrimSpace(got.CpaSyncError) == "" {
		t.Fatalf("expected persisted sync error: %#v", got)
	}
}

func TestRestoreKeptNewerRedisablePersistsSyncState(t *testing.T) {
	isolateActiveStore(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(stopBanPersistWorkerForTest)

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = state
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	var enableOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Disabled {
			enableOnce.Do(func() { close(enableEntered) })
			<-releaseEnable
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		// re-disable fails
		http.Error(w, "redisable boom", http.StatusBadGateway)
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "rk1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-2 * time.Hour), ResetAt: now.Add(-time.Minute),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	old, _ := activeStore.Get("rk1")

	done := make(chan struct{})
	go func() {
		_, _ = restoreExpiredBans(activeStore, now)
		close(done)
	}()
	select {
	case <-enableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("enable not entered")
	}
	activeStore.Set(banEntry{
		AuthID: "rk1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour),
		ResetSource: "header_absolute", CpaSynced: false,
	})
	close(releaseEnable)
	<-done
	if err := flushBanPersistWorker(); err != nil {
		t.Fatal(err)
	}
	loaded := newBanStore()
	if err := loaded.Load(state, now); err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get("rk1")
	if !ok {
		t.Fatal("newer ban missing on disk")
	}
	if got.Revision <= old.Revision {
		t.Fatalf("rev not newer: old=%d got=%d", old.Revision, got.Revision)
	}
	if got.CpaSynced || strings.TrimSpace(got.CpaSyncError) == "" {
		t.Fatalf("re-disable fail must persist CpaSyncError: %#v", got)
	}
}

// ---------- 11) unban re-disable failure is a failure; success not counted as unban ----------

func TestUnbanOneRedisableFailureIsErrorAndPersistsSyncError(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(stopBanPersistWorkerForTest)

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = state
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	var enableOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Disabled {
			enableOnce.Do(func() { close(enableEntered) })
			<-releaseEnable
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.Error(w, "redisable failed", http.StatusBadGateway)
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "u1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	old, _ := activeStore.Get("u1")

	// Ensure claimUnbanSlot is not blocked by leftover engine flags.
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()

	errCh := make(chan error, 1)
	var enabled, removed bool
	go func() {
		var e error
		enabled, removed, e = unbanOneAccount("u1", "test-pass")
		errCh <- e
	}()
	select {
	case <-enableEntered:
	case err := <-errCh:
		t.Fatalf("unban finished before enable entered: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("unban never entered enable PATCH")
	}
	activeStore.Set(banEntry{
		AuthID: "u1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(30 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: true,
	})
	close(releaseEnable)
	var err error
	select {
	case err = <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("unban hung after release")
	}
	if err == nil {
		t.Fatal("expected re-disable failure to surface as unban error")
	}
	if enabled || removed {
		t.Fatalf("must not report unban success: enabled=%v removed=%v", enabled, removed)
	}
	st := unbanJobStatus()
	if st["failed"].(int) < 1 {
		t.Fatalf("job failed count: %#v", st)
	}
	got, ok := activeStore.Get("u1")
	if !ok || got.Revision <= old.Revision {
		t.Fatalf("newer ban must remain: ok=%v %#v", ok, got)
	}
	if got.CpaSynced || strings.TrimSpace(got.CpaSyncError) == "" {
		t.Fatalf("CpaSyncError required: %#v", got)
	}
}

func TestUnbanBatchKeptNewerNotCountedAsEnabledSuccess(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	var redisable atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Disabled {
			select {
			case <-enableEntered:
			default:
				close(enableEntered)
			}
			<-releaseEnable
		} else {
			redisable.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "ub1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	old, _ := activeStore.Get("ub1")
	if err := startUnbanJob([]string{"ub1"}, "", "test-pass"); err != nil {
		t.Fatal(err)
	}
	<-enableEntered
	activeStore.Set(banEntry{
		AuthID: "ub1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(15 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: true,
	})
	close(releaseEnable)
	unbanJob.wg.Wait()
	st := unbanJobStatus()
	if st["enabled"].(int) != 0 {
		t.Fatalf("kept-newer must not count as unban enabled success: %#v", st)
	}
	if st["missing"].(int) != 0 {
		t.Fatalf("kept-newer must not count as missing: %#v", st)
	}
	if st["failed"].(int) != 1 {
		t.Fatalf("kept-newer conflict must count as failed: %#v", st)
	}
	if st["done"].(int) != st["enabled"].(int)+st["missing"].(int)+st["failed"].(int) {
		t.Fatalf("done conservation broken: %#v", st)
	}
	got, ok := activeStore.Get("ub1")
	if !ok || got.Revision <= old.Revision {
		t.Fatalf("newer ban missing: ok=%v %#v", ok, got)
	}
	if redisable.Load() < 1 {
		t.Fatal("expected re-disable")
	}
}

// ---------- A) atomic DeleteIfOrCurrent ----------

func TestDeleteIfOrCurrentAtomic(t *testing.T) {
	store := newBanStore()
	store.Set(banEntry{AuthID: "a", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Unix(1, 0), ResetAt: time.Unix(100, 0), ResetSource: "local_plus_fallback"})
	e1, _ := store.Get("a")
	deleted, cur, present := store.DeleteIfOrCurrent("a", e1.Revision)
	if !deleted || present {
		t.Fatalf("match should delete: deleted=%v present=%v", deleted, present)
	}
	store.Set(banEntry{AuthID: "a", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Unix(2, 0), ResetAt: time.Unix(200, 0), ResetSource: "local_plus_fallback"})
	old, _ := store.Get("a")
	store.Set(banEntry{AuthID: "a", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Unix(3, 0), ResetAt: time.Unix(50, 0), ResetSource: "header_absolute"})
	newer, _ := store.Get("a")
	if newer.Revision <= old.Revision {
		t.Fatal("revision should advance")
	}
	deleted, cur, present = store.DeleteIfOrCurrent("a", old.Revision)
	if deleted || !present {
		t.Fatalf("stale rev must not delete: deleted=%v present=%v", deleted, present)
	}
	if cur.Revision != newer.Revision {
		t.Fatalf("current rev=%d want %d", cur.Revision, newer.Revision)
	}
	// Missing id
	deleted, _, present = store.DeleteIfOrCurrent("nope", 1)
	if deleted || present {
		t.Fatal("missing should be absent")
	}
}

func TestCASDeleteIfMissTriggersRedisable(t *testing.T) {
	// End-to-end: enable snapshots rev1; concurrent Set advances rev; CAS must re-disable.
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	var redisable atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Disabled {
			redisable.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "atom1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	pre, _ := activeStore.Get("atom1.json")
	// Simulate race: newer ban already present before CAS loop (as if landed after All snapshot of pre).
	// clearBansMatchingTargetCAS uses pre revisions with DeleteIfOrCurrent.
	activeStore.Set(banEntry{
		AuthID: "atom1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(30 * time.Minute), ResetSource: "header_absolute", CpaSynced: false,
	})
	target := &pluginapi.HostAuthFileEntry{Name: "atom1.json", AuthIndex: "atom1"}
	removed, remains, cpaOK, changed, err := clearBansMatchingTargetCAS(activeStore, target, "atom1.json", []banEntry{pre})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || !remains || !cpaOK || !changed {
		t.Fatalf("removed=%d remains=%v cpaOK=%v changed=%v", removed, remains, cpaOK, changed)
	}
	if redisable.Load() < 1 {
		t.Fatal("expected re-disable after DeleteIfOrCurrent miss")
	}
	if _, ok := activeStore.Get("atom1.json"); !ok {
		t.Fatal("newer ban must remain")
	}
}

// ---------- B) re-disable fail => result not Disabled ----------

func TestEnableCASRedisableFailLeavesResultEnabledWithDisableAction(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Disabled {
			close(enableEntered)
			<-releaseEnable
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.Error(w, "redisable fail", http.StatusBadGateway)
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "bf1", Name: "bf1.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = callHostAuthList })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "bf1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex: "bf1", Name: "bf1.json", FileName: "bf1.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	errCh := make(chan error, 1)
	go func() { errCh <- setAuthDisabled("bf1.json", false, "test-pass", nil, true) }()
	<-enableEntered
	activeStore.Set(banEntry{
		AuthID: "bf1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(20 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: false,
	})
	close(releaseEnable)
	err := <-errCh
	if err == nil {
		t.Fatal("expected re-disable error")
	}
	engine.mu.Lock()
	row := engine.results[0]
	engine.mu.Unlock()
	if row.Disabled {
		t.Fatalf("re-disable fail must leave Disabled=false: %+v", row)
	}
	if row.Action != "disable" {
		t.Fatalf("want disable recommendation, got %q", row.Action)
	}
}

// ---------- C) persist=false no per-account ban save ----------

func TestBulkPersistFalseDoesNotTriggerPerAccountBanSave(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(stopBanPersistWorkerForTest)

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = state
	currentConfig.Store(cfg)
	t.Cleanup(func() {
		currentConfig.Store(oldCfg)
		banStoreSaveFn = func(path string) error { return activeStore.Save(path) }
	})

	var saves atomic.Int32
	banStoreSaveFn = func(path string) error {
		saves.Add(1)
		return activeStore.Save(path)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "p1", Name: "p1.json", FileName: "p1.json", Disabled: true, Classification: "quota_exhausted", Action: "enable"},
		{AuthIndex: "p2", Name: "p2.json", FileName: "p2.json", Disabled: true, Classification: "quota_exhausted", Action: "enable"},
		{AuthIndex: "p3", Name: "p3.json", FileName: "p3.json", Disabled: true, Classification: "quota_exhausted", Action: "enable"},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{Name: "p1.json", AuthIndex: "p1"},
			{Name: "p2.json", AuthIndex: "p2"},
			{Name: "p3.json", AuthIndex: "p3"},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = callHostAuthList })

	now := time.Now()
	for _, id := range []string{"p1.json", "p2.json", "p3.json"} {
		activeStore.Set(banEntry{
			AuthID: id, Provider: "xai", ErrorCode: exhaustedErrorCode,
			BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
		})
	}
	before := saves.Load()
	for _, name := range []string{"p1.json", "p2.json", "p3.json"} {
		if err := setAuthDisabled(name, false, "test-pass", nil, false); err != nil {
			t.Fatal(err)
		}
	}
	if saves.Load() != before {
		t.Fatalf("persist=false must not call banStoreSaveFn per account: before=%d after=%d", before, saves.Load())
	}
	// Final bulk save uses saveActiveStoreErr (direct Save), not the persist worker.
	if err := saveActiveStoreErr(); err != nil {
		t.Fatal(err)
	}
	if saves.Load() != before {
		t.Fatalf("saveActiveStoreErr should not use banStoreSaveFn; worker saves=%d", saves.Load()-before)
	}
	loaded := newBanStore()
	if err := loaded.Load(state, now); err != nil {
		t.Fatal(err)
	}
	// Successful enable clears bans when no concurrent newer ban; final save has that state.
	for _, id := range []string{"p1.json", "p2.json", "p3.json"} {
		if _, ok := loaded.Get(id); ok {
			t.Fatalf("%s should be cleared after enable + final save", id)
		}
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatalf("final save must write state file: %v", err)
	}
}

// ---------- E) unban conflict not missing ----------

func TestUnbanKeptNewerReturnsConflictNotMissing(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	engine.mu.Lock()
	engine.running, engine.applying, engine.applyDraining, engine.actionInFlight = false, false, false, 0
	engine.mu.Unlock()

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Disabled {
			select {
			case <-enableEntered:
			default:
				close(enableEntered)
			}
			<-releaseEnable
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "ec1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	errCh := make(chan error, 1)
	var enabled, removed bool
	go func() {
		var e error
		enabled, removed, e = unbanOneAccount("ec1", "test-pass")
		errCh <- e
	}()
	<-enableEntered
	activeStore.Set(banEntry{
		AuthID: "ec1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(15 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: true,
	})
	close(releaseEnable)
	err := <-errCh
	if err == nil || !errors.Is(err, errUnbanSupersededByNewerBan) {
		t.Fatalf("want superseded conflict, got %v (enabled=%v removed=%v)", err, enabled, removed)
	}
	if enabled || removed {
		t.Fatalf("must not report success/missing path: enabled=%v removed=%v", enabled, removed)
	}
}

// ---------- F) shutdown final flush after producers ----------

func TestShutdownFinalBanFlushAfterProducers(t *testing.T) {
	isolateActiveStore(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(func() {
		stopBanPersistWorkerForTest()
		rearmBanDisposeWorkersForTest()
		// restore engine flags
		engine.mu.Lock()
		engine.stopped = false
		engine.applyDraining = false
		engine.mu.Unlock()
	})

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = state
	currentConfig.Store(cfg)
	t.Cleanup(func() {
		currentConfig.Store(oldCfg)
		banStoreSaveFn = func(path string) error { return activeStore.Save(path) }
	})
	banStoreSaveFn = func(path string) error { return activeStore.Save(path) }

	releaseProducer := make(chan struct{})
	engine.runWG.Add(1)
	go func() {
		defer engine.runWG.Done()
		<-releaseProducer
		now := time.Now()
		activeStore.Set(banEntry{
			AuthID: "late-shutdown", Provider: "xai", ErrorCode: unauthorizedErrorCode,
			BannedAt: now, ResetAt: now.AddDate(100, 0, 0),
			ResetSource: "manual_unban", CpaSynced: false,
		})
	}()

	done := make(chan struct{})
	go func() {
		engine.shutdown()
		close(done)
	}()
	// Give shutdown time to reach runWG.Wait
	time.Sleep(30 * time.Millisecond)
	close(releaseProducer)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung")
	}

	loaded := newBanStore()
	if err := loaded.Load(state, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("late-shutdown"); !ok {
		t.Fatal("final flush after producers must persist late ban")
	}
}
