package main

import (
	"strings"
	"testing"
)

func TestUIPreviewStructureContract(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, marker := range []string{
		`class="page-head"`,
		`class="eyebrow`,
		`class="mode-tabs`,
		`class="access-row`,
		`id="managementKey"`,
		`class="toolbar"`,
		`class="sampling-row`,
		`class="schedule-row"`,
		`class="autoban-control"`,
		`class="autoban-bar"`,
		`class="autoban-actions"`,
		`class="overview`,
		`class="results`,
		`class="results-head"`,
		`class="table-wrap account-pool"`,
		`class="table-footer`,
		`data-icon="circle-help"`,
		`data-icon="scan-search"`,
		`data-icon="shield-ban"`,
		`id="tabInspect"`,
		`id="tabAutoban"`,
		`id="banEnabledToggle"`,
		`id="banSummary"`,
		`id="banRows"`,
		`id="rows"`,
		`id="pager"`,
		`id="banPager"`,
		`function switchTab`,
		`.inspection-only`,
		`.autoban-only`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("preview structure missing %q", marker)
		}
	}
	// No external icon CDN dependency.
	for _, bad := range []string{
		"unpkg.com/lucide",
		"cdn.jsdelivr.net",
		"data-lucide=",
	} {
		if strings.Contains(page, bad) {
			t.Fatalf("page must not depend on external icon CDN marker %q", bad)
		}
	}
	// CPA chrome must not be embedded in plugin page.
	for _, bad := range []string{
		`class="sidebar"`,
		`class="topbar"`,
		`CPAMP`,
	} {
		if strings.Contains(page, bad) {
			t.Fatalf("plugin page must not embed CPA chrome %q", bad)
		}
	}
}
