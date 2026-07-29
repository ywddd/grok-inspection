package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func seedScheduleEngineResults(t *testing.T, results []accountResult, cfg persistedInspectionSchedule) {
	t.Helper()
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)
	isolateActiveStore(t)
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	oldSchedule := engine.schedule
	engine.results = append([]accountResult(nil), results...)
	engine.schedule = cfg
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.stopped = false
	engine.shuttingDown = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.schedule = oldSchedule
		engine.running = false
		engine.applying = false
		engine.applyDraining = false
		engine.mu.Unlock()
	})
}

func withScheduleMgmtPassword(t *testing.T) {
	t.Helper()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPass) })
}

func TestScheduledQuotaExhaustedTargetsRequireExact429Signal(t *testing.T) {
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "exact", FileName: "exact.json", HTTPStatus: 429, Classification: "quota_exhausted", ErrorCode: exhaustedErrorCode},
		{AuthIndex: "bare", FileName: "bare.json", HTTPStatus: 429, Classification: "probe_error", ErrorCode: ""},
		{AuthIndex: "text-only", FileName: "text.json", HTTPStatus: 429, Classification: "quota_exhausted", ErrorCode: ""},
		{AuthIndex: "substring", FileName: "sub.json", HTTPStatus: 429, Classification: "quota_exhausted", ErrorCode: "x-" + exhaustedErrorCode},
		{AuthIndex: "wrong-class", FileName: "wc.json", HTTPStatus: 429, Classification: "probe_error", ErrorCode: exhaustedErrorCode},
		{AuthIndex: "wrong-status", FileName: "ws.json", HTTPStatus: 403, Classification: "quota_exhausted", ErrorCode: exhaustedErrorCode},
		{AuthIndex: "already", FileName: "already.json", HTTPStatus: 429, Classification: "quota_exhausted", ErrorCode: exhaustedErrorCode, Disabled: true},
	}, defaultInspectionSchedule())

	got := scheduledQuotaExhaustedTargets(scheduled403Disable, nil)
	if len(got) != 1 || got[0] != "exact" {
		t.Fatalf("targets=%v want [exact]", got)
	}
}

func TestScheduledUnauthorizedTargetsRequireExact401Signal(t *testing.T) {
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "exact", FileName: "exact.json", HTTPStatus: 401, Classification: "reauth", ErrorCode: unauthorizedErrorCode},
		{AuthIndex: "bare", FileName: "bare.json", HTTPStatus: 401, Classification: "reauth", ErrorCode: ""},
		{AuthIndex: "wrong-code", FileName: "wc.json", HTTPStatus: 401, Classification: "reauth", ErrorCode: "permission-denied"},
		{AuthIndex: "wrong-class", FileName: "wcl.json", HTTPStatus: 401, Classification: "probe_error", ErrorCode: unauthorizedErrorCode},
		{AuthIndex: "wrong-status", FileName: "ws.json", HTTPStatus: 403, Classification: "reauth", ErrorCode: unauthorizedErrorCode},
	}, defaultInspectionSchedule())

	got := scheduledUnauthorizedTargets(scheduled403Disable, nil)
	if len(got) != 1 || got[0] != "exact" {
		t.Fatalf("targets=%v want [exact]", got)
	}
}

func TestScheduledPermissionDeniedTargetsRejectSubstringCode(t *testing.T) {
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "exact", FileName: "exact.json", HTTPStatus: 403, Classification: "permission_denied", ErrorCode: permissionDeniedErrorCode},
		{AuthIndex: "contains", FileName: "contains.json", HTTPStatus: 403, Classification: "permission_denied", ErrorCode: "x-" + permissionDeniedErrorCode + "-y"},
		{AuthIndex: "prefix", FileName: "prefix.json", HTTPStatus: 403, Classification: "permission_denied", ErrorCode: permissionDeniedErrorCode + "-extra"},
	}, defaultInspectionSchedule())

	got := scheduledPermissionDeniedTargets(scheduled403Disable, nil)
	if len(got) != 1 || got[0] != "exact" {
		t.Fatalf("targets=%v want [exact] (substring must not match)", got)
	}
}

func TestScheduledTargetsIgnoreNonActionableStatuses(t *testing.T) {
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "n404", FileName: "n404.json", HTTPStatus: 404, Classification: "model_unavailable", ErrorCode: "not-found"},
		{AuthIndex: "n501", FileName: "n501.json", HTTPStatus: 501, Classification: "probe_error", ErrorCode: "not-implemented"},
		{AuthIndex: "net", FileName: "net.json", HTTPStatus: 0, Classification: "probe_error", ErrorCode: "", ErrorMessage: "tls handshake timeout"},
		{AuthIndex: "proxy", FileName: "proxy.json", HTTPStatus: 0, Classification: "probe_error", ErrorMessage: "proxy connect failed"},
	}, defaultInspectionSchedule())

	if got := scheduledQuotaExhaustedTargets(scheduled403Disable, nil); len(got) != 0 {
		t.Fatalf("429 targets=%v want none", got)
	}
	if got := scheduledUnauthorizedTargets(scheduled403Disable, nil); len(got) != 0 {
		t.Fatalf("401 targets=%v want none", got)
	}
	if got := scheduledPermissionDeniedTargets(scheduled403Disable, nil); len(got) != 0 {
		t.Fatalf("403 targets=%v want none", got)
	}
	if got := scheduledSpendingLimitTargets(scheduled402Disable, nil); len(got) != 0 {
		t.Fatalf("402 targets=%v want none", got)
	}
}

func TestScheduledHealthyRecoverTargetsRequireQuotaBanAndThisRun(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "ok-quota", FileName: "ok-quota.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
		{AuthIndex: "ok-manual", FileName: "ok-manual.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
		{AuthIndex: "ok-401", FileName: "ok-401.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
		{AuthIndex: "ok-402", FileName: "ok-402.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
		{AuthIndex: "ok-403", FileName: "ok-403.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
		{AuthIndex: "enabled", FileName: "enabled.json", HTTPStatus: 200, Classification: "healthy", Disabled: false},
		{AuthIndex: "not-healthy", FileName: "nh.json", HTTPStatus: 429, Classification: "quota_exhausted", ErrorCode: exhaustedErrorCode, Disabled: true},
		{AuthIndex: "out-of-scope", FileName: "oos.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
	}, defaultInspectionSchedule())

	activeStore.Set(banEntry{AuthID: "ok-quota.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now.Add(-24 * time.Hour), ResetAt: time.Now().Add(-time.Minute), ResetSource: "local_plus_fallback", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "ok-manual.json", Provider: "xai", ErrorCode: manualInspectionBanErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: manualInspectionBanResetSource, CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "ok-401.json", Provider: "xai", ErrorCode: unauthorizedErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: "manual_unban", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "ok-402.json", Provider: "xai", ErrorCode: spendingLimitErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: "manual_unban", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "ok-403.json", Provider: "xai", ErrorCode: permissionDeniedErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: "manual_unban", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "oos.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now, ResetAt: now.Add(24 * time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true})

	scope := map[string]struct{}{
		probedKeyForIdentity("ok-quota", "ok-quota.json"):   {},
		probedKeyForIdentity("ok-manual", "ok-manual.json"): {},
		probedKeyForIdentity("ok-401", "ok-401.json"):       {},
		probedKeyForIdentity("ok-402", "ok-402.json"):       {},
		probedKeyForIdentity("ok-403", "ok-403.json"):       {},
		probedKeyForIdentity("enabled", "enabled.json"):     {},
		probedKeyForIdentity("not-healthy", "nh.json"):      {},
	}
	got := scheduledHealthyRecoverTargets(scope)
	if len(got) != 1 || got[0] != "ok-quota" {
		t.Fatalf("recover targets=%v want [ok-quota]", got)
	}
}

func TestScheduledHealthyRecoverTargetsWaitForQuotaCooldown(t *testing.T) {
	now := time.Now()
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "cooling", FileName: "cooling.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
		{AuthIndex: "cooled", FileName: "cooled.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
	}, defaultInspectionSchedule())

	activeStore.Set(banEntry{AuthID: "cooling.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "cooled.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now.Add(-2 * time.Hour), ResetAt: now.Add(-time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true})

	got := scheduledHealthyRecoverTargets(nil)
	if len(got) != 1 || got[0] != "cooled" {
		t.Fatalf("recover targets=%v want [cooled]; active quota cooldown must not be bypassed by one healthy probe", got)
	}
}

func TestScheduledHealthyRecoverRejectsMixedAliasBans(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "mix", FileName: "mix.json", Name: "mix-display", Email: "mix@x.ai", HTTPStatus: 200, Classification: "healthy", Disabled: true},
		{AuthIndex: "pure", FileName: "pure.json", HTTPStatus: 200, Classification: "healthy", Disabled: true},
	}, defaultInspectionSchedule())

	// Same physical account, two ban rows via aliases: exhausted + unauthorized.
	activeStore.Set(banEntry{AuthID: "mix.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now, ResetAt: now.Add(24 * time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "mix", Provider: "xai", ErrorCode: unauthorizedErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: "manual_unban", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "pure.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now.Add(-24 * time.Hour), ResetAt: time.Now().Add(-time.Minute), ResetSource: "local_plus_fallback", CpaSynced: true})

	if accountHasExactQuotaExhaustionBan(accountResult{AuthIndex: "mix", FileName: "mix.json", Name: "mix-display", Email: "mix@x.ai"}) {
		t.Fatal("mixed exhausted+unauthorized aliases must not be recoverable")
	}
	if !accountHasExactQuotaExhaustionBan(accountResult{AuthIndex: "pure", FileName: "pure.json"}) {
		t.Fatal("pure exhausted ban must remain recoverable")
	}

	got := scheduledHealthyRecoverTargets(nil)
	if len(got) != 1 || got[0] != "pure" {
		t.Fatalf("recover targets=%v want [pure] (mixed account excluded)", got)
	}
}

func TestScheduledEnableSuccessRequiresNoMatchingBan(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	seedScheduleEngineResults(t, []accountResult{
		{AuthIndex: "cas-fail", FileName: "cas-fail.json", Disabled: false, Classification: "healthy"},
		{AuthIndex: "ok", FileName: "ok.json", Disabled: false, Classification: "healthy"},
		{AuthIndex: "still-off", FileName: "still-off.json", Disabled: true, Classification: "healthy"},
	}, defaultInspectionSchedule())

	// Simulate enable PATCH + result Disabled=false, but concurrent ban re-disable failed: ban remains.
	activeStore.Set(banEntry{AuthID: "cas-fail.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: false})

	if got := scheduledActionSuccessCount([]string{"cas-fail", "ok", "still-off"}, "enable"); got != 1 {
		t.Fatalf("enable success=%d want 1 (only ok with no ban and enabled)", got)
	}

	var recovered, deleted, failed int
	recordScheduledActionProgress([]string{"cas-fail", "ok", "still-off"}, "enable", &recovered, &deleted, &failed)
	if recovered != 1 || failed != 2 || deleted != 0 {
		t.Fatalf("progress recovered=%d failed=%d deleted=%d want recovered=1 failed=2", recovered, failed, deleted)
	}
}
func TestDefaultScheduleDisablesHealthyAutoRecover(t *testing.T) {
	cfg := defaultInspectionSchedule()
	if cfg.AutoRecoverHealthy {
		t.Fatal("AutoRecoverHealthy must default false")
	}
	got := normalizePersistedInspectionSchedule(persistedInspectionSchedule{})
	if got.AutoRecoverHealthy {
		t.Fatal("normalize must keep AutoRecoverHealthy false by default")
	}
}

func TestUpdateInspectionSchedulePersistsAutoRecoverHealthy(t *testing.T) {
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
	resetStoreIOForTest()
	engine.mu.Lock()
	old := engine.schedule
	engine.schedule = defaultInspectionSchedule()
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.waitAsyncPersist()
		engine.mu.Lock()
		engine.schedule = old
		engine.mu.Unlock()
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})

	on := true
	cfg, err := updateInspectionSchedule(inspectionScheduleUpdate{AutoRecoverHealthy: &on})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AutoRecoverHealthy {
		t.Fatalf("saved cfg=%+v", cfg)
	}
	raw, err := os.ReadFile(scheduleFilePath())
	if err != nil {
		t.Fatal(err)
	}
	var disk persistedInspectionSchedule
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if !disk.AutoRecoverHealthy {
		t.Fatalf("disk=%+v", disk)
	}
	status := inspectionScheduleStatus()
	if status["auto_recover_healthy"] != true {
		t.Fatalf("status auto_recover_healthy=%v", status["auto_recover_healthy"])
	}
}

func TestScheduledInspectionDisposesExact429And401Only(t *testing.T) {
	withScheduleMgmtPassword(t)
	useResultsStorePath(t, filepath.Join(t.TempDir(), "results.json"))

	var disabledNames []string
	var deleteHits atomic.Int32
	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteHits.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.Method == http.MethodPatch {
			var body struct {
				Name     string `json:"name"`
				Disabled bool   `json:"disabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Disabled {
				disabledNames = append(disabledNames, body.Name)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "q1", Name: "q1.json", Provider: "xai", Disabled: false},
			{AuthIndex: "bare429", Name: "bare429.json", Provider: "xai", Disabled: false},
			{AuthIndex: "a401", Name: "a401.json", Provider: "xai", Disabled: false},
			{AuthIndex: "n404", Name: "n404.json", Provider: "xai", Disabled: false},
			{AuthIndex: "n501", Name: "n501.json", Provider: "xai", Disabled: false},
			{AuthIndex: "net", Name: "net.json", Provider: "xai", Disabled: false},
			{AuthIndex: "sub403", Name: "sub403.json", Provider: "xai", Disabled: false},
			{AuthIndex: "exact403", Name: "exact403.json", Provider: "xai", Disabled: false},
		}}, nil
	}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		base := accountResult{AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name, Disabled: file.Disabled}
		switch file.AuthIndex {
		case "q1":
			base.HTTPStatus = 429
			base.Classification = "quota_exhausted"
			base.ErrorCode = exhaustedErrorCode
			base.Action = "disable"
		case "bare429":
			base.HTTPStatus = 429
			base.Classification = "probe_error"
			base.Action = "keep"
		case "a401":
			base.HTTPStatus = 401
			base.Classification = "reauth"
			base.ErrorCode = unauthorizedErrorCode
			base.Action = "delete"
		case "n404":
			base.HTTPStatus = 404
			base.Classification = "model_unavailable"
			base.Action = "keep"
		case "n501":
			base.HTTPStatus = 501
			base.Classification = "probe_error"
			base.Action = "keep"
		case "net":
			base.HTTPStatus = 0
			base.Classification = "probe_error"
			base.ErrorMessage = "proxy connect failed"
			base.Action = "keep"
		case "sub403":
			base.HTTPStatus = 403
			base.Classification = "permission_denied"
			base.ErrorCode = "x-permission-denied-y"
			base.Action = "disable"
		case "exact403":
			base.HTTPStatus = 403
			base.Classification = "permission_denied"
			base.ErrorCode = permissionDeniedErrorCode
			base.Action = "disable"
		}
		return base
	}
	t.Cleanup(func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	})

	seedScheduleEngineResults(t, nil, persistedInspectionSchedule{
		Enabled:                true,
		IntervalMinutes:        60,
		Workers:                2,
		PermissionDeniedAction: scheduled403Disable,
		SpendingLimitAction:    scheduled402Disable,
	})

	runScheduledInspection(inspectionScheduleSnapshot())

	wantDisabled := map[string]bool{"q1.json": true, "a401.json": true, "exact403.json": true}
	if len(disabledNames) != 3 {
		t.Fatalf("disabledNames=%v want 3 exact targets", disabledNames)
	}
	for _, name := range disabledNames {
		if !wantDisabled[name] {
			t.Fatalf("unexpected disable %q in %v", name, disabledNames)
		}
	}
	if deleteHits.Load() != 0 {
		t.Fatalf("401 must disable not delete; deleteHits=%d", deleteHits.Load())
	}

	entry, ok := activeStore.Get("q1.json")
	if !ok || entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("q1 ban=%+v ok=%v", entry, ok)
	}
	if entry.ResetSource == manualInspectionBanResetSource || entry.ResetSource == "manual_unban" {
		t.Fatalf("429 ResetSource=%q must be timed auto-restore, not manual", entry.ResetSource)
	}
	window := entry.ResetAt.Sub(entry.BannedAt)
	if window < 20*time.Hour || window > 28*time.Hour {
		t.Fatalf("429 ResetAt window=%s (BannedAt=%s ResetAt=%s) want ~24h", window, entry.BannedAt, entry.ResetAt)
	}
	entry401, ok := activeStore.Get("a401.json")
	if !ok || entry401.ErrorCode != unauthorizedErrorCode {
		t.Fatalf("401 ban=%+v ok=%v", entry401, ok)
	}
	if entry401.ResetSource != "manual_unban" && entry401.ResetSource != manualInspectionBanResetSource {
		t.Fatalf("401 ResetSource=%q want manual_unban/manual-disabled", entry401.ResetSource)
	}
	if !entry401.ResetAt.After(time.Now().AddDate(50, 0, 0)) {
		t.Fatalf("401 ResetAt=%s must be permanent manual (>=50y)", entry401.ResetAt)
	}

	sch := inspectionScheduleSnapshot()
	if sch.LastDisabled429 != 1 {
		t.Fatalf("LastDisabled429=%d want 1 (schedule=%+v)", sch.LastDisabled429, sch)
	}
	if sch.LastDisabled401 != 1 {
		t.Fatalf("LastDisabled401=%d want 1", sch.LastDisabled401)
	}
	if sch.LastDisabled403 != 1 {
		t.Fatalf("LastDisabled403=%d want 1", sch.LastDisabled403)
	}
	if sch.LastRecovered != 0 {
		t.Fatalf("recover default off, LastRecovered=%d", sch.LastRecovered)
	}
}

func TestScheduledInspectionHealthyRecoverOnlyQuotaWhenEnabled(t *testing.T) {
	withScheduleMgmtPassword(t)
	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	withQuotaBanConfig(t, dir, 24)

	var enableNames []string
	var disableNames []string
	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body struct {
				Name     string `json:"name"`
				Disabled bool   `json:"disabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Disabled {
				disableNames = append(disableNames, body.Name)
			} else {
				enableNames = append(enableNames, body.Name)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "quota", Name: "quota.json", Provider: "xai", Disabled: true},
			{AuthIndex: "manual", Name: "manual.json", Provider: "xai", Disabled: true},
			{AuthIndex: "u401", Name: "u401.json", Provider: "xai", Disabled: true},
			{AuthIndex: "s402", Name: "s402.json", Provider: "xai", Disabled: true},
			{AuthIndex: "p403", Name: "p403.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		return accountResult{
			AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name,
			Disabled: true, HTTPStatus: 200, Classification: "healthy", Action: "enable",
		}
	}
	t.Cleanup(func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	})

	seedScheduleEngineResults(t, []accountResult{{
		AuthIndex: "notprobed", Name: "notprobed.json", FileName: "notprobed.json",
		Disabled: true, HTTPStatus: 200, Classification: "healthy", Action: "enable",
	}}, persistedInspectionSchedule{
		Enabled:                true,
		IntervalMinutes:        60,
		Workers:                2,
		IncludeDisabled:        true,
		AutoRecoverHealthy:     true,
		PermissionDeniedAction: scheduled403Disable,
		SpendingLimitAction:    scheduled402Disable,
	})

	now := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	activeStore.Set(banEntry{AuthID: "quota.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now.Add(-24 * time.Hour), ResetAt: now.Add(-time.Minute), ResetSource: "local_plus_fallback", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "manual.json", Provider: "xai", ErrorCode: manualInspectionBanErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: manualInspectionBanResetSource, CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "u401.json", Provider: "xai", ErrorCode: unauthorizedErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: "manual_unban", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "s402.json", Provider: "xai", ErrorCode: spendingLimitErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: "manual_unban", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "p403.json", Provider: "xai", ErrorCode: permissionDeniedErrorCode, BannedAt: now, ResetAt: now.AddDate(100, 0, 0), ResetSource: "manual_unban", CpaSynced: true})
	activeStore.Set(banEntry{AuthID: "notprobed.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now, ResetAt: now.Add(24 * time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true})

	runScheduledInspection(inspectionScheduleSnapshot())

	if len(enableNames) != 1 || enableNames[0] != "quota.json" {
		t.Fatalf("enableNames=%v want [quota.json]", enableNames)
	}
	if len(disableNames) != 0 {
		t.Fatalf("unexpected disables=%v", disableNames)
	}
	if _, ok := activeStore.Get("quota.json"); ok {
		t.Fatal("quota ban should be cleared after healthy recover")
	}
	for _, keep := range []string{"manual.json", "u401.json", "s402.json", "p403.json", "notprobed.json"} {
		if _, ok := activeStore.Get(keep); !ok {
			t.Fatalf("ban %s must remain", keep)
		}
	}
	sch := inspectionScheduleSnapshot()
	if sch.LastRecovered != 1 {
		t.Fatalf("LastRecovered=%d want 1 schedule=%+v", sch.LastRecovered, sch)
	}
}
func TestScheduledInspectionHealthyRecoverOffByDefault(t *testing.T) {
	withScheduleMgmtPassword(t)
	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	withQuotaBanConfig(t, dir, 24)

	var enableHits atomic.Int32
	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var body struct {
				Disabled bool `json:"disabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body.Disabled {
				enableHits.Add(1)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "quota", Name: "quota.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		return accountResult{
			AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name,
			Disabled: true, HTTPStatus: 200, Classification: "healthy", Action: "enable",
		}
	}
	t.Cleanup(func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	})

	seedScheduleEngineResults(t, nil, persistedInspectionSchedule{
		Enabled:                true,
		IntervalMinutes:        60,
		Workers:                1,
		IncludeDisabled:        true,
		PermissionDeniedAction: scheduled403Disable,
		SpendingLimitAction:    scheduled402Disable,
	})

	now := time.Now()
	activeStore.Set(banEntry{AuthID: "quota.json", Provider: "xai", ErrorCode: exhaustedErrorCode, BannedAt: now, ResetAt: now.Add(24 * time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true})

	runScheduledInspection(inspectionScheduleSnapshot())
	if enableHits.Load() != 0 {
		t.Fatalf("default off must not recover, enableHits=%d", enableHits.Load())
	}
	if _, ok := activeStore.Get("quota.json"); !ok {
		t.Fatal("quota ban must remain when recover disabled")
	}
	sch := inspectionScheduleSnapshot()
	if sch.LastRecovered != 0 {
		t.Fatalf("LastRecovered=%d want 0", sch.LastRecovered)
	}
}

func TestScheduledCompletionStatusUsesErrorsEvenWhenFailedTotalZero(t *testing.T) {
	// CPA mutation OK but ban-state persist failed: counters show success, errors still set.
	if got := scheduledCompletionStatus(1, 0, []string{"updated in CPA but failed to persist ban state: disk full"}); got != "completed_with_errors" {
		t.Fatalf("status=%q want completed_with_errors when matched>0 and errors non-empty", got)
	}
	if got := scheduledCompletionStatus(2, 1, nil); got != "completed_with_errors" {
		t.Fatalf("status=%q want completed_with_errors when failedTotal>0", got)
	}
	if got := scheduledCompletionStatus(1, 0, nil); got != "completed" {
		t.Fatalf("status=%q want completed on clean success", got)
	}
	if got := scheduledCompletionStatus(0, 0, []string{"stale noise"}); got != "completed" {
		t.Fatalf("status=%q want completed when matched==0 (no actions)", got)
	}
	if got := scheduledCompletionStatus(1, 0, []string{"", "  "}); got != "completed" {
		t.Fatalf("status=%q want completed when error strings are blank", got)
	}
}
func TestScheduleUIAndI18NContractForSafeDispose(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, frag := range []string{
		`id="scheduleAutoRecoverHealthy"`,
		`data-i18n="schedule_auto_recover_healthy"`,
		`auto_recover_healthy`,
		`last_disabled_429`,
		`last_disabled_401`,
		`last_recovered`,
		`schedule_counts_429`,
		`schedule_counts_401`,
		`schedule_recovered_count`,
	} {
		if !strings.Contains(page, frag) {
			t.Fatalf("UI/page missing %q", frag)
		}
	}
	zh := extractI18NPack(page, "zh")
	en := extractI18NPack(page, "en")
	for _, key := range []string{
		"schedule_auto_recover_healthy",
		"schedule_counts_429",
		"schedule_counts_401",
		"schedule_recovered_count",
	} {
		if strings.TrimSpace(zh[key]) == "" {
			t.Fatalf("zh %s empty", key)
		}
		if strings.TrimSpace(en[key]) == "" {
			t.Fatalf("en %s empty", key)
		}
	}
	if zh["schedule_auto_recover_healthy"] == en["schedule_auto_recover_healthy"] {
		t.Fatal("zh/en auto-recover labels must differ")
	}
	// Status text prefixes recovered with " · "; labels must not already start with comma/顿号.
	if strings.HasPrefix(strings.TrimSpace(zh["schedule_recovered_count"]), "，") || strings.Contains(zh["schedule_recovered_count"], "，恢复") {
		t.Fatalf("zh schedule_recovered_count=%q must not lead with comma", zh["schedule_recovered_count"])
	}
	if strings.HasPrefix(strings.TrimSpace(en["schedule_recovered_count"]), ",") {
		t.Fatalf("en schedule_recovered_count=%q must not lead with comma", en["schedule_recovered_count"])
	}
	if !strings.HasPrefix(strings.TrimSpace(zh["schedule_recovered_count"]), "恢复") {
		t.Fatalf("zh schedule_recovered_count=%q want 恢复…", zh["schedule_recovered_count"])
	}
	if !strings.HasPrefix(strings.TrimSpace(en["schedule_recovered_count"]), "Recovered") {
		t.Fatalf("en schedule_recovered_count=%q want Recovered…", en["schedule_recovered_count"])
	}
}
