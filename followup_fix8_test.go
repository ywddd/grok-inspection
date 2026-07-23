package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
	// Full transaction: interval and enabled must be a coherent pair from one writer.
	// With even i: enabled=true interval=1+i; odd: enabled=false.
	if mem.Enabled {
		if mem.IntervalMinutes%2 == 0 {
			// interval = 1+i with i even => odd interval when enabled
			// i even => enabled true, interval = 1+even = odd. OK if odd.
		}
		if mem.IntervalMinutes%2 == 0 {
			t.Fatalf("incoherent enabled=true with even interval from odd writer: %+v", mem)
		}
	} else {
		if mem.IntervalMinutes%2 != 0 {
			// i odd => enabled false, interval = 1+odd = even
			t.Fatalf("incoherent enabled=false with odd interval: %+v", mem)
		}
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

	// Worker should retry until success.
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

	// Mark dirty again then stop while next saves fail once — final flush must run.
	activeStore.Set(banEntry{
		AuthID: "pf2", Provider: "xai", ErrorCode: unauthorizedErrorCode,
		BannedAt: now, ResetAt: now.AddDate(100, 0, 0),
		ResetSource: "manual_unban", CpaSynced: false,
	})
	// Fail the next single save then succeed for final flush.
	calls.Store(0)
	failUntil = 1
	banStoreSaveFn = func(path string) error {
		n := calls.Add(1)
		if n == 1 {
			return errors.New("fail once")
		}
		return activeStore.Save(path)
	}
	markBanStoreDirty()
	// Ensure dirty observed; stop should final-flush.
	time.Sleep(20 * time.Millisecond)
	stopBanPersistWorker()
	// stop leaves worker stopped; rearm for other tests via ForTest helper
	// but first verify pf2 on disk
	loaded := newBanStore()
	if err := loaded.Load(state, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Get("pf2"); !ok {
		t.Fatalf("final flush missing pf2; calls=%d", calls.Load())
	}
	// rearm
	stopBanPersistWorkerForTest()
}

// ---------- 6) TestMain resets engine after init load ----------

func TestEngineNotHoldingRepoDataAfterTestMainReset(t *testing.T) {
	// After TestMain reset, default paths are under temp; a fresh load of empty
	// temp must not reintroduce repo results. We simulate init pollution then reset.
	engine.mu.Lock()
	engine.results = []accountResult{{
		AuthIndex: "repo-pollution", Name: "repo.json", FileName: "repo.json",
		Classification: "ok", Action: "keep",
	}}
	engine.schedule.Enabled = true
	engine.schedule.IntervalMinutes = 99
	engine.mu.Unlock()

	resetEngineAndStoresForTestIsolation()

	engine.mu.Lock()
	defer engine.mu.Unlock()
	for _, r := range engine.results {
		if r.AuthIndex == "repo-pollution" || r.FileName == "repo.json" {
			t.Fatalf("repo pollution still present: %+v", r)
		}
	}
	// Empty temp has no schedule.json/results.json → defaults.
	if engine.schedule.IntervalMinutes == 99 && engine.schedule.Enabled {
		// default may enable false; 99 must not survive
		t.Fatalf("schedule not reset: %+v", engine.schedule)
	}
	if packageTestDataDir == "" {
		t.Fatal("missing packageTestDataDir")
	}
	// Ensure store path still temp
	if !strings.HasPrefix(filepath.Clean(storeFilePath()), filepath.Clean(packageTestDataDir)) {
		t.Fatalf("store path %q not under temp", storeFilePath())
	}
}

// ---------- 8) ErrorCodeDiag matches retained ErrorCode ----------

func TestBanStoreSetDiagMatchesRetainedErrorCode(t *testing.T) {
	store := newBanStore()
	store.Set(banEntry{
		AuthID: "d1", Provider: "xai", ErrorCode: permissionDeniedErrorCode,
		ErrorCodeDiag: "", BannedAt: time.Unix(1, 0), ResetAt: time.Unix(1000, 0),
		ResetSource: "manual_unban", CpaSynced: true,
	})
	store.Set(banEntry{
		AuthID: "d1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		ErrorCodeDiag: "subscription:free-usage-exhausted",
		BannedAt:      time.Unix(2, 0), ResetAt: time.Unix(50, 0),
		ResetSource: "local_plus_fallback", CpaSynced: false,
	})
	got, ok := store.Get("d1")
	if !ok {
		t.Fatal("missing")
	}
	if got.ErrorCode != permissionDeniedErrorCode {
		t.Fatalf("ErrorCode=%q", got.ErrorCode)
	}
	if got.ErrorCodeDiag == "subscription:free-usage-exhausted" {
		t.Fatalf("diag leaked from shorter ban under retained 403 code: %#v", got)
	}
	if got.ErrorCodeDiag != "" {
		t.Fatalf("diag should match retained 403 (empty): %#v", got)
	}
}

// ensure fmt used
var _ = fmt.Sprintf

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

	// persist=false bulk path: must still dirty-persist sync state after re-disable.
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
	if err := flushBanPersistWorker(); err != nil {
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
		t.Fatalf("kept-newer re-disable must not count as unban enabled success: %#v", st)
	}
	if st["failed"].(int) != 0 {
		t.Fatalf("successful re-disable is not a failure: %#v", st)
	}
	got, ok := activeStore.Get("ub1")
	if !ok || got.Revision <= old.Revision {
		t.Fatalf("newer ban missing: ok=%v %#v", ok, got)
	}
	if redisable.Load() < 1 {
		t.Fatal("expected re-disable")
	}
}
