package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestScheduleStatusInitialTextUsesI18n(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, `id="scheduleStatus"`) {
		t.Fatal("scheduleStatus missing")
	}
	if !strings.Contains(page, `id="scheduleStatus" class="schedule-status" data-i18n="schedule_loading"`) &&
		!strings.Contains(page, `data-i18n="schedule_loading"`) {
		t.Fatal("scheduleStatus must use data-i18n=schedule_loading so EN is not stuck on Chinese")
	}
	// Hardcoded Chinese without i18n attribute is the bug under review.
	re := regexp.MustCompile(`id="scheduleStatus"[^>]*>自动巡检状态加载中`)
	if re.MatchString(page) && !strings.Contains(page, `data-i18n="schedule_loading"`) {
		t.Fatal("scheduleStatus still hardcodes Chinese without i18n")
	}
	zh := extractI18NPack(page, "zh")
	en := extractI18NPack(page, "en")
	if strings.TrimSpace(zh["schedule_loading"]) == "" || strings.TrimSpace(en["schedule_loading"]) == "" {
		t.Fatal("schedule_loading missing in zh/en packs")
	}
	if strings.Contains(en["schedule_loading"], "自动巡检") {
		t.Fatalf("en schedule_loading still Chinese: %q", en["schedule_loading"])
	}
	if !strings.Contains(strings.ToLower(en["schedule_loading"]), "load") &&
		!strings.Contains(strings.ToLower(en["schedule_loading"]), "schedul") {
		t.Fatalf("en schedule_loading looks wrong: %q", en["schedule_loading"])
	}
	// applyStaticI18n must process data-i18n (already global); ensure key is wired.
	if !strings.Contains(page, "function applyStaticI18n") || !strings.Contains(page, "[data-i18n]") {
		t.Fatal("applyStaticI18n must apply data-i18n nodes including scheduleStatus")
	}
}

func TestMobileScheduleLongCheckboxesFullRowNoEllipsis(t *testing.T) {
	page := string(renderUIPage(pluginName))
	css := extractUICSS(page)
	// 640px media is the 390px phone contract.
	if !strings.Contains(css, "@media (max-width:640px)") {
		t.Fatal("missing 640px media")
	}
	mobile := cssMobile640(css)
	for _, marker := range []string{
		`#scheduleEnabled`,
		`#scheduleAutoRecoverHealthy`,
		`grid-column:1 / -1`,
		`white-space:normal`,
	} {
		if !strings.Contains(mobile, marker) {
			t.Fatalf("mobile schedule long-checkbox contract missing %q", marker)
		}
	}
	// Those two controls must not keep ellipsis on their labels inside the mobile rule.
	// Allow ellipsis elsewhere; require the :has(#scheduleEnabled) span rule to use normal wrap.
	re := regexp.MustCompile(`#scheduleEnabled\)[^{]*span[^{]*\{[^}]*white-space:\s*normal`)
	if !re.MatchString(mobile) && !strings.Contains(mobile, "#scheduleEnabled) span") {
		// looser: both ids appear near white-space:normal
		if !regexp.MustCompile(`scheduleEnabled[\s\S]{0,200}white-space:\s*normal`).MatchString(mobile) {
			t.Fatal("scheduleEnabled mobile label must allow white-space:normal (full English text)")
		}
	}
	if !regexp.MustCompile(`scheduleAutoRecoverHealthy[\s\S]{0,200}white-space:\s*normal`).MatchString(mobile) {
		t.Fatal("scheduleAutoRecoverHealthy mobile label must allow white-space:normal")
	}
	// Desktop may wrap naturally, but the mobile-only full-row :has rules stay scoped to 640px.
	outside := cssOutsideMobile(page)
	if strings.Contains(outside, "#scheduleAutoRecoverHealthy") && strings.Contains(outside, "grid-column:1 / -1") {
		// only fail if the full-row rule is outside mobile
		if regexp.MustCompile(`#scheduleAutoRecoverHealthy[\s\S]{0,120}grid-column:\s*1\s*/\s*-1`).MatchString(outside) {
			t.Fatal("full-row schedule checkbox rules must stay inside max-width:640px media")
		}
	}
}

func TestMobileEnglishTabTitlesFullyVisible(t *testing.T) {
	page := string(renderUIPage(pluginName))
	css := extractUICSS(page)
	mobile := cssMobile640(css)
	en := extractI18NPack(page, "en")
	for _, key := range []string{"tab_inspect", "tab_autoban"} {
		if strings.TrimSpace(en[key]) == "" {
			t.Fatalf("en %s missing", key)
		}
	}
	// Title rules in mobile: no nowrap+ellipsis on .tab-title (allow wrap / full text).
	// Desc may keep ellipsis.
	if regexp.MustCompile(`\.tab \.tab-title[^{]*\{[^}]*text-overflow:\s*ellipsis`).MatchString(mobile) &&
		regexp.MustCompile(`\.tab \.tab-title[^{]*\{[^}]*white-space:\s*nowrap`).MatchString(mobile) {
		t.Fatal("mobile .tab-title must not force nowrap+ellipsis (English titles truncate at 390px)")
	}
	if !regexp.MustCompile(`\.tab \.tab-title[^{]*\{[^}]*white-space:\s*normal`).MatchString(mobile) &&
		!strings.Contains(mobile, ".tab .tab-title") {
		t.Fatal("mobile tab-title rule missing")
	}
	if !regexp.MustCompile(`\.tab \.tab-title[^{]*\{[^}]*white-space:\s*normal`).MatchString(mobile) {
		t.Fatal("mobile .tab-title should use white-space:normal for full main titles")
	}
	// Desc still ellipsizes reasonably.
	if !regexp.MustCompile(`\.tab \.tab-desc[^{]*\{[^}]*text-overflow:\s*ellipsis`).MatchString(mobile) &&
		!regexp.MustCompile(`tab-desc[\s\S]{0,80}text-overflow:\s*ellipsis`).MatchString(mobile) {
		t.Fatal("mobile tab-desc should keep ellipsis")
	}
	// English titles are longer than Chinese; ensure i18n values are the expected full strings.
	if en["tab_inspect"] != "Account inspection" {
		t.Fatalf("en tab_inspect=%q", en["tab_inspect"])
	}
	if en["tab_autoban"] != "Realtime auto-ban" {
		t.Fatalf("en tab_autoban=%q", en["tab_autoban"])
	}
}

func TestDesktopScheduleControlsWrapInsteadOfClipping(t *testing.T) {
	css := extractUICSS(string(renderUIPage(pluginName)))
	for _, selector := range []string{`.schedule-controls`, `.schedule-actions`} {
		re := regexp.MustCompile(regexp.QuoteMeta(selector) + `\s*\{[^}]*flex-wrap:\s*wrap[^}]*overflow:\s*visible`)
		if !re.MatchString(css) {
			t.Fatalf("%s must wrap and remain visible for long English schedule labels", selector)
		}
	}
	if !regexp.MustCompile(`\.schedule-controls\s*\{[^}]*flex:\s*1\s+1\s+0`).MatchString(css) {
		t.Fatal("schedule controls must shrink beside the save button before wrapping internally")
	}
}

// cssMobile640 returns the body of the max-width:640px media block.
func cssMobile640(css string) string {
	idx := strings.Index(css, "@media (max-width:640px)")
	if idx < 0 {
		idx = strings.Index(css, "@media (max-width: 640px)")
	}
	if idx < 0 {
		return ""
	}
	j := idx + len("@media (max-width:640px)")
	for j < len(css) && css[j] != '{' {
		j++
	}
	if j >= len(css) {
		return ""
	}
	depth := 0
	k := j
	for k < len(css) {
		switch css[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[j : k+1]
			}
		}
		k++
	}
	return css[j:]
}


func TestMobileAutobanCategoryActionButtonsFullRow(t *testing.T) {
	page := string(renderUIPage(pluginName))
	css := extractUICSS(page)
	mobile := cssMobile640(css)
	// Long category action labels must use a single full-width column on phones.
	if !strings.Contains(mobile, ".autoban-action-buttons { display:grid; grid-template-columns:1fr;") {
		t.Fatal("mobile .autoban-action-buttons must be single-column (1fr)")
	}
	if strings.Contains(mobile, ".autoban-action-buttons { display:grid; grid-template-columns:1fr 1fr") {
		t.Fatal("mobile .autoban-action-buttons must not keep unused 1fr 1fr grid")
	}
	for _, id := range []string{"#banUnbanFilterBtn", "#banDeleteFilterBtn"} {
		if !strings.Contains(mobile, id) {
			t.Fatalf("mobile CSS missing %s", id)
		}
		re := regexp.MustCompile(regexp.QuoteMeta(id) + `[\s\S]{0,220}white-space:\s*normal`)
		if !re.MatchString(mobile) {
			t.Fatalf("mobile %s must allow white-space:normal for long category labels", id)
		}
	}
}
