package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeInspectionScheduleInterval(t *testing.T) {
	got, err := normalizeInspectionScheduleInterval(0)
	if err != nil || got != defaultInspectionScheduleIntervalMinutes {
		t.Fatalf("zero interval: got=%d err=%v", got, err)
	}
	got, err = normalizeInspectionScheduleInterval(1)
	if err != nil || got != 1 {
		t.Fatalf("one-minute interval: got=%d err=%v", got, err)
	}
	if _, err := normalizeInspectionScheduleInterval(-1); err == nil {
		t.Fatal("expected interval below minimum to fail")
	}
	if _, err := normalizeInspectionScheduleInterval(maxInspectionScheduleIntervalMinutes + 1); err == nil {
		t.Fatal("expected interval above maximum to fail")
	}
	got, err = normalizeInspectionScheduleInterval(30)
	if err != nil || got != 30 {
		t.Fatalf("valid interval: got=%d err=%v", got, err)
	}
}

func TestUpdateInspectionScheduleSavesEnabledOneMinute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	setStoreFilePathForTest(path)
	resetStoreIOForTest()

	engine.mu.Lock()
	oldSchedule := engine.schedule
	engine.schedule = defaultInspectionSchedule()
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.waitAsyncPersist()
		engine.mu.Lock()
		engine.schedule = oldSchedule
		engine.mu.Unlock()
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})

	enabled := true
	interval := 1
	cfg, err := updateInspectionSchedule(inspectionScheduleUpdate{
		Enabled:         &enabled,
		IntervalMinutes: &interval,
	})
	if err != nil {
		t.Fatalf("updateInspectionSchedule() error = %v", err)
	}
	if !cfg.Enabled || cfg.IntervalMinutes != 1 {
		t.Fatalf("saved schedule = %+v", cfg)
	}

	engine.waitAsyncPersist()
	snap, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatalf("loadPersistedSnapshot() error = %v", err)
	}
	if !snap.Schedule.Enabled || snap.Schedule.IntervalMinutes != 1 {
		t.Fatalf("persisted schedule = %+v", snap.Schedule)
	}
}

func TestInspectionScheduleDue(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	if !inspectionScheduleDue(now, time.Time{}) {
		t.Fatal("empty next-run time should be due")
	}
	if !inspectionScheduleDue(now, now) {
		t.Fatal("equal next-run time should be due")
	}
	if inspectionScheduleDue(now, now.Add(time.Second)) {
		t.Fatal("future next-run time should not be due")
	}
}

func TestNextInspectionScheduleRunUsesConfiguredInterval(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	got := nextInspectionScheduleRun(now, 30)
	want := now.Add(30 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("next run=%s want=%s", got, want)
	}
}

func TestScheduledInspectionRequestIsFullInspectionOnly(t *testing.T) {
	cfg := persistedInspectionSchedule{
		Workers:         12,
		IncludeDisabled: true,
		OnlyDisabled:    false,
	}
	req := scheduledInspectionRequest(cfg)
	if req.Workers != 12 || !req.IncludeDisabled || req.OnlyDisabled {
		t.Fatalf("request=%+v", req)
	}
	if req.Incremental || len(req.Classifications) != 0 {
		t.Fatalf("scheduled request must be a full inspection: %+v", req)
	}
}

func TestNormalizeScheduled403Action(t *testing.T) {
	if got, err := normalizeScheduled403Action(""); err != nil || got != scheduled403Disable {
		t.Fatalf("default action=%q err=%v", got, err)
	}
	if got, err := normalizeScheduled403Action("DELETE"); err != nil || got != scheduled403Delete {
		t.Fatalf("delete action=%q err=%v", got, err)
	}
	if _, err := normalizeScheduled403Action("enable"); err == nil {
		t.Fatal("unsupported scheduled 403 action must fail")
	}
}

func TestNormalizeScheduled402Action(t *testing.T) {
	if got, err := normalizeScheduled402Action(""); err != nil || got != scheduled402Disable {
		t.Fatalf("default action=%q err=%v", got, err)
	}
	if got, err := normalizeScheduled402Action("DELETE"); err != nil || got != scheduled402Delete {
		t.Fatalf("delete action=%q err=%v", got, err)
	}
	if _, err := normalizeScheduled402Action("enable"); err == nil {
		t.Fatal("unsupported scheduled 402 action must fail")
	}
}

func TestScheduledPermissionDeniedTargetsRequireExplicit403Code(t *testing.T) {
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "exact", HTTPStatus: 403, Classification: "permission_denied", ErrorCode: permissionDeniedErrorCode},
		{AuthIndex: "wrong-status", HTTPStatus: 429, Classification: "permission_denied", ErrorCode: permissionDeniedErrorCode},
		{AuthIndex: "wrong-code", HTTPStatus: 403, Classification: "permission_denied", ErrorCode: "cloudflare-denied"},
		{AuthIndex: "wrong-class", HTTPStatus: 403, Classification: "probe_error", ErrorCode: permissionDeniedErrorCode},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	got := scheduledPermissionDeniedTargets(scheduled403Disable)
	if len(got) != 1 || got[0] != "exact" {
		t.Fatalf("targets=%v want [exact]", got)
	}
}

func TestScheduledSpendingLimitTargetsRequireExplicit402Code(t *testing.T) {
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "exact", HTTPStatus: 402, Classification: "spending_limit", ErrorCode: spendingLimitErrorCode},
		{AuthIndex: "wrong-status", HTTPStatus: 403, Classification: "spending_limit", ErrorCode: spendingLimitErrorCode},
		{AuthIndex: "wrong-code", HTTPStatus: 402, Classification: "spending_limit", ErrorCode: "payment-required"},
		{AuthIndex: "wrong-class", HTTPStatus: 402, Classification: "probe_error", ErrorCode: spendingLimitErrorCode},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	got := scheduledSpendingLimitTargets(scheduled402Disable)
	if len(got) != 1 || got[0] != "exact" {
		t.Fatalf("targets=%v want [exact]", got)
	}
}

func TestScheduledDisablePreservesPermissionDeniedReason(t *testing.T) {
	e := &inspectionEngine{results: []accountResult{{
		AuthIndex:      "p1",
		FileName:       "p1.json",
		HTTPStatus:     403,
		Classification: "permission_denied",
		ErrorCode:      permissionDeniedErrorCode,
	}}}
	candidates, err := e.collectCandidates(applyRequest{
		AuthIndexes:  []string{"p1"},
		ForceAction:  scheduled403Disable,
		BanErrorCode: permissionDeniedErrorCode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].BanErrorCode != permissionDeniedErrorCode {
		t.Fatalf("candidates=%+v", candidates)
	}
}

func TestScheduledDisablePreservesSpendingLimitReason(t *testing.T) {
	e := &inspectionEngine{results: []accountResult{{
		AuthIndex:      "s1",
		FileName:       "s1.json",
		HTTPStatus:     402,
		Classification: "spending_limit",
		ErrorCode:      spendingLimitErrorCode,
	}}}
	candidates, err := e.collectCandidates(applyRequest{
		AuthIndexes:  []string{"s1"},
		ForceAction:  scheduled402Disable,
		BanErrorCode: spendingLimitErrorCode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].BanErrorCode != spendingLimitErrorCode {
		t.Fatalf("candidates=%+v", candidates)
	}
}

func TestScheduledDeleteSuccessCountKeepsFailures(t *testing.T) {
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "failed", FileName: "failed.json"},
		{AuthIndex: "unrelated", FileName: "other.json"},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	if got := scheduledActionSuccessCount([]string{"deleted", "failed"}, scheduled403Delete); got != 1 {
		t.Fatalf("deleted success count=%d want 1", got)
	}
}

func TestNormalizePersistedInspectionScheduleDefaults402Action(t *testing.T) {
	// Old results.json without spending_limit_action must default to disable.
	got := normalizePersistedInspectionSchedule(persistedInspectionSchedule{
		IntervalMinutes:        30,
		Workers:                8,
		PermissionDeniedAction: scheduled403Delete,
	})
	if got.SpendingLimitAction != scheduled402Disable {
		t.Fatalf("SpendingLimitAction=%q want %q", got.SpendingLimitAction, scheduled402Disable)
	}
	if got.PermissionDeniedAction != scheduled403Delete {
		t.Fatalf("PermissionDeniedAction=%q preserved, got %q", scheduled403Delete, got.PermissionDeniedAction)
	}
	if got.IntervalMinutes != 30 || got.Workers != 8 {
		t.Fatalf("other fields mutated: %+v", got)
	}
}

func TestScheduledSpendingLimitTargetsRequireExactCode(t *testing.T) {
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "exact", HTTPStatus: 402, Classification: "spending_limit", ErrorCode: spendingLimitErrorCode},
		{AuthIndex: "prefix", HTTPStatus: 402, Classification: "spending_limit", ErrorCode: "prefix-" + spendingLimitErrorCode},
		{AuthIndex: "contains", HTTPStatus: 402, Classification: "spending_limit", ErrorCode: "x-" + spendingLimitErrorCode + "-y"},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	got := scheduledSpendingLimitTargets(scheduled402Disable)
	if len(got) != 1 || got[0] != "exact" {
		t.Fatalf("targets=%v want [exact]", got)
	}
}

func TestScheduledDisableSuccessCountSupports402(t *testing.T) {
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "ok", FileName: "ok.json", Disabled: true},
		{AuthIndex: "fail", FileName: "fail.json", Disabled: false},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	if got := scheduledActionSuccessCount([]string{"ok", "fail"}, scheduled402Disable); got != 1 {
		t.Fatalf("402 disable success count=%d want 1", got)
	}
	if got := scheduledActionSuccessCount([]string{"gone", "fail"}, scheduled402Delete); got != 1 {
		t.Fatalf("402 delete success count=%d want 1", got)
	}
}

func TestRecordScheduledActionProgressSeparatesDeleteAndDisable(t *testing.T) {
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "d1", FileName: "d1.json", Disabled: true},
		{AuthIndex: "d2", FileName: "d2.json", Disabled: false},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	var disabled, deleted, failed int
	recordScheduledActionProgress([]string{"d1", "d2"}, scheduled402Disable, &disabled, &deleted, &failed)
	if disabled != 1 || deleted != 0 || failed != 1 {
		t.Fatalf("disable progress disabled=%d deleted=%d failed=%d", disabled, deleted, failed)
	}

	disabled, deleted, failed = 0, 0, 0
	recordScheduledActionProgress([]string{"missing", "d2"}, scheduled402Delete, &disabled, &deleted, &failed)
	if deleted != 1 || disabled != 0 || failed != 1 {
		t.Fatalf("delete progress disabled=%d deleted=%d failed=%d", disabled, deleted, failed)
	}
}
