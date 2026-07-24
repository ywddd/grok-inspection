package main

import (
	"strings"
	"testing"
)

func TestUI18nLookupKeepsEnglishEmptyStrings(t *testing.T) {
	page := string(renderUIPage(pluginName))

	// Guard against the truthy fallback that mixed EN pager copy with ZH fragments.
	for _, bad := range []string{
		`(pack && pack[key]) || (I18N.zh && I18N.zh[key])`,
		`(pack && pack[key]) || (REASON_I18N.zh && REASON_I18N.zh[key])`,
	} {
		if strings.Contains(page, bad) {
			t.Fatalf("i18n lookup still uses truthy fallback: %s", bad)
		}
	}
	for _, good := range []string{
		`Object.prototype.hasOwnProperty.call(pack, key)`,
		`Object.prototype.hasOwnProperty.call(I18N.zh, key)`,
		`Object.prototype.hasOwnProperty.call(REASON_I18N.zh, key)`,
	} {
		if !strings.Contains(page, good) {
			t.Fatalf("i18n lookup missing key-presence check: %s", good)
		}
	}

	en := extractI18NPack(page, "en")
	zh := extractI18NPack(page, "zh")
	// These EN values are intentionally empty; truthy lookup would fall back to Chinese.
	for _, key := range []string{"pager_total_prefix", "pager_page_suffix", "unban_filter_started_suffix"} {
		if _, ok := en[key]; !ok {
			t.Fatalf("en %s missing from I18N pack", key)
		}
		if en[key] != "" {
			t.Fatalf("en %s = %q, want empty string", key, en[key])
		}
		if strings.TrimSpace(zh[key]) == "" {
			t.Fatalf("zh %s should be non-empty so a buggy fallback is observable", key)
		}
	}

	// Dynamic ban pager fragment in English must not contain Chinese snippets.
	frag := en["pager_total_prefix"] + "0" + en["pager_total_suffix"] +
		en["pager_page_mid"] + "1" + en["pager_page_of"] + "1" + en["pager_page_suffix"] +
		en["per_page"]
	want := "0 total · page 1 / 1 · per page "
	// Allow either middle-dot variants already present in pack.
	if !strings.Contains(frag, "0 total") || !strings.Contains(frag, "page 1") || !strings.Contains(frag, "/ 1") || !strings.Contains(strings.ToLower(frag), "per page") {
		t.Fatalf("english pager fragment unexpected: %q (ref %q)", frag, want)
	}
	for _, zhFrag := range []string{"共", "个", "页", "每页"} {
		if strings.Contains(frag, zhFrag) {
			t.Fatalf("english dynamic pager contains Chinese %q: %q", zhFrag, frag)
		}
	}

	// Ban pager still composes from these keys (integration surface for the bug).
	for _, marker := range []string{
		`t('pager_total_prefix')`,
		`t('pager_total_suffix')`,
		`t('pager_page_mid')`,
		`t('pager_page_of')`,
		`t('pager_page_suffix')`,
		`t('per_page')`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("ban pager missing %s", marker)
		}
	}
}
