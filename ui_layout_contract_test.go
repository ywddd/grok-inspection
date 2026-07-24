package main

import (
	"strings"
	"testing"
)

func TestUILayoutBilingualContract(t *testing.T) {
	page := string(renderUIPage(pluginName))

	// Separate tab panels; default is account inspection.
	for _, marker := range []string{
		`id="tabInspect"`,
		`id="tabAutoban"`,
		`id="panel-inspect"`,
		`id="panel-autoban"`,
		`data-tab="inspect"`,
		`data-tab="autoban"`,
		`class="tab active"`,
		`class="panel active" id="panel-inspect"`,
		`switchTab('inspect')`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("missing tab/panel contract marker %q", marker)
		}
	}
	if strings.Contains(page, `class="panel active" id="panel-autoban"`) {
		t.Fatal("autoban panel must not be active by default")
	}

	// Compact page head + accessible help affordance (no always-visible long subtitle block).
	for _, marker := range []string{
		`class="page-head"`,
		`id="helpBtn"`,
		`id="helpPopover"`,
		`id="heroSub"`,
		`function updateModeHelp`,
		`function closeHelpPopover`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("missing help/page-head marker %q", marker)
		}
	}
	if strings.Contains(page, `class="sub" id="heroSub" data-i18n="subtitle"`) {
		t.Fatal("long subtitle must not remain an always-visible hero paragraph")
	}

	// Realtime auto-ban controls + six category filters + pool/pager IDs preserved.
	for _, marker := range []string{
		`id="banEnabledToggle"`,
		`id="banEnabledPill"`,
		`id="banEnabledHint"`,
		`id="banRefreshBtn"`,
		`id="banUnbanFilterBtn"`,
		`id="banUnbanAllBtn"`,
		`id="banFilterHint"`,
		`id="banUnsyncedBanner"`,
		`id="banSummary"`,
		`id="banRows"`,
		`id="banEmpty"`,
		`id="banPager"`,
		`id="banError"`,
		`data-ban-filter="all"`,
		`data-ban-filter="quota"`,
		`data-ban-filter="spending_limit"`,
		`data-ban-filter="permission"`,
		`data-ban-filter="unauthorized"`,
		`data-ban-filter="manual"`,
		`id="banCount"`,
		`id="banQuotaCount"`,
		`id="banSpendingLimitCount"`,
		`id="banPermissionCount"`,
		`id="banUnauthorizedCount"`,
		`id="banManualDisabledCount"`,
		`class="autoban-control"`,
		`class="autoban-bar`,
		`class="autoban-actions"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("missing autoban UX contract marker %q", marker)
		}
	}

	// Matching account-pool containers for inspection and auto-ban.
	if c := strings.Count(page, `class="table-wrap account-pool"`); c != 2 {
		t.Fatalf("want 2 account-pool containers, got %d", c)
	}

	// Layout CSS contracts: six-up ban cards, tablet 3-col, mobile 2-col, help popover, dark tabs.
	for _, marker := range []string{
		`.summary.ban-summary { grid-template-columns:repeat(6,minmax(0,1fr))`,
		`@media (min-width:761px) and (max-width:1220px)`,
		`@media (max-width:760px)`,
		`.help-popover`,
		`html[data-grok-theme="dark"] .grok-inspection-page .tab.active`,
		`html[data-grok-theme="dark"] .grok-inspection-page th`,
		`html[data-grok-theme="dark"] .grok-inspection-page .status-pill`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("missing layout CSS contract marker %q", marker)
		}
	}
	if !strings.Contains(page, `grid-template-columns:repeat(3,minmax(0,1fr))`) {
		t.Fatal("tablet/medium ban summary should fall back to 3 columns")
	}

	zh := extractI18NPack(page, "zh")
	en := extractI18NPack(page, "en")
	for _, key := range []string{
		"title", "help_title", "help_autoban_title",
		"help_inspect_p1", "help_inspect_p2", "help_inspect_p3",
		"help_autoban_p1", "help_autoban_p2",
		"ban_title", "tab_inspect", "tab_autoban",
	} {
		if strings.TrimSpace(zh[key]) == "" {
			t.Fatalf("zh %s missing/empty", key)
		}
		if strings.TrimSpace(en[key]) == "" {
			t.Fatalf("en %s missing/empty", key)
		}
	}
	if en["title"] != "Grok inspection" {
		t.Fatalf("en compact title = %q, want Grok inspection", en["title"])
	}
	if !strings.Contains(en["ban_unban_filter"], "Unban") {
		t.Fatalf("en ban_unban_filter = %q", en["ban_unban_filter"])
	}
	if zh["title"] == en["title"] {
		t.Fatal("zh/en page titles should differ")
	}
}
