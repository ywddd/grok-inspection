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
