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

	// Keep the icon and two-line copy centered as one unit, while the copy itself
	// stays left-aligned with an explicit title/subtitle rhythm.
	for _, marker := range []string{
		"align-items:center; gap:9px",
		"text-align:left",
		"font-size:14px; line-height:20px",
		"margin-top:2px",
		"font-size:11px; line-height:16px",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("tab typography contract missing %q", marker)
		}
	}
	if strings.Contains(css, "padding:8px; align-items:flex-start") ||
		regexp.MustCompile(`\.(?:tab|mode-tab)(?:,|\s*\{)[^{}]*\{[^}]*align-items:\s*flex-start`).MatchString(css) {
		t.Fatal("mobile tabs must keep icon and copy vertically centered")
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
	// Status must never be squeezed into a narrow column: it wraps to its own line instead.
	if !regexp.MustCompile(`\.schedule-row\s*\{[^}]*display:\s*flex[^}]*flex-wrap:\s*wrap`).MatchString(css) {
		t.Fatal("schedule-row must be a wrapping flex row so the status can take a full line")
	}
	if !regexp.MustCompile(`#scheduleStatus\s*\{[^}]*flex:\s*1\s+1\s+300px`).MatchString(css) {
		t.Fatal("schedule status needs a non-shrinking flex basis so it wraps instead of turning into a narrow column")
	}
	// Mobile may still wrap/grid; ensure 640 keeps grid for schedule-controls.
	if !strings.Contains(css, "@media (max-width:640px)") || !strings.Contains(css, "schedule-controls") {
		t.Fatal("mobile schedule layout rules missing")
	}
}

func TestUIHelpPopoverUsesNormalOverlayStacking(t *testing.T) {
	page := string(renderUIPage(pluginName))
	css := extractUICSSForHygiene(page)

	// Switching tabs must close help (already in switchTab).
	if !strings.Contains(page, "function switchTab(name)") || !strings.Contains(page, "closeHelpPopover()") {
		t.Fatal("switchTab must close help popover")
	}
	// The popover is a normal overlay; it must not click through to covered tabs.
	for _, obsolete := range []string{"function tabFromPoint", "elementsFromPoint", "switchTab(tabEl.dataset.tab)"} {
		if strings.Contains(page, obsolete) {
			t.Fatalf("obsolete help click-through behavior still present: %q", obsolete)
		}
	}
	for _, marker := range []string{"pointerdown", "pop.contains(target)", "closeHelpPopover()"} {
		if !strings.Contains(page, marker) {
			t.Fatalf("help outside-click behavior missing %q", marker)
		}
	}
	// Escape still closes help.
	if !strings.Contains(page, `ev.key === 'Escape'`) && !strings.Contains(page, `ev.key === "Escape"`) {
		t.Fatal("Escape must still close help popover")
	}
	// Desktop opens into the title-row's free space; narrower screens open below
	// the button. In both cases the popover stays above page controls.
	for _, marker := range []string{
		"z-index:100",
		"top:-4px; left:calc(100% + 10px); right:auto",
		"@media (max-width:900px)",
		"top:40px; left:0; right:auto",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("help popover layout contract missing %q", marker)
		}
	}
	if strings.Contains(css, "z-index:60") {
		t.Fatal("mode tabs must not stack above the help popover")
	}
}

func TestUIHelpButtonAndNumericFieldsUseIntentionalAlignment(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))

	for _, marker := range []string{
		".title-row .help-btn {\n          width:28px; height:28px; border-radius:7px;",
		".field input[type=\"number\"] {\n          width:42px; min-width:0; padding:0; border:0; outline:0; background:transparent; text-align:center;",
		".sampling-controls .field input[type=\"number\"] {\n          width:52px; flex:0 0 52px; text-align:center;",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("UI alignment contract missing %q", marker)
		}
	}
	if strings.Contains(css, "border-radius:50%; color:var(--muted-2)") {
		t.Fatal("help button must not add a second circular visual around the circle-help icon")
	}
	if strings.Contains(css, `input[type="number"] {
          width:52px; flex:0 0 52px; text-align:left;`) ||
		strings.Contains(css, "background:transparent; text-align:right; font-weight:650") {
		t.Fatal("numeric fields must not retain mixed left/right alignment")
	}
}

func TestUIHelpPopoverStaysInsideMobilePageHead(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))

	for _, marker := range []string{
		".page-head { position:relative; }",
		".help-wrap { position:static; }",
		"top:calc(100% + 8px); left:0; right:0; width:auto;",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("mobile help popover containment contract missing %q", marker)
		}
	}
}

func TestUIHelpButtonOverridesHostButtonDefaults(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))

	for _, marker := range []string{
		".grok-inspection-page .title-row .help-btn {\n      width:28px !important; height:28px !important; min-height:28px !important;",
		"border:1px solid transparent !important; border-radius:7px !important;",
		"background:transparent !important; color:var(--muted-2) !important;",
		".grok-inspection-page .title-row .help-btn:hover,\n    .grok-inspection-page .title-row .help-btn[aria-expanded=\"true\"]",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("embedded help button override contract missing %q", marker)
		}
	}
}

func TestUINumericFieldsOverrideHostInputDefaults(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))

	for _, marker := range []string{
		"html[data-grok-theme] .grok-inspection-page .field input[type=\"number\"]",
		"min-height:0 !important; height:auto !important;",
		"text-align:center !important;",
		"border:0 !important; border-radius:0 !important;",
		"background:transparent !important; box-shadow:none !important;",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("embedded numeric input override contract missing %q", marker)
		}
	}
}

func TestUIAccountPoolShowsFullNamesAndDarkAutobanIcon(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))

	for _, marker := range []string{
		"table-layout:auto",
		"min-width:1100px",
		"word-break:break-all",
		"overflow-wrap:anywhere",
		"text-overflow:clip",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("account pool full-name contract missing: %s", marker)
		}
	}
	// Must not keep fixed+ellipsis on account cells as the primary layout.
	if strings.Contains(css, "table-layout:fixed") {
		t.Fatal("account pool tables should not use table-layout:fixed")
	}
	if strings.Contains(css, "td.col-name, td.account { color:#0f172a; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }") {
		t.Fatal("account cells must not force single-line ellipsis truncation")
	}
	if !strings.Contains(css, "html[data-grok-theme=\"dark\"] .autoban-heading-icon") &&
		!strings.Contains(css, `html[data-grok-theme="dark"] .autoban-heading-icon`) {
		t.Fatal("dark autoban heading icon rule missing")
	}
	if !strings.Contains(css, "background:#252b63") {
		t.Fatal("dark autoban heading icon needs non-white background")
	}
}

func TestUIButtonPaletteMatchesReleasedV016(t *testing.T) {
	page := string(renderUIPage(pluginName))
	css := extractUICSSForHygiene(page)

	// Base default buttons match v0.1.16 white outline style.
	for _, marker := range []string{
		"border:1px solid #d1d5db",
		"background:#fff",
		"color:#334155",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("default button token missing: %s", marker)
		}
	}

	// Variant colors must beat .grok-inspection-page button.btn via higher specificity + !important.
	for _, marker := range []string{
		`.grok-inspection-page .btn.primary`,
		`.grok-inspection-page button.primary`,
		`background:#2563eb !important`,
		`border-color:#2563eb !important`,
		`color:#fff !important`,
		`html:not([data-grok-theme="dark"]) .grok-inspection-page .btn.soft`,
		`background:#eef2ff !important`,
		`border-color:#c7d2fe !important`,
		`color:#3730a3 !important`,
		`html:not([data-grok-theme="dark"]) .grok-inspection-page .btn.danger`,
		`background:#fef2f2 !important`,
		`border-color:#fecaca !important`,
		`color:#b91c1c !important`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("button variant contract missing: %s", marker)
		}
	}

	// Template keeps v0.1.16 soft/primary/danger assignments.
	for _, marker := range []string{
		`id="runBtn" class="btn primary"`,
		`id="applyBtn" class="btn soft"`,
		`id="incrBtn" class="btn soft"`,
		`id="filterRunBtn" class="btn soft"`,
		`id="sampleBtn" class="btn soft"`,
		`id="scheduleSaveBtn" class="btn soft"`,
		`id="batchDisableBtn" class="btn soft"`,
		`id="batchEnableBtn" class="btn soft"`,
		`id="batchDeleteBtn" class="btn danger"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("button class assignment missing: %s", marker)
		}
	}

	// Dark default .btn must not swallow primary/soft/danger.
	if strings.Contains(css, "html[data-grok-theme=\"dark\"] .btn,") {
		t.Fatal("dark .btn rule must exclude primary/soft/danger")
	}
	if !strings.Contains(css, "html[data-grok-theme=\"dark\"] .btn:not(.primary):not(.soft):not(.danger)") {
		t.Fatal("dark default button selector should exclude variants")
	}
}

func TestUIPaletteMatchesReleasedV016(t *testing.T) {
	css := extractUICSSForHygiene(string(renderUIPage(pluginName)))

	for _, token := range []string{
		"--bg: #f5f7fb;",
		"--surface: #ffffff;",
		"--surface-muted: #fbfdff;",
		"--surface-2: #f8fafc;",
		"--text: #0f172a;",
		"--muted: #64748b;",
		"--line: #e2e8f0;",
		"--line-strong: #cbd5e1;",
	} {
		if !strings.Contains(css, token) {
			t.Fatalf("released v0.1.16 light palette token missing: %s", token)
		}
	}

	for _, marker := range []string{
		"background:#2563eb !important;",
		"color:#fff !important;",
		"border-color:#2563eb !important;",
		"background:#dbeafe !important;",
		"border-color:#93c5fd !important;",
		"background:#eef2ff !important",
		"border-color:#c7d2fe !important",
		"color:#3730a3 !important",
		"background:#fef2f2 !important",
		"border-color:#fecaca !important",
		"color:#b91c1c !important",
		"background:#dcfce7; color:#166534;",
		"background:#fee2e2; color:#991b1b;",
		"--line-subtle: #f1f5f9;",
		"background:var(--surface-muted) !important;",
		`.grok-inspection-page .field input[type="number"]`,
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("released v0.1.16 component color missing: %s", marker)
		}
	}

	dark := regexp.MustCompile(`(?s)html\[data-grok-theme="dark"\]\s*\{[^}]*\}`).FindString(css)
	if dark == "" {
		t.Fatal("released v0.1.16 dark palette tokens missing")
	}
	for _, token := range []string{
		"--bg:#111827;", "--surface:#182131;", "--surface-muted:#151d2b;", "--surface-2:#1d2737;",
		"--text:#f8fafc;", "--muted:#a7b3c7;", "--line:#334155;", "--line-strong:#475569;",
		"--line-subtle:#273449;",
	} {
		if !strings.Contains(dark, token) {
			t.Fatalf("released v0.1.16 dark palette token missing: %s", token)
		}
	}
	for _, marker := range []string{
		"background:#1e3a5f !important;",
		"border-color:#3b82f6 !important;",
		"background:#60a5fa;",
	} {
		if !strings.Contains(css, marker) {
			t.Fatalf("released v0.1.16 dark component color missing: %s", marker)
		}
	}

	for _, stale := range []string{
		"#f5f7fa", "#0f151d", "#eef4ff", "#0f8a66", "#a86108", "#c73838", "#7253c7",
	} {
		if strings.Contains(css, stale) {
			t.Fatalf("post-v0.1.16 palette color must not remain: %s", stale)
		}
	}
}
