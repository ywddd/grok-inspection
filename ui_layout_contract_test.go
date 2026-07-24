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
		`id="panel-inspect"`,
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
		`@media (max-width:640px)`,
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

	// Desktop/base layout selectors must live outside the mobile media block.
	assertCSSSelectorsOutsideMobile(t, page, []string{
		".page-head {",
		".page-head-main {",
		".title-row {",
		".head-actions {",
		".lang-ctl {",
		".help-wrap {",
		".help-btn {",
		".help-popover {",
		".autoban-control {",
		".autoban-bar {",
		".autoban-actions {",
		".summary.ban-summary { grid-template-columns:repeat(6,minmax(0,1fr))",
		".grok-inspection-page .tabs {",
		`html[data-grok-theme="dark"] .grok-inspection-page .tab.active`,
	})
	if n := countCSSMedia(page, "max-width:640px"); n != 1 {
		t.Fatalf("want exactly 1 max-width:640px media block, got %d", n)
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

// cssOutsideMobile returns the UI CSS with all max-width:640px media blocks removed.
func cssOutsideMobile(page string) string {
	css := extractUICSS(page)
	var b strings.Builder
	i := 0
	for i < len(css) {
		idx := strings.Index(css[i:], "@media")
		if idx < 0 {
			b.WriteString(css[i:])
			break
		}
		idx += i
		b.WriteString(css[i:idx])
		// find opening brace of this media query
		j := idx + len("@media")
		for j < len(css) && css[j] != '{' {
			j++
		}
		if j >= len(css) {
			b.WriteString(css[idx:])
			break
		}
		header := css[idx:j]
		// brace-match media body
		depth := 0
		k := j
		for k < len(css) {
			switch css[k] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					k++
					goto doneMedia
				}
			}
			k++
		}
	doneMedia:
		if strings.Contains(header, "max-width:640px") {
			// drop mobile media block
			i = k
			continue
		}
		b.WriteString(css[idx:k])
		i = k
	}
	return b.String()
}

func extractUICSS(page string) string {
	// Prefer the embedded style block from the rendered page.
	start := strings.Index(page, "<style>")
	end := strings.Index(page, "</style>")
	if start >= 0 && end > start {
		return page[start+len("<style>") : end]
	}
	return uiCSS
}

func countCSSMedia(page, needle string) int {
	css := extractUICSS(page)
	count := 0
	i := 0
	for i < len(css) {
		idx := strings.Index(css[i:], "@media")
		if idx < 0 {
			break
		}
		idx += i
		j := idx + len("@media")
		for j < len(css) && css[j] != '{' {
			j++
		}
		if j >= len(css) {
			break
		}
		header := css[idx:j]
		depth := 0
		k := j
		for k < len(css) {
			switch css[k] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					k++
					goto done
				}
			}
			k++
		}
	done:
		if strings.Contains(header, needle) {
			count++
		}
		i = k
	}
	return count
}

func assertCSSSelectorsOutsideMobile(t *testing.T, page string, selectors []string) {
	t.Helper()
	outside := cssOutsideMobile(page)
	css := extractUICSS(page)
	for _, sel := range selectors {
		if !strings.Contains(css, sel) {
			t.Fatalf("CSS missing selector/snippet %q", sel)
		}
		if !strings.Contains(outside, sel) {
			t.Fatalf("desktop/base CSS selector %q must not live only inside max-width:640px media", sel)
		}
		// First occurrence must also be outside mobile: compare positions in full CSS
		// by ensuring the first match index is not inside a mobile media span.
		first := strings.Index(css, sel)
		if first < 0 {
			t.Fatalf("CSS missing selector/snippet %q", sel)
		}
		if cssIndexInsideMobileMedia(css, first) {
			t.Fatalf("first occurrence of %q is inside max-width:640px media; desktop would miss it", sel)
		}
	}
}

func cssIndexInsideMobileMedia(css string, pos int) bool {
	i := 0
	for i < len(css) {
		idx := strings.Index(css[i:], "@media")
		if idx < 0 {
			return false
		}
		idx += i
		j := idx + len("@media")
		for j < len(css) && css[j] != '{' {
			j++
		}
		if j >= len(css) {
			return false
		}
		header := css[idx:j]
		depth := 0
		k := j
		for k < len(css) {
			switch css[k] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					k++
					goto done
				}
			}
			k++
		}
	done:
		if strings.Contains(header, "max-width:640px") && pos >= idx && pos < k {
			return true
		}
		i = k
	}
	return false
}
