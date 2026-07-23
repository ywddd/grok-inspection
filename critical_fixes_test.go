package main

import (
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

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

func isBusyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "busy") ||
		strings.Contains(msg, "already running") ||
		strings.Contains(msg, "unban") ||
		strings.Contains(msg, "忙") ||
		strings.Contains(msg, "解禁") ||
		strings.Contains(msg, "巡检进行中") ||
		strings.Contains(msg, "批量操作")
}
func resetUnbanJobForTest() {
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.runID++
	unbanJob.mu.Unlock()
	unbanJob.wg.Wait()
}

func isolateUnbanJob(t *testing.T) {
	t.Helper()
	resetUnbanJobForTest()
	t.Cleanup(func() { resetUnbanJobForTest() })
}

// 1) Old in-flight unban must not delete a newer concurrent ban.
func TestUnbanSuccessDoesNotDeleteNewerBan(t *testing.T) {
	var enableCalls atomic.Int32
	blockEnable := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCalls.Add(1)
		<-blockEnable
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	isolateUnbanJob(t)
	isolateActiveStore(t)
	activeStore.Set(banEntry{
		AuthID:      "race-auth",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    time.Now().Add(-time.Hour),
		ResetAt:     time.Now().Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})
	old, ok := activeStore.Get("race-auth")
	if !ok || old.Revision == 0 {
		t.Fatalf("seed ban missing: %#v", old)
	}

	if err := startUnbanJob([]string{"race-auth"}, "", "test-pass"); err != nil {
		t.Fatalf("startUnbanJob: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for enableCalls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if enableCalls.Load() == 0 {
		close(blockEnable)
		t.Fatal("enable never called")
	}

	activeStore.Set(banEntry{
		AuthID:      "race-auth",
		Provider:    "xai",
		ErrorCode:   permissionDeniedErrorCode,
		BannedAt:    time.Now(),
		ResetAt:     time.Now().Add(24 * time.Hour),
		ResetSource: "manual_unban",
		CpaSynced:   true,
	})
	newer, ok := activeStore.Get("race-auth")
	if !ok || newer.Revision <= old.Revision {
		close(blockEnable)
		t.Fatalf("expected newer revision, old=%d newer=%#v", old.Revision, newer)
	}

	close(blockEnable)
	unbanJob.wg.Wait()

	got, still := activeStore.Get("race-auth")
	if !still {
		t.Fatal("old in-flight unban deleted a newer ban")
	}
	if got.Revision != newer.Revision {
		t.Fatalf("ban revision changed: got=%d want=%d", got.Revision, newer.Revision)
	}
	if got.ErrorCode != permissionDeniedErrorCode {
		t.Fatalf("ban error_code = %q, want permission-denied", got.ErrorCode)
	}
}

// 2) Stop must keep unban busy until the in-flight worker exits.
func TestStopUnbanBlocksNewStartUntilDrain(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	isolateUnbanJob(t)
	isolateActiveStore(t)
	for i := 0; i < 3; i++ {
		activeStore.Set(banEntry{
			AuthID:      fmt.Sprintf("stop-%d", i),
			Provider:    "xai",
			ErrorCode:   exhaustedErrorCode,
			BannedAt:    time.Now(),
			ResetAt:     time.Now().Add(time.Hour),
			ResetSource: "local_plus_fallback",
			CpaSynced:   true,
		})
	}

	if err := startUnbanJob(nil, "quota", "test-pass"); err != nil {
		t.Fatalf("startUnbanJob: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	stopUnbanJob()

	st := unbanJobStatus()
	if st["running"] != true {
		close(release)
		t.Fatalf("after stop, job should still be draining/busy, status=%#v", st)
	}
	if err := startUnbanJob(nil, "quota", "test-pass"); err == nil || !strings.Contains(err.Error(), "busy") {
		close(release)
		t.Fatalf("expected busy while draining, err=%v", err)
	}

	close(release)
	unbanJob.wg.Wait()
	st = unbanJobStatus()
	if st["running"] != false {
		t.Fatalf("after drain, running should be false, status=%#v", st)
	}
}

// 3) Single unban holds the atomic claim for the whole host call; other jobs cannot start.
func TestSingleUnbanClaimBlocksInspectionAndBulkJobs(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	isolateUnbanJob(t)
	isolateActiveStore(t)
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	oldRunning := engine.running
	oldApplying := engine.applying
	oldDraining := engine.applyDraining
	oldInFlight := engine.actionInFlight
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.results = []accountResult{{
		AuthIndex: "x1", Name: "x1.json", FileName: "x1.json",
		Classification: "quota_exhausted", Action: "disable", Disabled: false,
	}}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.running = oldRunning
		engine.applying = oldApplying
		engine.applyDraining = oldDraining
		engine.actionInFlight = oldInFlight
		engine.mu.Unlock()
		resetUnbanJobForTest()
	})

	activeStore.Set(banEntry{
		AuthID:      "single-claim",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    time.Now(),
		ResetAt:     time.Now().Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})

	var (
		unbanErr error
		done     sync.WaitGroup
	)
	done.Add(1)
	go func() {
		defer done.Done()
		_, _, unbanErr = unbanOneAccount("single-claim", "test-pass")
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("single unban never entered host enable call")
	}

	// Host call still in-flight: claim must block inspection / bulk unban / bulk apply.
	if err := engine.start(startRequest{Workers: 1}); err == nil || !(isBusyErr(err)) {
		close(release)
		done.Wait()
		t.Fatalf("start during single unban: %v", err)
	}
	if err := startUnbanJob([]string{"single-claim"}, "", "test-pass"); err == nil || !strings.Contains(err.Error(), "busy") {
		close(release)
		done.Wait()
		t.Fatalf("startUnbanJob during single unban: %v", err)
	}
	if err := engine.startApply(applyRequest{ForceAction: "disable", AuthIndexes: []string{"x1"}}, "test-pass", nil); err == nil || !isBusyErr(err) {
		close(release)
		done.Wait()
		t.Fatalf("startApply during single unban: %v", err)
	}

	close(release)
	done.Wait()
	if unbanErr != nil {
		t.Fatalf("unbanOneAccount: %v", unbanErr)
	}
	st := unbanJobStatus()
	if st["running"] != false {
		t.Fatalf("slot not released after single unban: %#v", st)
	}
}

// 4a) waitHostCalls(0) waits until release.
func TestWaitHostCallsZeroTimeoutWaitsForRelease(t *testing.T) {
	for hostCallInflight() > 0 {
		releaseHostCall()
	}
	// Prior shutdown tests leave admission closed; re-open before acquire.
	rearmHostCallAdmissionForTest()
	t.Cleanup(func() {
		for hostCallInflight() > 0 {
			releaseHostCall()
		}
		rearmHostCallAdmissionForTest()
	})
	acquireHostCall()
	done := make(chan struct{})
	go func() {
		waitHostCalls(0)
		close(done)
	}()
	select {
	case <-done:
		releaseHostCall()
		t.Fatal("waitHostCalls(0) returned before release")
	case <-time.After(80 * time.Millisecond):
	}
	releaseHostCall()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitHostCalls(0) did not return after release")
	}
}

// 4b) stop path waits until restore worker finishes (local channels, no global Once).
func TestWaitRestoreLoopDoneBlocksUntilWorkerExits(t *testing.T) {
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	started := make(chan struct{})
	go func() {
		defer close(doneCh)
		close(started)
		<-stopCh
		// Simulate in-flight restore work after stop is signaled.
		time.Sleep(80 * time.Millisecond)
	}()
	<-started

	returned := make(chan struct{})
	go func() {
		close(stopCh)
		waitRestoreLoopDone(doneCh)
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("waitRestoreLoopDone returned before worker finished")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitRestoreLoopDone did not observe worker exit")
	}
}

// 5) Ban state loads only from the configured state file path.
func TestLoadBanStateUsesConfiguredPathOnly(t *testing.T) {
	dir := t.TempDir()
	goodPath := filepath.Join(dir, "good-bans.json")
	badPath := filepath.Join(dir, "bad-bans.json")

	good := newBanStore()
	good.Set(banEntry{
		AuthID:      "from-good",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    time.Now(),
		ResetAt:     time.Now().Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})
	if err := good.Save(goodPath); err != nil {
		t.Fatal(err)
	}
	bad := newBanStore()
	bad.Set(banEntry{
		AuthID:      "from-bad",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    time.Now(),
		ResetAt:     time.Now().Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})
	if err := bad.Save(badPath); err != nil {
		t.Fatal(err)
	}

	isolateActiveStore(t)
	_ = os.Setenv("GROK_INSPECTION_DATA_DIR", filepath.Dir(badPath))
	t.Cleanup(func() { restorePackageTestDataEnv() })

	oldCfg := loadedConfig()
	cfg := defaultPluginConfig()
	cfg.PersistState = true
	cfg.StateFile = goodPath
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })
	loadBanState(cfg)

	if _, ok := activeStore.Get("from-good"); !ok {
		t.Fatal("expected ban from configured state file")
	}
	if _, ok := activeStore.Get("from-bad"); ok {
		t.Fatal("must not load ban state from default/unrelated path")
	}
}

// 6) Concurrent Save must not let a stale snapshot overwrite a newer one.
func TestBanStoreSaveStaleSnapshotCannotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bans.json")
	store := newBanStore()

	var wg sync.WaitGroup
	const n = 40
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("auth-%03d", i)
			store.Set(testEntry(id, time.Unix(int64(1000+i), 0)))
			if err := store.Save(path); err != nil {
				t.Errorf("Save(%s): %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	reloaded := newBanStore()
	if err := reloaded.Load(path, time.Now()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("auth-%03d", i)
		if _, ok := reloaded.Get(id); !ok {
			t.Fatalf("missing %s after concurrent saves (stale snapshot overwrite?)", id)
		}
	}
}

// 7) No Scheduler capability; scheduler.pick is unknown_method; UsagePlugin stays on.
func TestPluginDoesNotHandleSchedulerPick(t *testing.T) {
	reg := pluginRegistration()
	if reg.Capabilities.Scheduler {
		t.Fatal("Scheduler capability must be false")
	}
	if !reg.Capabilities.UsagePlugin {
		t.Fatal("UsagePlugin must remain enabled for autoban")
	}

	raw, err := handleMethod(pluginabi.MethodSchedulerPick, []byte(`{}`))
	if err != nil {
		t.Fatalf("handleMethod error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "unknown_method") {
		t.Fatalf("scheduler.pick should be unknown_method, got %s", text)
	}
	if strings.Contains(text, `"handled"`) {
		t.Fatalf("scheduler.pick must not return a pick response: %s", text)
	}

	// Usage path still works (invalid body becomes empty ok envelope).
	usageRaw, errUsage := handleMethod(pluginabi.MethodUsageHandle, []byte(`{}`))
	if errUsage != nil {
		t.Fatalf("usage handle error = %v", errUsage)
	}
	if !strings.Contains(string(usageRaw), `"ok":true`) && !strings.Contains(string(usageRaw), `"ok": true`) {
		// okEnvelope always sets ok true
		if !strings.Contains(string(usageRaw), "ok") {
			t.Fatalf("usage handle unexpected: %s", string(usageRaw))
		}
	}
}

// 8) Force enable/disable recalculates recommended action.
func TestSetAuthDisabledRecalculatesAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{
			AuthIndex: "q1", Name: "q1.json", FileName: "q1.json",
			Disabled: true, Classification: "quota_exhausted", Action: "keep", Reason: "quota",
		},
		{
			AuthIndex: "p1", Name: "p1.json", FileName: "p1.json",
			Disabled: true, Classification: "permission_denied", Action: "keep", Reason: "permission",
		},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	if err := setAuthDisabled("q1.json", false, "test-pass", nil, false); err != nil {
		t.Fatalf("enable quota: %v", err)
	}
	if err := setAuthDisabled("p1.json", false, "test-pass", nil, false); err != nil {
		t.Fatalf("enable permission: %v", err)
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()
	var q, p *accountResult
	for i := range engine.results {
		switch engine.results[i].AuthIndex {
		case "q1":
			q = &engine.results[i]
		case "p1":
			p = &engine.results[i]
		}
	}
	if q == nil || p == nil {
		t.Fatalf("results missing: %#v", engine.results)
	}
	if q.Disabled || q.Action != "disable" {
		t.Fatalf("quota after enable: disabled=%v action=%q", q.Disabled, q.Action)
	}
	if p.Disabled || p.Action != "disable" {
		t.Fatalf("permission after enable: disabled=%v action=%q", p.Disabled, p.Action)
	}
}

// 9) Classify-scoped inspect keeps disabled accounts visible on the category card.
func TestClassifyScopedInspectIncludesDisabledTargets(t *testing.T) {
	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	defer func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	}()

	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "en1", Name: "en1.json", Provider: "xai", Disabled: false},
		{AuthIndex: "dis1", Name: "dis1.json", Provider: "xai", Disabled: true},
	}
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: files}, nil
	}
	var probed []string
	var mu sync.Mutex
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		mu.Lock()
		probed = append(probed, file.AuthIndex)
		mu.Unlock()
		return accountResult{
			AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name,
			Disabled: file.Disabled, Classification: "quota_exhausted", Action: "disable",
		}
	}

	e := &inspectionEngine{
		running: true, runID: 1, workers: 2,
		results: []accountResult{
			{AuthIndex: "en1", Name: "en1.json", FileName: "en1.json", Disabled: false, Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "dis1", Name: "dis1.json", FileName: "dis1.json", Disabled: true, Classification: "quota_exhausted", Action: "keep"},
		},
	}
	e.run(1, 2, false, false, false, []string{"quota_exhausted"})

	mu.Lock()
	defer mu.Unlock()
	if len(probed) != 2 {
		t.Fatalf("probed=%v, want both enabled and disabled category accounts", probed)
	}
	seen := map[string]bool{}
	for _, id := range probed {
		seen[id] = true
	}
	if !seen["en1"] || !seen["dis1"] {
		t.Fatalf("probed=%v", probed)
	}
}

// 10) Docs should not claim "never auto-disable" while default is enabled.
func TestReadmeMatchesAutobanDefault(t *testing.T) {
	for _, name := range []string{"README.md", "README.en.md"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if strings.Contains(text, "插件不会自动禁用") || strings.Contains(text, "never auto-disables") || strings.Contains(text, "The plugin never auto-disables") {
			t.Fatalf("%s still claims the plugin never auto-disables", name)
		}
	}
	if !defaultPluginConfig().Enabled {
		t.Fatal("default autoban_enabled should be true")
	}
}

func TestRecommendActionForClassification(t *testing.T) {
	if got := recommendAction("quota_exhausted", false); got != "disable" {
		t.Fatalf("quota enabled -> %q", got)
	}
	if got := recommendAction("quota_exhausted", true); got != "keep" {
		t.Fatalf("quota disabled -> %q", got)
	}
	if got := recommendAction("permission_denied", false); got != "disable" {
		t.Fatalf("permission enabled -> %q", got)
	}
	if got := recommendAction("healthy", true); got != "enable" {
		t.Fatalf("healthy disabled -> %q", got)
	}
	if got := recommendAction("reauth", false); got != "delete" {
		t.Fatalf("reauth -> %q", got)
	}
}
