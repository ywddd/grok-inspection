package main

import (
	"strings"
	"testing"
)

func TestUIScheduleStatusMapsFailedAndStopped(t *testing.T) {
	page := string(renderUIPage(pluginName))
	// Branch coverage in scheduleStatusText.
	for _, frag := range []string{
		"status === 'failed' ? t('schedule_failed')",
		"status === 'stopped' ? t('schedule_stopped')",
		"status === 'completed' ? t('schedule_completed')",
	} {
		if !strings.Contains(page, frag) {
			t.Fatalf("scheduleStatusText missing branch: %s", frag)
		}
	}
	zh := extractI18NPack(page, "zh")
	en := extractI18NPack(page, "en")
	for _, key := range []string{"schedule_failed", "schedule_stopped", "schedule_completed"} {
		if strings.TrimSpace(zh[key]) == "" {
			t.Fatalf("zh %s missing/empty", key)
		}
		if strings.TrimSpace(en[key]) == "" {
			t.Fatalf("en %s missing/empty", key)
		}
	}
	// EN must be explicit English phrases (not falling back to waiting).
	if !strings.Contains(strings.ToLower(en["schedule_failed"]), "fail") {
		t.Fatalf("en schedule_failed=%q", en["schedule_failed"])
	}
	if !strings.Contains(strings.ToLower(en["schedule_stopped"]), "stop") {
		t.Fatalf("en schedule_stopped=%q", en["schedule_stopped"])
	}
	// ZH must differ from waiting text.
	if zh["schedule_failed"] == zh["schedule_waiting"] || zh["schedule_stopped"] == zh["schedule_waiting"] {
		t.Fatalf("zh failed/stopped must not equal waiting")
	}
}
