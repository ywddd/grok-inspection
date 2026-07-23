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

// ---------- 1) 403 boundary / content-safety precedence ----------

func TestIsAccountLevelPermissionDeniedRequiresHTTP403(t *testing.T) {
	msg := "Access to the chat endpoint is denied. Please ensure you're using the correct credentials."
	if isAccountLevelPermissionDenied(http.StatusInternalServerError, permissionDeniedErrorCode, msg) {
		t.Fatal("500 must not be account-level permission denied")
	}
	if isAccountLevelPermissionDenied(http.StatusOK, permissionDeniedErrorCode, msg) {
		t.Fatal("200 must not be account-level permission denied")
	}
	if !isAccountLevelPermissionDenied(http.StatusForbidden, permissionDeniedErrorCode, msg) {
		t.Fatal("403 with account evidence must match")
	}
}

func TestClassifySafetyWithUnauthorizedTextIsProbeErrorNotReauth(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang: LangEN, ChatStatus: 403,
		ChatCode:  "permission-denied",
		ChatError: "Content violates usage guidelines: unauthorized content",
	})
	if got.Classification == "permission_denied" || got.Classification == "reauth" || got.Action == "disable" || got.Action == "delete" {
		t.Fatalf("safety+unauthorized text must stay probe_error/keep: %+v", got)
	}
	if got.Classification != "probe_error" || got.Action != "keep" {
		t.Fatalf("want probe_error/keep, got %+v", got)
	}
}

func TestClassifyNon403SuspendedTextNotPermissionDenied(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang: LangEN, ChatStatus: 500,
		ChatCode: "internal", ChatError: "account suspended temporarily",
	})
	if got.Classification == "permission_denied" {
		t.Fatalf("non-403 suspended text must not be permission_denied: %+v", got)
	}
}

// ---------- 2) 401 always unauthorized category/error_code ----------

func TestDetect401AlwaysUnauthorizedEvenWhenBodyIsPermissionDenied(t *testing.T) {
	cfg := defaultPluginConfig()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	entry, ok := detectBan(pluginapi.UsageRecord{
		Provider: "xai", AuthID: "a401", Failed: true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 401,
			Body:       `{"code":"permission-denied","error":"no"}`,
		},
	}, cfg, now)
	if !ok {
		t.Fatal("401 must ban")
	}
	if entry.ErrorCode != unauthorizedErrorCode {
		t.Fatalf("visible error_code=%q want %q", entry.ErrorCode, unauthorizedErrorCode)
	}
	if banCategoryOf(entry.ErrorCode) != "unauthorized" {
		t.Fatalf("category=%s", banCategoryOf(entry.ErrorCode))
	}
	// Optional diagnostic may keep body code without polluting category.
	if entry.ErrorCodeDiag != "" && entry.ErrorCodeDiag != "permission-denied" {
		t.Fatalf("diag=%q", entry.ErrorCodeDiag)
	}
}

func TestDetect401BodySpendingLimitStillUnauthorizedCategory(t *testing.T) {
	cfg := defaultPluginConfig()
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	entry, ok := detectBan(pluginapi.UsageRecord{
		Provider: "xai", AuthID: "a401b", Failed: true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 401,
			Body:       `{"code":"personal-team-blocked:spending-limit","error":"x"}`,
		},
	}, cfg, now)
	if !ok {
		t.Fatal("401 must ban")
	}
	if entry.ErrorCode != unauthorizedErrorCode {
		t.Fatalf("error_code=%q", entry.ErrorCode)
	}
	if banCategoryOf(entry.ErrorCode) != "unauthorized" {
		t.Fatalf("category=%s", banCategoryOf(entry.ErrorCode))
	}
}

// ---------- 3) reset clock injection ----------

func TestAbsoluteResetTimeUsesProvidedNowAndRejectsStaleRFC3339(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	hPast := http.Header{}
	hPast.Set("X-RateLimit-Reset-At", now.Add(-time.Hour).Format(time.RFC3339))
	if _, ok := absoluteResetTime(hPast, now); ok {
		t.Fatal("past RFC3339 must be rejected")
	}
	hFar := http.Header{}
	hFar.Set("X-RateLimit-Reset-At", now.AddDate(10, 0, 0).Format(time.RFC3339))
	if _, ok := absoluteResetTime(hFar, now); ok {
		t.Fatal("far-future RFC3339 must be rejected")
	}
	hOK := http.Header{}
	want := now.Add(2 * time.Hour)
	hOK.Set("X-RateLimit-Reset-At", want.Format(time.RFC3339))
	got, ok := absoluteResetTime(hOK, now)
	if !ok || !got.Equal(want) {
		t.Fatalf("got=%v ok=%v want=%v", got, ok, want)
	}
	// Unix ms with fixed now
	hMs := http.Header{}
	ms := now.Add(90 * time.Minute).UnixMilli()
	hMs.Set("X-RateLimit-Reset-At", fmt.Sprintf("%d", ms))
	got, ok = absoluteResetTime(hMs, now)
	if !ok || got.UnixMilli() != ms {
		t.Fatalf("ms got=%v ok=%v", got, ok)
	}
}

// ---------- 4) schedule.json concurrency / independent load / typed error ----------

func TestUpdateInspectionScheduleDoesNotExposeUnpersistedConfig(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	resetStoreIOForTest()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})
	engine.mu.Lock()
	old := engine.schedule
	engine.schedule = defaultInspectionSchedule()
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.schedule = old
		engine.mu.Unlock()
	})

	// Point schedule path under a non-writable parent (file as dir).
	bad := filepath.Join(dir, "not-dir")
	if err := os.WriteFile(bad, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Override via results path parent trick used previously
	setStoreFilePathForTest(filepath.Join(bad, "results.json"))

	enabled := true
	interval := 9
	before := inspectionScheduleSnapshot()
	_, err := updateInspectionSchedule(inspectionScheduleUpdate{Enabled: &enabled, IntervalMinutes: &interval})
	if err == nil {
		t.Fatal("expected persist failure")
	}
	var pe *schedulePersistError
	if !errors.As(err, &pe) {
		t.Fatalf("want typed schedulePersistError, got %T %v", err, err)
	}
	after := inspectionScheduleSnapshot()
	if after.Enabled != before.Enabled || after.IntervalMinutes != before.IntervalMinutes {
		t.Fatalf("run loop saw unpersisted config: before=%+v after=%+v", before, after)
	}
}

func TestLoadFromDiskUsesScheduleJSONWhenResultsMissing(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	resetStoreIOForTest()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})
	cfg := defaultInspectionSchedule()
	cfg.Enabled = true
	cfg.IntervalMinutes = 11
	if err := saveInspectionScheduleSync(cfg); err != nil {
		t.Fatal(err)
	}
	// results.json absent
	engine.mu.Lock()
	engine.schedule = defaultInspectionSchedule()
	engine.results = nil
	engine.mu.Unlock()
	engine.loadFromDisk()
	got := inspectionScheduleSnapshot()
	if !got.Enabled || got.IntervalMinutes != 11 {
		t.Fatalf("schedule from schedule.json not loaded: %+v", got)
	}
}

func TestScheduleSaveSerialAndDoubleSave(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	resetStoreIOForTest()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cfg := defaultInspectionSchedule()
			cfg.Enabled = true
			cfg.IntervalMinutes = 1 + (n % 5)
			errs <- saveInspectionScheduleSync(cfg)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent save: %v", err)
		}
	}
	// consecutive overwrites
	for i := 0; i < 5; i++ {
		cfg := defaultInspectionSchedule()
		cfg.IntervalMinutes = 2 + i
		if err := saveInspectionScheduleSync(cfg); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := loadInspectionScheduleFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.IntervalMinutes != 6 {
		t.Fatalf("last write wins: %+v", loaded)
	}
}

// ---------- 5) dispose queue bounded stop ----------

func TestDisposeQueueStartStopNoAddWaitRace(t *testing.T) {
	// Concurrent startWorkers / stopAndWait must not panic on WaitGroup.
	for round := 0; round < 50; round++ {
		q := newBanDisposeQueue(16, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			q.startWorkers()
		}()
		go func() {
			defer wg.Done()
			q.stopAndWait()
		}()
		wg.Wait()
		// Second stop is idempotent.
		q.stopAndWait()
	}
}

func TestDisposeQueueStopDiscardsPendingAndDoesNotDrainNetwork(t *testing.T) {
	// Capacity filled while held: stop must drop pending quickly without processing.
	q := newBanDisposeQueue(32, 2)
	q.mu.Lock()
	q.testHold = true
	q.mu.Unlock()
	q.startWorkers()
	for i := 0; i < 32; i++ {
		if !q.enqueue(fmt.Sprintf("pend-%d", i), uint64(i+1)) {
			t.Fatalf("enqueue %d rejected", i)
		}
	}
	if q.queuedCount() != 32 {
		t.Fatalf("queued=%d", q.queuedCount())
	}
	start := time.Now()
	q.stopAndWait()
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("stop took too long (%s); must not drain pending network work", elapsed)
	}
	if q.queuedCount() != 0 || q.pendingCount() != 0 {
		t.Fatalf("pending not discarded: q=%d p=%d", q.queuedCount(), q.pendingCount())
	}
	// Enqueue after stop must fail.
	if q.enqueue("after-stop", 1) {
		t.Fatal("enqueue after stop must fail")
	}
}

// ---------- 6) usage persist is bounded dirty worker ----------

func TestUsagePersistUsesDirtyWorkerNotPerCallGoroutine(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)
	stopBanPersistWorkerForTest()
	t.Cleanup(stopBanPersistWorkerForTest)

	dir := t.TempDir()
	state := filepath.Join(dir, "bans.json")
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.Enabled = true
	cfg.PersistState = true
	cfg.StateFile = state
	cfg.LogMatches = false
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		rec := pluginapi.UsageRecord{
			Provider: "xai",
			AuthID:   fmt.Sprintf("persist-%d", i),
			Failed:   true,
			Failure: pluginapi.UsageFailure{
				StatusCode: 429,
				Body:       `{"code":"subscription:free-usage-exhausted","error":"x"}`,
			},
		}
		if _, err := handleUsageRecord(rec, cfg, now); err != nil {
			t.Fatalf("usage %d: %v", i, err)
		}
	}
	// Coalesced flush must land all local unsynced bans without panicking.
	if err := flushBanPersistWorker(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Reload from disk proves persist worker wrote.
	loaded := newBanStore()
	if err := loaded.Load(state, now); err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Count() < 20 {
		t.Fatalf("persisted count=%d want >=20", loaded.Count())
	}
	// Queue-full path also marks dirty/unsynced.
	markBanDisposeQueueFullForTest(t)
	rec := pluginapi.UsageRecord{
		Provider: "xai", AuthID: "qfull-persist", Failed: true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       `{"code":"subscription:free-usage-exhausted","error":"x"}`,
		},
	}
	entry, err := handleUsageRecord(rec, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	if entry.CpaSynced {
		t.Fatal("queue full must leave unsynced")
	}
	if err := flushBanPersistWorker(); err != nil {
		t.Fatal(err)
	}
	got, ok := activeStore.Get("qfull-persist")
	if !ok || got.CpaSynced {
		t.Fatalf("local unsynced missing: ok=%v %#v", ok, got)
	}
}

// ---------- 7) enable CAS vs concurrent ban ----------

func TestEnableCASKeepsNewerBanAndRedisables(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	var redisable atomic.Int32
	var enablePatches atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "status") {
			var body struct {
				Name     string `json:"name"`
				Disabled bool   `json:"disabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body.Disabled {
				enablePatches.Add(1)
				// Block first enable so a concurrent ban can land.
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
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "cas1", Name: "cas1.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "cas1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	old, _ := activeStore.Get("cas1.json")

	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex: "cas1", Name: "cas1.json", FileName: "cas1.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- setAuthDisabled("cas1.json", false, "test-pass", nil, false)
	}()
	select {
	case <-enableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("enable never entered management PATCH")
	}
	// Newer ban lands while enable is in flight.
	activeStore.Set(banEntry{
		AuthID: "cas1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(30 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: false,
	})
	newer, _ := activeStore.Get("cas1.json")
	if newer.Revision <= old.Revision {
		t.Fatalf("newer revision not advanced: old=%d new=%d", old.Revision, newer.Revision)
	}
	close(releaseEnable)
	select {
	case err := <-errCh:
		// CAS may return re-disable error; either nil or non-nil is ok if state is correct.
		_ = err
	case <-time.After(5 * time.Second):
		t.Fatal("enable hung")
	}
	got, ok := activeStore.Get("cas1.json")
	if !ok {
		t.Fatal("newer ban must not be cleared by enable CAS")
	}
	if got.Revision != newer.Revision {
		t.Fatalf("revision changed unexpectedly: got=%d want=%d", got.Revision, newer.Revision)
	}
	if redisable.Load() < 1 {
		t.Fatal("expected re-disable after concurrent ban")
	}
	if enablePatches.Load() < 1 {
		t.Fatal("expected enable patch")
	}
}

func TestBatchDeleteRemoteFailKeepsLocalEvenWithDupAliases(t *testing.T) {
	isolateActiveStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			http.Error(w, "upstream boom", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "dup.json", Provider: "xai", ErrorCode: permissionDeniedErrorCode,
		BannedAt: now, ResetAt: now.AddDate(50, 0, 0), ResetSource: "manual_unban", CpaSynced: true,
	})
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "a1", Name: "dup-a", FileName: "dup.json", Classification: "permission_denied", Action: "delete"},
		{AuthIndex: "a2", Name: "dup-b", FileName: "dup.json", Classification: "permission_denied", Action: "delete"},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	fails := deleteAuthFilesBatch(engine.results, "test-pass", nil, false)
	if len(fails) == 0 {
		t.Fatal("expected remote failures")
	}
	if _, ok := activeStore.Get("dup.json"); !ok {
		t.Fatal("ban must remain when remote DELETE fails")
	}
	engine.mu.Lock()
	n := len(engine.results)
	engine.mu.Unlock()
	if n != 2 {
		t.Fatalf("results cleared on failure: n=%d", n)
	}
}

// ---------- 8) restore single critical section re-disable error ----------

func TestRestoreReDisableFailureWritesCpaSyncError(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)

	// Deterministic: expired ban, enable succeeds, but a newer revision is already
	// present when enable returns; re-disable fails and must write CpaSyncError.
	// We inject the newer ban after snapshotting expectedRev by interleaving on
	// the enable PATCH (blocked until newer Set completes).
	enableGate := make(chan struct{})
	releaseEnable := make(chan struct{})
	var redisableFails atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Disabled {
			close(enableGate)
			<-releaseEnable
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		redisableFails.Add(1)
		http.Error(w, "redisable failed", http.StatusBadGateway)
	}))
	defer server.Close()
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{
		AuthID: "rest1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-2 * time.Hour), ResetAt: now.Add(-time.Minute),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	old, _ := activeStore.Get("rest1")

	done := make(chan struct{})
	go func() {
		_, _ = restoreExpiredBans(activeStore, now)
		close(done)
	}()
	select {
	case <-enableGate:
	case <-time.After(3 * time.Second):
		t.Fatal("restore enable never hit management")
	}
	// Land newer ban while enable is still in flight (same critical section after return).
	activeStore.Set(banEntry{
		AuthID: "rest1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour),
		ResetSource: "header_absolute", CpaSynced: false,
	})
	close(releaseEnable)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("restore hung")
	}
	got, ok := activeStore.Get("rest1")
	if !ok {
		t.Fatal("newer ban must remain after restore race")
	}
	if got.Revision <= old.Revision {
		t.Fatalf("expected newer revision kept: old=%d got=%d", old.Revision, got.Revision)
	}
	if redisableFails.Load() < 1 {
		t.Fatal("expected re-disable attempt")
	}
	if got.CpaSynced || strings.TrimSpace(got.CpaSyncError) == "" {
		t.Fatalf("re-disable failure must set real CpaSyncError: %#v", got)
	}
}

func TestBanStoreSetKeepsLongerManualWindowAndSource(t *testing.T) {
	store := newBanStore()
	store.Set(banEntry{
		AuthID: "m1", Provider: "xai", ErrorCode: permissionDeniedErrorCode,
		BannedAt: time.Unix(1, 0), ResetAt: time.Unix(1000, 0),
		ResetSource: "manual_unban", CpaSynced: true,
	})
	first, _ := store.Get("m1")
	store.Set(banEntry{
		AuthID: "m1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: time.Unix(2, 0), ResetAt: time.Unix(50, 0),
		ResetSource: "local_plus_fallback", CpaSynced: false,
	})
	second, _ := store.Get("m1")
	if !second.ResetAt.Equal(time.Unix(1000, 0)) {
		t.Fatalf("ResetAt=%v", second.ResetAt)
	}
	if second.ResetSource != "manual_unban" {
		t.Fatalf("ResetSource=%q", second.ResetSource)
	}
	if second.ErrorCode != permissionDeniedErrorCode {
		t.Fatalf("ErrorCode=%q (must keep manual/403 reason)", second.ErrorCode)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("revision must advance")
	}
}
