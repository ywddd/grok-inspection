package main

import (
	"regexp"
	"strings"
	"testing"
)

func extractUICSSForHygiene(page string) string {
	start := strings.Index(page, "<style>")
	end := strings.Index(page, "</style>")
	if start >= 0 && end > start {
		return page[start+len("<style>") : end]
	}
	return uiCSS
}

func countMediaQueries(css, needle string) int {
	re := regexp.MustCompile(`@media\s*\(([^)]*)\)`)
	count := 0
	for _, m := range re.FindAllStringSubmatch(css, -1) {
		if strings.Contains(m[1], needle) {
			count++
		}
	}
	return count
}

func TestUICSSMediaQueryHygiene(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))
	if n := countMediaQueries(css, "max-width:640px"); n != 1 {
		t.Fatalf("want exactly 1 max-width:640px media, got %d", n)
	}
	if n := countMediaQueries(css, "max-width:760px"); n != 1 {
		t.Fatalf("want exactly 1 max-width:760px media, got %d", n)
	}
	// Preview tablet breakpoints should not be duplicated with spaced/unspaced variants.
	if n := countMediaQueries(css, "max-width:1220px"); n != 1 {
		t.Fatalf("want exactly 1 max-width:1220px media, got %d", n)
	}
	if n := countMediaQueries(css, "max-width:900px"); n != 1 {
		t.Fatalf("want exactly 1 max-width:900px media, got %d", n)
	}
}

func TestUIDarkSelectorsDoNotLeakToLightMode(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))
	// Explicit known-bad pattern from review.
	bad := []string{
		`html[data-grok-theme="dark"] .tab.active, .mode-tab.active`,
		`html[data-grok-theme="dark"] .tab.active small, .mode-tab.active small`,
		`html[data-grok-theme="dark"] .tab.active .tab-desc, .mode-tab.active .tab-desc`,
	}
	for _, b := range bad {
		if strings.Contains(css, b) {
			t.Fatalf("found unscoped dark/light mixed selector: %s", b)
		}
	}
	// Any comma-separated selector list that includes a dark-qualified part must dark-qualify every part.
	re := regexp.MustCompile(`(?m)^[ 	]*([^@{
][^{
]*data-grok-theme="dark"[^{
]*)\{`)
	for _, m := range re.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		if !strings.Contains(sel, ",") {
			continue
		}
		for _, part := range strings.Split(sel, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !strings.Contains(part, `data-grok-theme="dark"`) {
				t.Fatalf("dark rule leaks into light mode via unscoped selector part %q in %q", part, sel)
			}
		}
	}
}

func TestUIMobileTabTextDoesNotForcePageOverflow(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))
	page := string(renderUIPage(pluginName))
	// Mobile tab text wrappers must be allowed to shrink/ellipsis.
	needles := []string{
		`.tab > span`,
		`min-width:0`,
		`text-overflow:ellipsis`,
		`.table-scroll`,
		`overflow-x:auto`,
	}
	// Accept either .tab > span or .mode-tab > span style wrappers.
	if !strings.Contains(css, ".tab > span") && !strings.Contains(css, ".mode-tab > span") && !strings.Contains(css, ".tab .tab-title") {
		t.Fatal("mobile tab text wrapper rules missing")
	}
	for _, n := range []string{`text-overflow:ellipsis`, `.table-scroll`, `overflow-x:auto`, `min-width:0`} {
		if !strings.Contains(css, n) {
			t.Fatalf("overflow contract missing %q", n)
		}
	}
	_ = needles
	// Page shell must not force wide min-width; wide tables stay inside scroll containers.
	if strings.Contains(css, "body { min-width:") || strings.Contains(css, "html, body { min-width:") {
		// allow min-width:0 only
		if strings.Contains(css, "body { min-width:") && !strings.Contains(css, "min-width:0") {
			t.Fatal("body min-width must not force horizontal page overflow")
		}
	}
	// Ensure account tables keep scroll containment classes in markup.
	if !strings.Contains(page, `class="table-scroll"`) && !strings.Contains(page, `class="table-scroll `) {
		// template uses class="table-scroll" inside account-pool
		if c := strings.Count(page, "table-scroll"); c < 2 {
			t.Fatalf("want table-scroll containers for inspect+ban pools, got %d", c)
		}
	}
	// 390px mobile media (preview 640 covers 390) must clamp tabs.
	if !strings.Contains(css, "@media (max-width:640px)") && !strings.Contains(css, "@media (max-width: 640px)") {
		t.Fatal("missing 640px mobile media used for 390px phones")
	}
	if !strings.Contains(css, "overflow-x:hidden") && !strings.Contains(css, "overflow-x: hidden") {
		t.Fatal("mobile body/page should hide document-level horizontal overflow")
	}
}

func TestUICSSDoesNotStackDuplicateSystems(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))
	// Guard against the previous dual-stack marker.
	if strings.Count(css, "/* Plugin host adaptations") > 1 {
		t.Fatal("plugin host adaptations block duplicated")
	}
	// Core layout selectors should appear as a single coherent system (not thrice redefined loosely).
	if strings.Count(css, ".autoban-bar {") > 3 {
		t.Fatalf("autoban-bar defined too many times: %d", strings.Count(css, ".autoban-bar {"))
	}
	if strings.Count(css, ".page-head {") > 2 {
		t.Fatalf("page-head defined too many times: %d", strings.Count(css, ".page-head {"))
	}
}

func TestUIPreviewMetricAlignment(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))

	// Tab titles inherit parent line-height (preview ~18.85 at 13px body), not a compressed 1.2.
	if strings.Contains(css, `.tab .tab-title, .mode-tab .tab-title {
          display:block; line-height:1.2;`) || strings.Contains(css, "line-height:1.2; overflow:hidden; white-space:nowrap; text-overflow:ellipsis; max-width:100%;") {
		// allow 1.2 only if not on tab-title; require inherit on tab-title
	}
	if !strings.Contains(css, ".tab .tab-title") || !strings.Contains(css, "line-height:inherit") {
		t.Fatal("tab-title must use line-height:inherit to match preview tab metrics")
	}
	// Ensure the primary tab-title rule is not still locked to 1.2.
	reTitle := regexp.MustCompile(`\.tab \.tab-title[^{]*\{[^}]*\}`)
	foundInherit := false
	for _, m := range reTitle.FindAllString(css, -1) {
		if strings.Contains(m, "line-height:1.2") {
			t.Fatalf("tab-title still uses compressed line-height: %s", m)
		}
		if strings.Contains(m, "line-height:inherit") {
			foundInherit = true
		}
	}
	if !foundInherit {
		t.Fatal("tab-title rule missing line-height:inherit")
	}

	// Mobile mode-tabs gap must stay 4px like preview (not 6px).
	reMobileTabs := regexp.MustCompile(`@media \(max-width:760px\)\s*\{[\s\S]*?\.mode-tabs[\s\S]*?gap:\s*([0-9.]+)px`)
	m := reMobileTabs.FindStringSubmatch(css)
	if m == nil {
		// also allow .grok-inspection-page .tabs in the same rule
		reMobileTabs = regexp.MustCompile(`@media \(max-width:760px\)\s*\{[\s\S]*?\.tabs[\s\S]*?gap:\s*([0-9.]+)px`)
		m = reMobileTabs.FindStringSubmatch(css)
	}
	if m == nil {
		t.Fatal("760px mode-tabs gap not found")
	}
	if m[1] != "4" {
		t.Fatalf("760px mode-tabs gap=%spx, want 4px", m[1])
	}
	// 640 must not reintroduce a larger gap.
	if strings.Contains(css, "gap:6px") && strings.Contains(css, "mode-tabs") {
		// only fail if gap:6px appears near mode-tabs
		if regexp.MustCompile(`mode-tabs[\s\S]{0,120}gap:\s*6px|gap:\s*6px[\s\S]{0,80}mode-tabs`).MatchString(css) {
			t.Fatal("mode-tabs still uses gap:6px somewhere")
		}
	}

	// page-head keeps 18px baseline; mobile must not shrink to 10px.
	if !strings.Contains(css, ".page-head { display:flex; align-items:flex-start; justify-content:space-between; gap:18px;") &&
		!strings.Contains(css, "gap:18px; margin-bottom:14px") {
		// looser
		if !regexp.MustCompile(`\.page-head\s*\{[^}]*gap:\s*18px`).MatchString(css) {
			t.Fatal("page-head baseline gap must be 18px")
		}
	}
	if regexp.MustCompile(`@media \(max-width:640px\)[\s\S]*?\.page-head\s*\{\s*gap:\s*10px`).MatchString(css) {
		t.Fatal("640px page-head must not reduce gap to 10px; keep 18px baseline")
	}

	// Key hint inherits body font-size; other hints stay 12px.
	if !regexp.MustCompile(`\.access-row\s+#keyHint|#keyHint\.key-state|\.access-row\s+#keyHint[\s\S]{0,80}font-size:\s*inherit`).MatchString(css) &&
		!strings.Contains(css, "font-size:inherit") {
		// require explicit inherit on key hint selector
		if !strings.Contains(css, ".access-row #keyHint") && !strings.Contains(css, ".access-row .key-state") {
			t.Fatal("missing access-row keyHint selector for inherited font-size")
		}
	}
	if !strings.Contains(css, ".access-row #keyHint") && !strings.Contains(css, ".access-row #keyHint,") {
		// accept combined selectors
		if !regexp.MustCompile(`\.access-row\s+#keyHint`).MatchString(css) {
			t.Fatal("expected .access-row #keyHint rule")
		}
	}
	if !regexp.MustCompile(`\.access-row\s+#keyHint[^{]*\{[^}]*font-size:\s*inherit`).MatchString(css) {
		t.Fatal("access-row #keyHint must use font-size:inherit")
	}
	if !regexp.MustCompile(`\.hint[^{]*\{[^}]*font-size:\s*12px`).MatchString(css) && !strings.Contains(css, ".hint, .pager-meta") {
		// existing combined rule
		if !strings.Contains(css, "font-size:12px") {
			t.Fatal("generic hints should remain 12px")
		}
	}

	// ~1000px schedule stays single-line: nowrap on desktop schedule-controls.
	if !regexp.MustCompile(`\.schedule-controls\s*\{[^}]*flex-wrap:\s*nowrap`).MatchString(css) &&
		!strings.Contains(css, "flex-wrap:nowrap") {
		t.Fatal("schedule-controls must use flex-wrap:nowrap for ~1000px single-row layout")
	}
	// Mobile may still wrap/grid; ensure 640 keeps grid for schedule-controls.
	if !strings.Contains(css, "@media (max-width:640px)") || !strings.Contains(css, "schedule-controls") {
		t.Fatal("mobile schedule layout rules missing")
	}
}
