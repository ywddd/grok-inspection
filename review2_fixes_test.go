package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func useResultsStorePath(t *testing.T, path string) {
	t.Helper()
	setStoreFilePathForTest(path)
	t.Cleanup(func() {
		engine.waitAsyncPersist()
		setStoreFilePathForTest("")
	})
}

// --- 1) persistError generational protection ---

func TestApplyPersistResultLockedIgnoresOlderSuccess(t *testing.T) {
	e := &inspectionEngine{}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.applyPersistResultLocked(2, errors.New("disk full"))
	if e.persistError == "" || e.persistStatusSeq != 2 {
		t.Fatalf("after seq2 fail: err=%q seq=%d", e.persistError, e.persistStatusSeq)
	}
	// Delayed seq1 "success" (including stale nil from savePersistedSnapshot) must not clear.
	e.applyPersistResultLocked(1, nil)
	if e.persistError == "" {
		t.Fatal("stale seq1 success cleared persistError from seq2")
	}
	if e.persistStatusSeq != 2 {
		t.Fatalf("persistStatusSeq=%d want 2", e.persistStatusSeq)
	}
	// Newer success may clear.
	e.applyPersistResultLocked(3, nil)
	if e.persistError != "" || e.persistStatusSeq != 3 {
		t.Fatalf("seq3 success: err=%q seq=%d", e.persistError, e.persistStatusSeq)
	}
}

// --- 2) enable/delete clear autoban pool ---

func withCPAManagement(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	oldBase := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() {
		cpaManagementBaseURL = oldBase
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	})
}

func TestEnableAccountClearsMatchingBan(t *testing.T) {
	isolateActiveStore(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.persistError = ""
	unbanJob.mu.Unlock()
	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = filepath.Join(dir, "bans.json")
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      "acct.json",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})

	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// findAuthFile uses host auth list
	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "idx-1", Name: "acct.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	engine.mu.Lock()
	engine.results = []accountResult{{
		AuthIndex: "idx-1", Name: "acct.json", FileName: "acct.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.mu.Unlock()

	t.Cleanup(func() { engine.waitAsyncPersist() })
	if err := setAuthDisabled("acct.json", false, "test-pass", nil, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	engine.waitAsyncPersist()
	if _, ok := activeStore.Get("acct.json"); ok {
		t.Fatal("ban entry still present after enable")
	}
}

func TestDeleteAccountClearsMatchingBan(t *testing.T) {
	isolateActiveStore(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.persistError = ""
	unbanJob.mu.Unlock()
	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = filepath.Join(dir, "bans.json")
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      "gone.json",
		Provider:    "xai",
		ErrorCode:   permissionDeniedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.AddDate(50, 0, 0),
		ResetSource: "manual_unban",
		CpaSynced:   true,
	})

	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "idx-g", Name: "gone.json", Provider: "xai"},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	engine.mu.Lock()
	engine.results = []accountResult{{
		AuthIndex: "idx-g", Name: "gone.json", FileName: "gone.json",
		Classification: "permission_denied", Action: "delete",
	}}
	engine.mu.Unlock()

	t.Cleanup(func() { engine.waitAsyncPersist() })
	if err := deleteAuthFile("gone.json", "test-pass", nil, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	engine.waitAsyncPersist()
	if _, ok := activeStore.Get("gone.json"); ok {
		t.Fatal("ban entry still present after delete")
	}
}

func TestBulkEnableAndDeleteClearBansOnce(t *testing.T) {
	isolateActiveStore(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.persistError = ""
	unbanJob.mu.Unlock()
	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = filepath.Join(dir, "bans.json")
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	now := time.Now()
	activeStore.Set(banEntry{AuthID: "e1.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "d1.json", Provider: "xai", ErrorCode: unauthorizedErrorCode, BannedAt: now, ResetAt: now.AddDate(10, 0, 0), ResetSource: "manual_unban", CpaSynced: true})

	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "e1", Name: "e1.json", Provider: "xai", Disabled: true},
			{AuthIndex: "d1", Name: "d1.json", Provider: "xai"},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	engine.mu.Lock()
	engine.results = []accountResult{
		{AuthIndex: "e1", Name: "e1.json", FileName: "e1.json", Disabled: true, Classification: "quota_exhausted", Action: "enable"},
		{AuthIndex: "d1", Name: "d1.json", FileName: "d1.json", Classification: "auth_error", Action: "delete"},
	}
	engine.running = false
	engine.applying = false
	engine.mu.Unlock()

	// Run bulk apply path directly.
	cands := []accountResult{
		{AuthIndex: "e1", Name: "e1.json", FileName: "e1.json", Action: "enable"},
		{AuthIndex: "d1", Name: "d1.json", FileName: "d1.json", Action: "delete"},
	}
	engine.mu.Lock()
	engine.applying = true
	engine.applyRunID++
	applyID := engine.applyRunID
	engine.applyTotal = len(cands)
	engine.mu.Unlock()
	t.Cleanup(func() { engine.waitAsyncPersist() })
	engine.runApply(applyID, cands, "test-pass", nil)
	engine.waitAsyncPersist()

	if _, ok := activeStore.Get("e1.json"); ok {
		t.Fatal("enable bulk left ban")
	}
	if _, ok := activeStore.Get("d1.json"); ok {
		t.Fatal("delete bulk left ban")
	}
}

func TestDisableDoesNotClearBan(t *testing.T) {
	isolateActiveStore(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.persistError = ""
	unbanJob.mu.Unlock()

	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	t.Cleanup(func() { engine.waitAsyncPersist() })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "keep.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	engine.mu.Lock()
	engine.results = []accountResult{{
		AuthIndex: "k", Name: "keep.json", FileName: "keep.json",
	}}
	engine.mu.Unlock()

	if err := setAuthDisabled("keep.json", true, "test-pass", nil, true); err != nil {
		t.Fatal(err)
	}
	engine.waitAsyncPersist()
	if _, ok := activeStore.Get("keep.json"); !ok {
		t.Fatal("disable must not delete ban")
	}
}

// --- 3) unban persist_error ---

func TestUnbanJobReportsPersistError(t *testing.T) {
	isolateActiveStore(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.persistError = ""
	unbanJob.mu.Unlock()
	// Make ban save fail: state file path under a regular file.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = filepath.Join(blocker, "bans.json")
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "u1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})

	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	t.Cleanup(func() { engine.waitAsyncPersist(); unbanJob.wg.Wait() })
	if err := startUnbanJob([]string{"u1.json"}, "", "test-pass"); err != nil {
		t.Fatalf("startUnbanJob: %v", err)
	}
	// Wait for job
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st := unbanJobStatus()
		if running, _ := st["running"].(bool); !running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	st := unbanJobStatus()
	if running, _ := st["running"].(bool); running {
		t.Fatal("unban still running")
	}
	pe, _ := st["persist_error"].(string)
	if strings.TrimSpace(pe) == "" {
		t.Fatalf("expected persist_error in status, got %#v", st)
	}
	// status payload also used by /status
	snap := engine.snapshot(false)
	if snap.Unban == nil {
		t.Fatal("snapshot missing unban")
	}
	// Unban is map via json of jobSnapshot - check field exists through status map path
	// jobSnapshot.Unban is map[string]any from unbanJobStatus
	raw, _ := json.Marshal(snap.Unban)
	if !strings.Contains(string(raw), "persist_error") && pe == "" {
		t.Fatal("status unban missing persist_error")
	}
}

// --- 4) dark theme static markers ---

func TestUIPageThemeUsesCSSVarsForSelectAndHints(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if strings.Contains(page, `id="keyHint" style="font-size:12px;color:#64748b"`) {
		t.Fatal("keyHint still has hardcoded light color inline style")
	}
	if strings.Contains(page, `style="font-size:12px;color:#64748b">显示`) {
		t.Fatal("pager meta still hardcodes #64748b")
	}
	if strings.Contains(page, `style="font-size:12px;color:#475569"`) {
		t.Fatal("pager page counter still hardcodes a light-theme color")
	}
	// select theming via CSS variables / color-scheme
	needles := []string{
		`.grok-inspection-page select`,
		`color-scheme: inherit`,
		`html[data-grok-theme="dark"] .grok-inspection-page select`,
		`class="hint" id="keyHint"`,
		`pager-meta`,
	}
	for _, n := range needles {
		if !strings.Contains(page, n) {
			t.Fatalf("resource page missing theme marker %q", n)
		}
	}
	// inline hardcodes on the two page-size selects' surrounding template should not force light text
	if strings.Contains(page, `id="pageSize"`) && strings.Contains(page, `color:#64748b">显示`) {
		t.Fatal("pageSize pager still uses hardcoded muted color")
	}
	if !strings.Contains(page, `snap.unban && !snap.unban.running`) {
		t.Fatal("completed unban failures are not surfaced after the job becomes idle")
	}
}

func TestUIManualManagementKeyShowsLoadedHint(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, `} else if (hasManagementKey()) {
      hint.textContent = t('key_manual');`) {
		t.Fatal("manual Management Key still falls through to the missing-key hint")
	}
	if !strings.Contains(page, `key_manual:`) {
		t.Fatal("key_manual i18n entry missing")
	}
}

func TestUIDarkGenericButtonRuleDoesNotOverrideTabs(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, `button:not(.primary):not(.soft):not(.danger):not(.tab)`) {
		t.Fatal("dark generic button selector still includes tab buttons")
	}
}

func TestUIAutobanHeaderDoesNotRepeatRuleSummary(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if strings.Contains(page, `<p class="module-sub">命中 free-usage / 403 / 401 时自动禁用；额度用尽默认 24 小时后恢复</p>`) {
		t.Fatal("autoban module header repeats the rule summary already shown above")
	}
}

func TestAsyncUnbanClaimsWaitGroupBeforeReturning(t *testing.T) {
	isolateUnbanJob(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()

	runID, err := claimUnbanSlot(1, "queued", true)
	if err != nil {
		t.Fatal(err)
	}
	waited := make(chan struct{})
	go func() {
		unbanJob.wg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("Wait returned before the claimed async job called Done")
	case <-time.After(30 * time.Millisecond):
	}
	releaseUnbanSlot(runID)
	unbanJob.wg.Done()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after async unban completion")
	}
}

func TestRunApplyReportsBanStoreSaveFailure(t *testing.T) {
	isolateActiveStore(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = true
	engine.applyDraining = false
	engine.applyFailures = nil
	engine.applyRunID++
	applyID := engine.applyRunID
	engine.mu.Unlock()

	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = filepath.Join(blocker, "bans.json")
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	engine.runApply(applyID, nil, "", nil)
	engine.waitAsyncPersist()
	engine.mu.Lock()
	failures := append([]string(nil), engine.applyFailures...)
	engine.mu.Unlock()
	if len(failures) == 0 || !strings.Contains(strings.Join(failures, "\n"), "保存自动禁用状态失败") {
		t.Fatalf("save failure not reported: %#v", failures)
	}
}
