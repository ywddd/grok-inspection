package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateInspectionScheduleSavesOnlyDisabled(t *testing.T) {
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

	only := true
	include := true // contradictory input must normalize
	cfg, err := updateInspectionSchedule(inspectionScheduleUpdate{
		OnlyDisabled:    &only,
		IncludeDisabled: &include,
	})
	if err != nil {
		t.Fatalf("updateInspectionSchedule() error = %v", err)
	}
	if !cfg.OnlyDisabled {
		t.Fatalf("saved OnlyDisabled=false, want true: %+v", cfg)
	}
	if cfg.IncludeDisabled {
		t.Fatalf("OnlyDisabled=true must force IncludeDisabled=false: %+v", cfg)
	}

	raw, err := os.ReadFile(scheduleFilePath())
	if err != nil {
		t.Fatalf("schedule file: %v", err)
	}
	var disk persistedInspectionSchedule
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatalf("schedule json: %v", err)
	}
	if !disk.OnlyDisabled || disk.IncludeDisabled {
		t.Fatalf("persisted schedule only/include = %+v", disk)
	}

	status := inspectionScheduleStatus()
	if got, _ := status["only_disabled"].(bool); !got {
		t.Fatalf("status only_disabled=%v want true", status["only_disabled"])
	}
	if got, _ := status["include_disabled"].(bool); got {
		t.Fatalf("status include_disabled=%v want false", status["include_disabled"])
	}

	// Switching to include_disabled must clear only_disabled.
	includeOn := true
	cfg, err = updateInspectionSchedule(inspectionScheduleUpdate{IncludeDisabled: &includeOn})
	if err != nil {
		t.Fatalf("switch include: %v", err)
	}
	if !cfg.IncludeDisabled || cfg.OnlyDisabled {
		t.Fatalf("include switch schedule=%+v", cfg)
	}
}

func TestNormalizePersistedInspectionScheduleOnlyDisabledMutex(t *testing.T) {
	got := normalizePersistedInspectionSchedule(persistedInspectionSchedule{
		IncludeDisabled: true,
		OnlyDisabled:    true,
	})
	if !got.OnlyDisabled {
		t.Fatalf("OnlyDisabled should stay true: %+v", got)
	}
	if got.IncludeDisabled {
		t.Fatalf("OnlyDisabled=true must clear IncludeDisabled: %+v", got)
	}
}

func TestScheduledInspectionRequestUsesOnlyDisabled(t *testing.T) {
	req := scheduledInspectionRequest(persistedInspectionSchedule{
		Workers:         4,
		IncludeDisabled: true,
		OnlyDisabled:    true,
	})
	if !req.OnlyDisabled {
		t.Fatalf("OnlyDisabled not passed: %+v", req)
	}
	// Request builder may still surface IncludeDisabled from cfg; engine start
	// normalizes the pair. Prefer the schedule builder to already clear it.
	if req.IncludeDisabled {
		t.Fatalf("OnlyDisabled schedule must not keep IncludeDisabled=true: %+v", req)
	}
	if req.Workers != 4 || req.Incremental || len(req.Classifications) != 0 {
		t.Fatalf("only-disabled scheduled request shape: %+v", req)
	}
}

func TestUIScheduleOnlyDisabledContract(t *testing.T) {
	page := string(renderUIPage(pluginName))

	for _, marker := range []string{
		`id="scheduleOnlyDisabled"`,
		`data-i18n="schedule_only_disabled"`,
		`only_disabled: $('scheduleOnlyDisabled').checked`,
		`$('scheduleOnlyDisabled').checked = !!data.only_disabled`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("schedule only-disabled UI missing marker %q", marker)
		}
	}

	// Mutual exclusion with schedule include-disabled, mirroring manual inspect wireExclusive.
	if !strings.Contains(uiScriptWire, "scheduleOnlyDisabled") {
		t.Fatal("uiScriptWire must include scheduleOnlyDisabled in dirty listeners")
	}
	hasExclusive := strings.Contains(page, "scheduleOnly.checked = false") ||
		strings.Contains(page, "scheduleOnlyDisabled').checked = false") ||
		strings.Contains(page, `$('scheduleOnlyDisabled').checked = false`)
	hasExclusiveReverse := strings.Contains(page, "scheduleInclude.checked = false") ||
		strings.Contains(page, "scheduleIncludeDisabled').checked = false") ||
		strings.Contains(page, `$('scheduleIncludeDisabled').checked = false`)
	if !hasExclusive || !hasExclusiveReverse {
		t.Fatalf("schedule include/only exclusive handlers missing (include->only:%v only->include:%v)", hasExclusive, hasExclusiveReverse)
	}

	zh := extractI18NPack(page, "zh")
	en := extractI18NPack(page, "en")
	if strings.TrimSpace(zh["schedule_only_disabled"]) == "" {
		t.Fatal("zh schedule_only_disabled missing/empty")
	}
	if strings.TrimSpace(en["schedule_only_disabled"]) == "" {
		t.Fatal("en schedule_only_disabled missing/empty")
	}
	if zh["schedule_only_disabled"] == en["schedule_only_disabled"] {
		t.Fatalf("zh/en schedule_only_disabled should differ: zh=%q en=%q", zh["schedule_only_disabled"], en["schedule_only_disabled"])
	}
	// Keep wording aligned with the manual-inspect only_disabled labels.
	if !strings.Contains(zh["schedule_only_disabled"], "禁用") {
		t.Fatalf("zh schedule_only_disabled=%q", zh["schedule_only_disabled"])
	}
	if !strings.Contains(strings.ToLower(en["schedule_only_disabled"]), "disabled") {
		t.Fatalf("en schedule_only_disabled=%q", en["schedule_only_disabled"])
	}
}
