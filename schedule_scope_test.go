package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The scheduled sample run must reuse the toolbar inputs instead of a second copy.
func TestScheduleSampleReusesToolbarInputs(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, stray := range []string{"id=\"scheduleSampleCount\"", "id=\"scheduleSamplePercent\"", "scheduleSampleCountField", "scheduleSamplePercentField"} {
		if strings.Contains(page, stray) {
			t.Fatalf("schedule row must not define its own sample input %q", stray)
		}
	}
	if !strings.Contains(uiScriptSchedule, "parseSampleInputs()") || !strings.Contains(uiScriptSchedule, "saveSamplePrefs(sample)") {
		t.Fatal("saveSchedule must read and persist the toolbar sample inputs")
	}
	if !strings.Contains(uiScriptSchedule, "schedule-linked") {
		t.Fatal("sample scope must highlight the toolbar sample row it reuses")
	}
}

// The status payload must expose scope/sample so the UI can restore them after save.
func TestInspectionScheduleStatusExposesScope(t *testing.T) {
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
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

	scope := scheduleScopeSample
	percent := 25
	if _, err := updateInspectionSchedule(inspectionScheduleUpdate{Scope: &scope, SamplePercent: &percent}); err != nil {
		t.Fatalf("save sample schedule: %v", err)
	}

	status := inspectionScheduleStatus()
	if got := status["scope"]; got != scheduleScopeSample {
		t.Fatalf("status scope=%v want sample", got)
	}
	if got := status["sample_percent"]; got != 25 {
		t.Fatalf("status sample_percent=%v want 25", got)
	}
	if _, ok := status["sample_count"]; !ok {
		t.Fatal("status must carry sample_count so the UI can restore it")
	}
}
func TestNormalizeScheduleScope(t *testing.T) {
	if got, err := normalizeScheduleScope(""); err != nil || got != scheduleScopeFull {
		t.Fatalf("default scope=%q err=%v", got, err)
	}
	if got, err := normalizeScheduleScope("SAMPLE"); err != nil || got != scheduleScopeSample {
		t.Fatalf("sample scope=%q err=%v", got, err)
	}
	if _, err := normalizeScheduleScope("incremental"); err == nil {
		t.Fatal("unsupported scope must fail")
	}
}

func TestScheduledInspectionRequestUsesSampleConfig(t *testing.T) {
	req := scheduledInspectionRequest(persistedInspectionSchedule{
		Workers:       8,
		Scope:         scheduleScopeSample,
		SampleCount:   25,
		SamplePercent: 10,
	})
	if !req.Sample || req.SampleCount != 25 || req.SamplePercent != 10 {
		t.Fatalf("sample request=%+v", req)
	}
	if req.Incremental || len(req.Classifications) != 0 {
		t.Fatalf("sample run must not be incremental or category scoped: %+v", req)
	}
}

func TestScheduledInspectionRequestFallsBackToFullOnInvalidSample(t *testing.T) {
	req := scheduledInspectionRequest(persistedInspectionSchedule{
		Workers: 8,
		Scope:   scheduleScopeSample,
	})
	if req.Sample {
		t.Fatalf("sample without count/percent must fall back to full: %+v", req)
	}
}

func TestNormalizePersistedScheduleDropsUnusableSample(t *testing.T) {
	cfg := normalizePersistedInspectionSchedule(persistedInspectionSchedule{
		Scope: scheduleScopeSample,
	})
	if cfg.Scope != scheduleScopeFull {
		t.Fatalf("scope=%q want full", cfg.Scope)
	}
	cfg = normalizePersistedInspectionSchedule(persistedInspectionSchedule{
		Scope:         scheduleScopeFull,
		SampleCount:   5,
		SamplePercent: 5,
	})
	if cfg.SampleCount != 0 || cfg.SamplePercent != 0 {
		t.Fatalf("full scope must clear sample params: %+v", cfg)
	}
}

func TestUpdateInspectionScheduleSampleRoundTrip(t *testing.T) {
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
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

	scope := scheduleScopeSample
	if _, err := updateInspectionSchedule(inspectionScheduleUpdate{Scope: &scope}); err == nil {
		t.Fatal("sample scope without count/percent must be rejected")
	}

	count := 30
	cfg, err := updateInspectionSchedule(inspectionScheduleUpdate{Scope: &scope, SampleCount: &count})
	if err != nil {
		t.Fatalf("save sample schedule: %v", err)
	}
	if cfg.Scope != scheduleScopeSample || cfg.SampleCount != 30 {
		t.Fatalf("saved schedule=%+v", cfg)
	}

	full := scheduleScopeFull
	cfg, err = updateInspectionSchedule(inspectionScheduleUpdate{Scope: &full})
	if err != nil {
		t.Fatalf("switch back to full: %v", err)
	}
	if cfg.SampleCount != 0 || cfg.SamplePercent != 0 {
		t.Fatalf("full scope must clear sample params: %+v", cfg)
	}
}

func TestScheduledTargetsHonourProbedScope(t *testing.T) {
	engine.mu.Lock()
	prev := engine.results
	engine.results = []accountResult{
		{AuthIndex: "a1", HTTPStatus: 402, Classification: "spending_limit", ErrorCode: spendingLimitErrorCode},
		{AuthIndex: "a2", HTTPStatus: 402, Classification: "spending_limit", ErrorCode: spendingLimitErrorCode},
	}
	engine.mu.Unlock()
	defer func() {
		engine.mu.Lock()
		engine.results = prev
		engine.mu.Unlock()
	}()

	all := scheduledSpendingLimitTargets(scheduled402Disable, nil)
	if len(all) != 2 {
		t.Fatalf("nil scope must match every row, got %v", all)
	}

	scoped := scheduledSpendingLimitTargets(scheduled402Disable, map[string]struct{}{"ai:a2": {}})
	if len(scoped) != 1 || scoped[0] != "a2" {
		t.Fatalf("scoped run must only act on probed rows, got %v", scoped)
	}
}
