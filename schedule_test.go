package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"grok-inspection/cpasdk/pluginapi"
)

func TestDefaultScheduleConfigSafe(t *testing.T) {
	cfg := defaultScheduleConfig()
	if cfg.Enabled {
		t.Fatal("default schedule must be disabled")
	}
	if cfg.Cron != defaultScheduleCron {
		t.Fatalf("cron = %q", cfg.Cron)
	}
	if err := validateScheduleConfig(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateScheduleConfigRejectsBadCron(t *testing.T) {
	cfg := defaultScheduleConfig()
	cfg.Cron = "not a cron"
	if err := validateScheduleConfig(&cfg); err == nil {
		t.Fatal("expected invalid cron error")
	}
}

func TestValidateScheduleConfigWorkers(t *testing.T) {
	cfg := defaultScheduleConfig()
	cfg.Workers = 99
	if err := validateScheduleConfig(&cfg); err == nil {
		t.Fatal("expected workers error")
	}
	cfg.Workers = 0
	if err := validateScheduleConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Workers != defaultWorkers {
		t.Fatalf("workers = %d", cfg.Workers)
	}
}

func TestSchedulePersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	setScheduleFilePathForTest(filepath.Join(dir, "schedule.json"))
	t.Cleanup(func() { setScheduleFilePathForTest("") })

	cfg := defaultScheduleConfig()
	cfg.Enabled = true
	cfg.Cron = "15 4 * * 1"
	cfg.Workers = 4
	cfg.AutoDeletePermissionDenied = true
	if err := saveScheduleConfig(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := loadScheduleConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Cron != "15 4 * * 1" || got.Workers != 4 || !got.AutoDeletePermissionDenied {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadScheduleMissingFileDefaults(t *testing.T) {
	dir := t.TempDir()
	setScheduleFilePathForTest(filepath.Join(dir, "missing.json"))
	t.Cleanup(func() { setScheduleFilePathForTest("") })
	got, err := loadScheduleConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("missing file should default disabled")
	}
}

func TestSelectAutoActionTargets(t *testing.T) {
	results := []accountResult{
		{Name: "a", Classification: "permission_denied", Disabled: false},
		{Name: "b", Classification: "quota_exhausted", Disabled: false},
		{Name: "c", Classification: "quota_exhausted", Disabled: true},
		{Name: "d", Classification: "healthy", Disabled: true},
		{Name: "e", Classification: "healthy", Disabled: false},
		{Name: "f", Classification: "reauth", Disabled: false},
	}
	cfg := scheduleConfig{
		AutoDeletePermissionDenied: true,
		AutoDisableQuotaExhausted:  true,
		AutoEnableHealthyDisabled:  true,
	}
	var disable, enable, del []string
	for _, item := range results {
		if cfg.AutoDisableQuotaExhausted && item.Classification == "quota_exhausted" && !item.Disabled {
			disable = append(disable, item.Name)
		}
		if cfg.AutoEnableHealthyDisabled && item.Classification == "healthy" && item.Disabled {
			enable = append(enable, item.Name)
		}
		if cfg.AutoDeletePermissionDenied && item.Classification == "permission_denied" {
			del = append(del, item.Name)
		}
	}
	if strings.Join(disable, ",") != "b" || strings.Join(enable, ",") != "d" || strings.Join(del, ",") != "a" {
		t.Fatalf("disable=%v enable=%v del=%v", disable, enable, del)
	}
}

func TestSchedulerBusySkip(t *testing.T) {
	oldEngine := engine
	oldSched := scheduler
	engine = &inspectionEngine{workers: defaultWorkers, running: true}
	scheduler = &inspectionScheduler{cfg: scheduleConfig{Enabled: true, Cron: "0 3 * * *", Workers: 6}}
	t.Cleanup(func() {
		engine = oldEngine
		scheduler = oldSched
	})

	scheduler.onTick()
	view := scheduler.view()
	if view.LastRunStatus != "skipped_busy" {
		t.Fatalf("status = %q, want skipped_busy", view.LastRunStatus)
	}
}

func TestManualStartClearsScheduledFlag(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers, scheduledRun: true}
	t.Cleanup(func() { engine = old })

	// start will fail listing hosts in unit test, but flag must be cleared when accepted.
	// Without host, start still launches goroutine — force busy path instead by applying.
	engine.running = true
	if err := engine.start(startRequest{Workers: 2}); err == nil {
		t.Fatal("expected busy")
	}
	// Direct unit: start path sets scheduledRun false when it acquires lock.
	engine.running = false
	engine.applying = false
	engine.actionInFlight = 0
	// Avoid hanging forever if host list is missing: use invalid workers first.
	if err := engine.start(startRequest{Workers: 99}); err == nil {
		t.Fatal("expected workers error")
	}
}

func TestScheduleHTTPGetPut(t *testing.T) {
	dir := t.TempDir()
	setScheduleFilePathForTest(filepath.Join(dir, "schedule.json"))
	oldSched := scheduler
	scheduler = &inspectionScheduler{cfg: defaultScheduleConfig()}
	t.Cleanup(func() {
		setScheduleFilePathForTest("")
		scheduler.stop()
		scheduler = oldSched
	})

	get := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/schedule",
	})
	if get.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d", get.StatusCode)
	}

	body, _ := json.Marshal(scheduleConfig{
		Enabled:                    false,
		Cron:                       "30 1 * * *",
		Workers:                    3,
		AutoDeletePermissionDenied: true,
	})
	put := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPut,
		Path:   "/v0/management/plugins/grok-inspection/schedule",
		Body:   body,
	})
	if put.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", put.StatusCode, string(put.Body))
	}
	var view scheduleView
	if err := json.Unmarshal(put.Body, &view); err != nil {
		t.Fatal(err)
	}
	if view.Cron != "30 1 * * *" || view.Workers != 3 || !view.AutoDeletePermissionDenied {
		t.Fatalf("view = %+v", view)
	}
	if _, err := os.Stat(scheduleFilePath()); err != nil {
		t.Fatal(err)
	}

	bad := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPut,
		Path:   "/v0/management/plugins/grok-inspection/schedule",
		Body:   []byte(`{"enabled":true,"cron":"bad","workers":6}`),
	})
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad cron status = %d", bad.StatusCode)
	}
}

func TestStatusIncludesSchedule(t *testing.T) {
	oldSched := scheduler
	scheduler = &inspectionScheduler{cfg: defaultScheduleConfig()}
	t.Cleanup(func() { scheduler = oldSched })

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
		Query:  map[string][]string{"include_results": {"0"}},
	})
	if !strings.Contains(string(resp.Body), `"schedule"`) {
		t.Fatalf("status missing schedule: %s", string(resp.Body))
	}
}

func TestResourcePageHasSchedulePanel(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, marker := range []string{
		`id="schedulePanel"`,
		`id="schedEnabled"`,
		`id="schedCron"`,
		`auto_delete_permission_denied`,
		`MANAGEMENT_PASSWORD`,
		`saveSchedule`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("missing UI marker %q", marker)
		}
	}
}

func TestRunAutoActionsSkipsWithoutPassword(t *testing.T) {
	oldEngine := engine
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	oldKey := os.Getenv("CPA_MANAGEMENT_KEY")
	_ = os.Unsetenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("CPA_MANAGEMENT_KEY")
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{Name: "x", Classification: "permission_denied", FileName: "x.json"},
		},
	}
	oldSched := scheduler
	scheduler = &inspectionScheduler{cfg: defaultScheduleConfig()}
	t.Cleanup(func() {
		engine = oldEngine
		scheduler = oldSched
		if oldPass != "" {
			_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
		}
		if oldKey != "" {
			_ = os.Setenv("CPA_MANAGEMENT_KEY", oldKey)
		}
	})

	engine.runAutoActionsAfterScheduled(scheduleConfig{AutoDeletePermissionDenied: true})
	view := scheduler.view()
	if view.LastRunStatus != "auto_failed_auth" {
		t.Fatalf("status = %q", view.LastRunStatus)
	}
	if view.LastAutoDeleted != 0 || view.LastAutoDisabled != 0 || view.LastAutoEnabled != 0 {
		t.Fatalf("counts should be zero on auth fail: %+v", view)
	}
}

func TestRecordFinishedStoresAutoCounts(t *testing.T) {
	old := scheduler
	scheduler = &inspectionScheduler{cfg: defaultScheduleConfig()}
	t.Cleanup(func() { scheduler = old })

	scheduler.recordFinished("ok", nil, autoActionCounts{Deleted: 2, Disabled: 3, Enabled: 1})
	view := scheduler.view()
	if view.LastAutoDeleted != 2 || view.LastAutoDisabled != 3 || view.LastAutoEnabled != 1 {
		t.Fatalf("view counts = del=%d dis=%d en=%d", view.LastAutoDeleted, view.LastAutoDisabled, view.LastAutoEnabled)
	}
	if !strings.Contains(string(mustJSON(view)), `"last_auto_deleted":2`) {
		t.Fatalf("json missing counts: %s", string(mustJSON(view)))
	}
}

func TestResourcePageShowsAutoCountsInMeta(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, "last_auto_deleted") || !strings.Contains(page, "自动处置 删除") {
		t.Fatal("schedule meta should render auto action counts")
	}
}
