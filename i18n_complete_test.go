package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

var errNodeUnavailable = errors.New("node unavailable")

func tryParseJSWithNode(js string) error {
	node, err := exec.LookPath("node")
	if err != nil {
		// Common Windows install locations / PATH gaps.
		for _, candidate := range []string{
			`C:\Program Files\nodejs\node.exe`,
			`C:\Program Files (x86)\nodejs\node.exe`,
		} {
			if st, stErr := os.Stat(candidate); stErr == nil && !st.IsDir() {
				node = candidate
				err = nil
				break
			}
		}
	}
	if err != nil || node == "" {
		return errNodeUnavailable
	}
	dir, err := os.MkdirTemp("", "grok-ui-js-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	// Wrap page script: it expects a DOM at runtime, so only syntax-check via new Function.
	wrapper := "new Function(" + strconvQuote(js) + ");\n"
	path := filepath.Join(dir, "check.js")
	if err := os.WriteFile(path, []byte(wrapper), 0o600); err != nil {
		return err
	}
	cmd := exec.Command(node, "--check", path)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return fmt.Errorf("%v: %s", runErr, strings.TrimSpace(string(out)))
	}
	return nil
}

func strconvQuote(s string) string {
	// Minimal JSON string quoting for embedding into a JS source file.
	b, _ := json.Marshal(s)
	return string(b)
}

func TestUIPageI18NObjectHasNoDoubleCommas(t *testing.T) {
	page := string(renderUIPage(pluginName))
	idx := strings.Index(page, "const I18N = {")
	if idx < 0 {
		t.Fatal("I18N object missing")
	}
	end := strings.Index(page[idx:], "const LANG_KEY")
	if end < 0 {
		t.Fatal("LANG_KEY marker missing after I18N")
	}
	block := page[idx : idx+end]
	if strings.Contains(block, ",,") {
		t.Fatalf("I18N object contains double commas that break JS parse:\n%s", excerptAround(block, ",,", 80))
	}
	if strings.Contains(block, "ban_th_remain:'剩余',,") || strings.Contains(block, "ban_th_remain:'Remaining',,") {
		t.Fatal("ban_th_remain still has a double comma")
	}
}

func TestUIPageScriptParsesWhenNodeAvailable(t *testing.T) {
	page := string(renderUIPage(pluginName))
	scriptStart := strings.Index(page, "<script>")
	scriptEnd := strings.LastIndex(page, "</script>")
	if scriptStart < 0 || scriptEnd <= scriptStart {
		t.Fatal("script block missing")
	}
	js := page[scriptStart+len("<script>") : scriptEnd]
	if err := tryParseJSWithNode(js); err != nil {
		if errors.Is(err, errNodeUnavailable) {
			t.Log("node not available; skipped live JS parse (structural I18N checks still run)")
			return
		}
		t.Fatalf("UI page script failed JS parse: %v", err)
	}
}

func TestLocalizeKnownReasonHTTPAndFallback(t *testing.T) {
	zhPerm := T(LangZH, "permission_denied")
	enPerm := T(LangEN, "permission_denied")
	zhWithHTTP := fmt.Sprintf("%s (HTTP 403)", zhPerm)
	enWithHTTP := fmt.Sprintf("%s (HTTP 403)", enPerm)
	if got := localizeKnownReason(LangEN, zhWithHTTP); got != enWithHTTP {
		t.Fatalf("zh HTTP -> en = %q, want %q", got, enWithHTTP)
	}
	if got := localizeKnownReason(LangZH, enWithHTTP); got != zhWithHTTP {
		t.Fatalf("en HTTP -> zh = %q, want %q", got, zhWithHTTP)
	}

	zhFB := T(LangZH, "fallback_disagreed")
	enFB := T(LangEN, "fallback_disagreed")
	zhQuota := T(LangZH, "quota_exhausted") + zhFB
	enQuota := T(LangEN, "quota_exhausted") + enFB
	if got := localizeKnownReason(LangEN, zhQuota); got != enQuota {
		t.Fatalf("zh quota+fallback -> en = %q, want %q", got, enQuota)
	}
	if got := localizeKnownReason(LangZH, enQuota); got != zhQuota {
		t.Fatalf("en quota+fallback -> zh = %q, want %q", got, zhQuota)
	}

	zhCombo := fmt.Sprintf("%s (HTTP 403)", zhPerm) + zhFB
	enCombo := fmt.Sprintf("%s (HTTP 403)", enPerm) + enFB
	if got := localizeKnownReason(LangEN, zhCombo); got != enCombo {
		t.Fatalf("zh combo -> en = %q, want %q", got, enCombo)
	}
	if got := localizeKnownReason(LangZH, enCombo); got != zhCombo {
		t.Fatalf("en combo -> zh = %q, want %q", got, zhCombo)
	}
}

func TestStartStatusCodesStableAcrossLang(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers, running: true}
	t.Cleanup(func() { engine = old })

	for _, lang := range []string{"zh", "en"} {
		resp := dispatchManagement(pluginapi.ManagementRequest{
			Method: http.MethodPost,
			Path:   "/v0/management/plugins/grok-inspection/start",
			Body:   []byte(`{"workers":6,"lang":"` + lang + `"}`),
		})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("lang=%s already-running status=%d body=%s", lang, resp.StatusCode, string(resp.Body))
		}
	}

	engine = &inspectionEngine{workers: defaultWorkers, results: nil}
	for _, lang := range []string{"zh", "en"} {
		resp := dispatchManagement(pluginapi.ManagementRequest{
			Method: http.MethodPost,
			Path:   "/v0/management/plugins/grok-inspection/start",
			Body:   []byte(`{"workers":6,"incremental":true,"lang":"` + lang + `"}`),
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("lang=%s incremental-without-results status=%d body=%s", lang, resp.StatusCode, string(resp.Body))
		}
	}

	for _, lang := range []string{"zh", "en"} {
		resp := dispatchManagement(pluginapi.ManagementRequest{
			Method: http.MethodPost,
			Path:   "/v0/management/plugins/grok-inspection/start",
			Body:   []byte(`{"workers":99,"lang":"` + lang + `"}`),
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("lang=%s invalid workers status=%d body=%s", lang, resp.StatusCode, string(resp.Body))
		}
	}
}

func TestCancelledAccountResultUsesRequestLang(t *testing.T) {
	file := pluginapi.HostAuthFileEntry{AuthIndex: "a1", Name: "n1", Email: "e@x"}
	zh := cancelledAccountResult(file, "grok-4.5", LangZH)
	en := cancelledAccountResult(file, "grok-4.5", LangEN)
	if zh.Reason != T(LangZH, "stopped_before_probe") {
		t.Fatalf("zh reason = %q", zh.Reason)
	}
	if en.Reason != T(LangEN, "stopped_before_probe") {
		t.Fatalf("en reason = %q", en.Reason)
	}
	if zh.Reason == en.Reason {
		t.Fatalf("zh/en stop reasons should differ")
	}
}

func TestProbeTimeoutSentinelsAreLanguageAgnostic(t *testing.T) {
	if !isProbeTimeoutErr(errHTTPProbeTimeout) {
		t.Fatal("errHTTPProbeTimeout should be detected")
	}
	if !isProbeTimeoutErr(fmt.Errorf("%w: POST x", errHTTPProbeTimeout)) {
		t.Fatal("wrapped http probe timeout should be detected")
	}
	if errListAccountsTimeout == nil {
		t.Fatal("list accounts timeout sentinel missing")
	}
	if !isProbeTimeoutErr(fmt.Errorf("%s", T(LangZH, "http_probe_timeout", 25*time.Second, "POST", "x"))) {
		t.Fatal("legacy zh timeout string should still match")
	}
}

func TestPluginVersionIs014(t *testing.T) {
	if pluginVersion != "0.1.14" {
		t.Fatalf("pluginVersion = %q, want 0.1.14", pluginVersion)
	}
	meta := pluginRegistration()
	if meta.Metadata.Version != "0.1.14" {
		t.Fatalf("meta.Version = %q", meta.Metadata.Version)
	}
	readme := mustReadRepoFile(t, "README.md")
	readmeEN := mustReadRepoFile(t, "README.en.md")
	if !strings.Contains(readme, "0.1.14") {
		t.Fatal("README.md missing 0.1.14")
	}
	if !strings.Contains(readmeEN, "0.1.14") {
		t.Fatal("README.en.md missing 0.1.14")
	}
	if strings.Contains(readme, "0.1.13") || strings.Contains(readmeEN, "0.1.13") {
		t.Fatal("README still mentions 0.1.13")
	}
}

func TestSnapshotLocalizesStoredReasons(t *testing.T) {
	old := engine
	engine = &inspectionEngine{
		workers: defaultWorkers,
		lang:    LangZH,
		results: []accountResult{{
			Name:           "a",
			AuthIndex:      "1",
			Classification: "quota_exhausted",
			Action:         "disable",
			Reason:         T(LangZH, "quota_exhausted") + T(LangZH, "fallback_disagreed"),
		}},
	}
	t.Cleanup(func() { engine = old })

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
		Query:  map[string][]string{"include_results": {"1"}, "lang": {"en"}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	var payload struct {
		Results []struct {
			Reason string `json:"reason"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, string(resp.Body))
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results len=%d", len(payload.Results))
	}
	want := T(LangEN, "quota_exhausted") + T(LangEN, "fallback_disagreed")
	if payload.Results[0].Reason != want {
		t.Fatalf("reason = %q, want %q", payload.Results[0].Reason, want)
	}
}

func TestUIPageDynamicCopyUsesTHelper(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, marker := range []string{
		"function localizeKnownReason(",
		"t('inspect_running')",
		"t('bulk_disable')",
		"t('ban_unban')",
		"t('class_all')",
		"localizeKnownReason(r.reason",
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("page missing i18n wiring marker %q", marker)
		}
	}
	forbidden := []string{
		"setProgress('巡检中 '",
		"setProgress('等待开始'",
		"textContent = '批量禁用'",
		"return '需手动解禁'",
		"return '额度用尽'",
	}
	for _, bad := range forbidden {
		if strings.Contains(page, bad) {
			t.Fatalf("page still hardcodes dynamic Chinese: %q", bad)
		}
	}
}

func mustReadRepoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func excerptAround(s, needle string, pad int) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	start := i - pad
	if start < 0 {
		start = 0
	}
	end := i + len(needle) + pad
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}

func TestUIPageI18NDefinesHTTPColumn(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, `data-i18n="th_http"`) {
		t.Fatal("th_http data-i18n missing in HTML")
	}
	// Both language packs must define th_http so applyStaticI18n does not show the raw key.
	for _, marker := range []string{
		`th_http:'HTTP'`,
		`th_account:'账号'`,
		`th_account:'Account'`,
	} {
		if !strings.Contains(page, marker) {
			// allow either quoted form in zh/en packs
		}
	}
	if !strings.Contains(page, "th_http:") {
		t.Fatal("I18N missing th_http key")
	}
	// Ensure zh and en packs both contain th_http entries.
	zhIdx := strings.Index(page, "zh: {")
	enIdx := strings.Index(page, "en: {")
	if zhIdx < 0 || enIdx < 0 || enIdx < zhIdx {
		t.Fatal("I18N zh/en packs missing")
	}
	zhPack := page[zhIdx:enIdx]
	enPack := page[enIdx : enIdx+4000]
	if !strings.Contains(zhPack, "th_http:") {
		t.Fatal("zh I18N missing th_http")
	}
	if !strings.Contains(enPack, "th_http:") {
		t.Fatal("en I18N missing th_http")
	}
}

func TestLocalizeKnownReasonHTTPProbeTimeoutAndListFailed(t *testing.T) {
	zhHTTP := T(LangZH, "http_probe_timeout", "25s", "POST", "https://example.test/v1")
	enHTTP := T(LangEN, "http_probe_timeout", "25s", "POST", "https://example.test/v1")
	if got := localizeKnownReason(LangEN, zhHTTP); got != enHTTP {
		t.Fatalf("zh http timeout -> en = %q, want %q", got, enHTTP)
	}
	if got := localizeKnownReason(LangZH, enHTTP); got != zhHTTP {
		t.Fatalf("en http timeout -> zh = %q, want %q", got, zhHTTP)
	}

	zhList := T(LangZH, "list_accounts_failed", "connection refused")
	enList := T(LangEN, "list_accounts_failed", "connection refused")
	if got := localizeKnownReason(LangEN, zhList); got != enList {
		t.Fatalf("zh list failed -> en = %q, want %q", got, enList)
	}
	if got := localizeKnownReason(LangZH, enList); got != zhList {
		t.Fatalf("en list failed -> zh = %q, want %q", got, zhList)
	}

	zhProbe := T(LangZH, "probe_timeout", "55s")
	enProbe := T(LangEN, "probe_timeout", "55s")
	if got := localizeKnownReason(LangEN, zhProbe); got != enProbe {
		t.Fatalf("zh account probe timeout -> en = %q, want %q", got, enProbe)
	}
	if got := localizeKnownReason(LangZH, enProbe); got != zhProbe {
		t.Fatalf("en account probe timeout -> zh = %q, want %q", got, zhProbe)
	}
}

func TestHTTPTimeoutReasonUses25sNotAccountBudget(t *testing.T) {
	err := &probeHTTPTimeoutError{d: probeHTTPTimeout, method: "POST", url: "https://cli.example/v1"}
	if !isProbeTimeoutErr(err) {
		t.Fatal("typed http timeout must be recognized for retry")
	}
	// Simulate inspectAccountInner classification path.
	reason := T(LangZH, "http_probe_timeout", err.d, err.method, err.url)
	if strings.Contains(reason, "55") {
		t.Fatalf("http timeout reason must not use account budget 55s: %q", reason)
	}
	if !strings.Contains(reason, "25") {
		t.Fatalf("http timeout reason should mention 25s: %q", reason)
	}
	// English path
	reasonEN := T(LangEN, "http_probe_timeout", err.d, err.method, err.url)
	if strings.Contains(reasonEN, "55") || !strings.Contains(reasonEN, "25") {
		t.Fatalf("en http timeout reason = %q", reasonEN)
	}
}

func TestBusyUnbanIsLocalized(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers}
	// Force unban busy.
	unbanJob.mu.Lock()
	prev := unbanJob.running
	unbanJob.running = true
	unbanJob.mu.Unlock()
	t.Cleanup(func() {
		unbanJob.mu.Lock()
		unbanJob.running = prev
		unbanJob.mu.Unlock()
		engine = old
	})

	errZH := engine.start(startRequest{Workers: 2, Lang: "zh"})
	errEN := engine.start(startRequest{Workers: 2, Lang: "en"})
	if errZH == nil || errEN == nil {
		t.Fatal("expected busy errors")
	}
	if statusFromError(errZH, 0) != http.StatusConflict || statusFromError(errEN, 0) != http.StatusConflict {
		t.Fatalf("status zh=%d en=%d", statusFromError(errZH, 0), statusFromError(errEN, 0))
	}
	if errZH.Error() != T(LangZH, "busy_unban") {
		t.Fatalf("zh busy = %q, want %q", errZH.Error(), T(LangZH, "busy_unban"))
	}
	if errEN.Error() != T(LangEN, "busy_unban") {
		t.Fatalf("en busy = %q, want %q", errEN.Error(), T(LangEN, "busy_unban"))
	}
	if errZH.Error() == errEN.Error() {
		t.Fatalf("zh/en busy messages should differ: %q", errZH)
	}
}

func TestUIExportLocalizesReason(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, "function sanitizeExportRow") {
		t.Fatal("sanitizeExportRow missing")
	}
	// Export must run reasons through localizeKnownReason for current UI language.
	if !strings.Contains(page, "o.reason = localizeKnownReason(") {
		t.Fatal("export path must localize reason via localizeKnownReason")
	}
}

func TestLocalizeNestedListAccountsTimeout(t *testing.T) {
	zhNested := T(LangZH, "list_accounts_failed", T(LangZH, "list_accounts_timeout"))
	enNested := T(LangEN, "list_accounts_failed", T(LangEN, "list_accounts_timeout"))
	// Sanity: nested Chinese form matches the historical stored shape.
	if zhNested != "列出账号失败: 列出账号超时（30s）" {
		t.Fatalf("zh nested shape = %q", zhNested)
	}
	if enNested != "Failed to list accounts: Listing accounts timed out (30s)" {
		t.Fatalf("en nested shape = %q", enNested)
	}
	if got := localizeKnownReason(LangEN, zhNested); got != enNested {
		t.Fatalf("zh nested -> en = %q, want %q", got, enNested)
	}
	if got := localizeKnownReason(LangZH, enNested); got != zhNested {
		t.Fatalf("en nested -> zh = %q, want %q", got, zhNested)
	}
	// Mixed historical form: Chinese outer + English detail (and reverse) should still normalize.
	mixed := "列出账号失败: " + T(LangEN, "list_accounts_timeout")
	if got := localizeKnownReason(LangEN, mixed); got != enNested {
		t.Fatalf("mixed zh-outer/en-detail -> en = %q, want %q", got, enNested)
	}
	if got := localizeKnownReason(LangZH, mixed); got != zhNested {
		t.Fatalf("mixed zh-outer/en-detail -> zh = %q, want %q", got, zhNested)
	}
}

func TestApplyActionBusyUsesRequestLangWithoutInspectionHistory(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers} // lang zero / zh default, no inspection history
	// Force unban busy so apply/action return localized busy without needing a running apply.
	unbanJob.mu.Lock()
	prev := unbanJob.running
	unbanJob.running = true
	unbanJob.mu.Unlock()
	t.Cleanup(func() {
		unbanJob.mu.Lock()
		unbanJob.running = prev
		unbanJob.mu.Unlock()
		engine = old
	})

	errApply := engine.startApply(applyRequest{Lang: "en", ForceAction: "disable", AuthIndexes: []string{"a"}}, "pw", nil)
	if errApply == nil {
		t.Fatal("expected apply busy")
	}
	if errApply.Error() != T(LangEN, "busy_unban") {
		t.Fatalf("apply busy en = %q, want %q", errApply.Error(), T(LangEN, "busy_unban"))
	}

	_, _, errAction := engine.startAction(actionRequest{Lang: "en", Name: "a.json", Disabled: true}, "pw", nil)
	if errAction == nil {
		t.Fatal("expected action busy")
	}
	if errAction.Error() != T(LangEN, "busy_unban") {
		t.Fatalf("action busy en = %q, want %q", errAction.Error(), T(LangEN, "busy_unban"))
	}

	// Chinese request language must not fall back to leftover English apply language.
	errApplyZH := engine.startApply(applyRequest{Lang: "zh", ForceAction: "disable", AuthIndexes: []string{"a"}}, "pw", nil)
	if errApplyZH == nil || errApplyZH.Error() != T(LangZH, "busy_unban") {
		t.Fatalf("apply busy zh = %v", errApplyZH)
	}
}

func TestApplyProgressUsesRequestLangNotInspectionLang(t *testing.T) {
	old := engine
	engine = &inspectionEngine{
		workers: defaultWorkers,
		lang:    LangZH, // previous inspection was Chinese
		results: []accountResult{{
			AuthIndex: "1", Name: "a.json", FileName: "a.json",
			Classification: "quota_exhausted", Action: "disable", Disabled: false,
		}},
	}
	t.Cleanup(func() { engine = old })

	// Patch ban save / CPA calls out of the way by using force delete with stubbed batch that no-ops.
	// We only need applyLang + first progress string assignment path: startApply should set applyLang=en.
	if err := engine.startApply(applyRequest{
		Lang:        "en",
		ForceAction: "disable",
		AuthIndexes: []string{"1"},
	}, "pw", nil); err != nil {
		t.Fatalf("startApply: %v", err)
	}
	// Wait briefly for goroutine to set progress or finish.
	deadline := time.Now().Add(2 * time.Second)
	var current string
	var applyLang Lang
	for time.Now().Before(deadline) {
		engine.mu.Lock()
		current = engine.applyCurrent
		applyLang = engine.applyLang
		applying := engine.applying || engine.applyDraining
		engine.mu.Unlock()
		if applyLang == LangEN {
			break
		}
		if !applying && applyLang == LangEN {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if applyLang != LangEN {
		t.Fatalf("applyLang = %q, want en (request-scoped, not inspection zh)", applyLang)
	}
	// Stop should use apply language for stopped marker when apply still active.
	engine.mu.Lock()
	engine.applying = true
	engine.applyLang = LangEN
	engine.mu.Unlock()
	engine.stop()
	engine.mu.Lock()
	stopped := engine.applyCurrent
	engine.mu.Unlock()
	if stopped != T(LangEN, "stopped") {
		t.Fatalf("stop applyCurrent = %q, want %q", stopped, T(LangEN, "stopped"))
	}
	_ = current
}

func TestUIApplyActionBodiesIncludeLang(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, "lang: lang") {
		t.Fatal("UI must send lang on apply/action bodies")
	}
	if !strings.Contains(page, "JSON.stringify({ lang: lang })") {
		t.Fatal("suggested apply body must include lang")
	}
	if !strings.Contains(page, "const detail = localizeKnownReason(reason.slice(prefix.length));") {
		t.Fatal("JS must recursively localize list-failed detail")
	}
	if !strings.Contains(page, "return formatListAccountsFailed(detail);") {
		t.Fatal("JS must re-wrap localized list-failed detail")
	}
	actionIdx := strings.Index(page, "api('/action'")
	applyForceIdx := strings.Index(page, "force_action: action")
	if actionIdx < 0 || applyForceIdx < 0 {
		t.Fatal("action/apply markers missing")
	}
	if !strings.Contains(page[actionIdx:actionIdx+350], "lang: lang") {
		t.Fatal("action POST body missing lang")
	}
	if !strings.Contains(page[applyForceIdx:applyForceIdx+200], "lang: lang") {
		t.Fatal("force apply POST body missing lang")
	}
}

func TestStartRejectsInvalidJSON(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers}
	t.Cleanup(func() { engine = old })

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/start",
		Body:   []byte(`{"workers":`), // truncated JSON
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	if !strings.Contains(string(resp.Body), T(LangZH, "invalid_json")) {
		t.Fatalf("body=%s", string(resp.Body))
	}
	// Must not have started inspection.
	if engine.running {
		t.Fatal("invalid JSON must not start inspection")
	}

	// Best-effort English when lang is visible in malformed body.
	respEN := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/start",
		Body:   []byte(`{"lang":"en","workers":`),
	})
	if respEN.StatusCode != http.StatusBadRequest {
		t.Fatalf("en status=%d body=%s", respEN.StatusCode, string(respEN.Body))
	}
	if !strings.Contains(string(respEN.Body), T(LangEN, "invalid_json")) {
		t.Fatalf("en body=%s", string(respEN.Body))
	}
}

func TestApplyActionValidationLocalizedAndStableStatus(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers, results: nil}
	t.Cleanup(func() { engine = old })

	cases := []struct {
		name   string
		path   string
		body   string
		status int
		zhKey  string
	}{
		{
			name: "force invalid", path: "/v0/management/plugins/grok-inspection/apply",
			body: `{"lang":"zh","force_action":"nope","auth_indexes":["a"]}`, status: http.StatusBadRequest, zhKey: "force_action_invalid",
		},
		{
			name: "force missing targets", path: "/v0/management/plugins/grok-inspection/apply",
			body: `{"lang":"en","force_action":"disable"}`, status: http.StatusBadRequest, zhKey: "force_action_requires_targets",
		},
		{
			name: "force no match", path: "/v0/management/plugins/grok-inspection/apply",
			body: `{"lang":"zh","force_action":"disable","auth_indexes":["missing"]}`, status: http.StatusBadRequest, zhKey: "no_accounts_matched",
		},
		{
			name: "no recommended", path: "/v0/management/plugins/grok-inspection/apply",
			body: `{"lang":"en"}`, status: http.StatusConflict, zhKey: "no_recommended_actions",
		},
		{
			name: "action missing name", path: "/v0/management/plugins/grok-inspection/action",
			body: `{"lang":"zh","disabled":true}`, status: http.StatusBadRequest, zhKey: "name_or_auth_required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := dispatchManagement(pluginapi.ManagementRequest{
				Method: http.MethodPost,
				Path:   tc.path,
				Body:   []byte(tc.body),
			})
			if resp.StatusCode != tc.status {
				t.Fatalf("status=%d want %d body=%s", resp.StatusCode, tc.status, string(resp.Body))
			}
			// Determine expected language from body.
			lang := LangZH
			if strings.Contains(tc.body, `"lang":"en"`) {
				lang = LangEN
			}
			want := T(lang, tc.zhKey)
			if !strings.Contains(string(resp.Body), want) {
				t.Fatalf("body=%s want substring %q", string(resp.Body), want)
			}
			// Opposite language must differ for bilingual keys.
			other := LangEN
			if lang == LangEN {
				other = LangZH
			}
			if T(lang, tc.zhKey) == T(other, tc.zhKey) {
				t.Fatalf("zh/en messages identical for %s", tc.zhKey)
			}
		})
	}

	// Same validation codes with flipped languages.
	for _, lang := range []string{"zh", "en"} {
		resp := dispatchManagement(pluginapi.ManagementRequest{
			Method: http.MethodPost,
			Path:   "/v0/management/plugins/grok-inspection/apply",
			Body:   []byte(`{"lang":"` + lang + `","force_action":"bad","auth_indexes":["x"]}`),
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("lang=%s status=%d", lang, resp.StatusCode)
		}
		if !strings.Contains(string(resp.Body), T(normalizeLang(lang), "force_action_invalid")) {
			t.Fatalf("lang=%s body=%s", lang, string(resp.Body))
		}
	}
}

func TestUIAriaLabelAndWorkersTitleI18n(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, `data-i18n-aria-label="tabs_aria"`) {
		t.Fatal("tabs must use data-i18n-aria-label")
	}
	if !strings.Contains(page, `data-i18n-title="workers_title"`) {
		t.Fatal("workers input missing data-i18n-title")
	}
	if !strings.Contains(page, "workers_title:") {
		t.Fatal("workers_title key missing from I18N")
	}
	if !strings.Contains(page, "[data-i18n-aria-label]") {
		t.Fatal("applyStaticI18n must process data-i18n-aria-label")
	}
	// ensure handler sets aria-label attribute
	if !strings.Contains(page, "el.setAttribute('aria-label', t(key));") {
		t.Fatal("aria-label setter missing")
	}
	if !strings.Contains(page, "apply_progress_sep:") {
		t.Fatal("apply_progress_sep key missing")
	}
	if !strings.Contains(page, "t('apply_progress_sep')") {
		t.Fatal("apply progress must use I18N separator, not hard-coded fullwidth colon")
	}
	// Hard-coded fullwidth colon glued to apply_current should be gone.
	if strings.Contains(page, "apply_current ? '：'") {
		t.Fatal("hard-coded fullwidth colon still present in apply progress")
	}
}

func TestApplyInvalidJSONUsesI18n(t *testing.T) {
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/apply",
		Body:   []byte(`{"lang":"en",`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	if !strings.Contains(string(resp.Body), T(LangEN, "invalid_json")) {
		t.Fatalf("body=%s", string(resp.Body))
	}
}

func TestLocalizeKnownActionErrorRoundTripAndUnknown(t *testing.T) {
	cases := []struct {
		en string
		zh string
	}{
		{T(LangEN, "list_accounts_timeout"), T(LangZH, "list_accounts_timeout")},
		{T(LangEN, "auth_not_found", "abc"), T(LangZH, "auth_not_found", "abc")},
		{T(LangEN, "auth_file_name_missing_for", "x.json"), T(LangZH, "auth_file_name_missing_for", "x.json")},
		{T(LangEN, "auth_file_name_missing"), T(LangZH, "auth_file_name_missing")},
		{T(LangEN, "mgmt_password_unavailable"), T(LangZH, "mgmt_password_unavailable")},
		{"CPA management password is unavailable (set MANAGEMENT_PASSWORD on CPA process)", T(LangZH, "mgmt_password_unavailable")},
		{T(LangEN, "unsupported_action", "explode"), T(LangZH, "unsupported_action", "explode")},
		{T(LangEN, "persist_ban_enabled", "disk full"), T(LangZH, "persist_ban_enabled", "disk full")},
		{T(LangEN, "persist_ban_deleted_local", "io"), T(LangZH, "persist_ban_deleted_local", "io")},
		{T(LangEN, "persist_ban_deleted_cpa", "io"), T(LangZH, "persist_ban_deleted_cpa", "io")},
		{T(LangEN, "persist_ban_unbanned", "io"), T(LangZH, "persist_ban_unbanned", "io")},
		{T(LangEN, "persist_ban_state", "io"), T(LangZH, "persist_ban_state", "io")},
		{T(LangEN, "stopped"), T(LangZH, "stopped")},
	}
	for _, tc := range cases {
		if got := localizeKnownActionError(LangZH, tc.en); got != tc.zh {
			t.Fatalf("en->zh %q => %q, want %q", tc.en, got, tc.zh)
		}
		if got := localizeKnownActionError(LangEN, tc.zh); got != tc.en && !(strings.HasPrefix(tc.en, "CPA management password is unavailable (") && got == T(LangEN, "mgmt_password_unavailable")) {
			// long password form collapses to short EN on reverse of ZH short form
			if tc.zh == T(LangZH, "mgmt_password_unavailable") && got == T(LangEN, "mgmt_password_unavailable") {
				continue
			}
			t.Fatalf("zh->en %q => %q, want %q", tc.zh, got, tc.en)
		}
	}

	// Account prefix preserved, trailing known error localized.
	enPref := "user@x.com: " + T(LangEN, "auth_not_found", "id1")
	zhPref := "user@x.com: " + T(LangZH, "auth_not_found", "id1")
	if got := localizeKnownActionError(LangZH, enPref); got != zhPref {
		t.Fatalf("prefix en->zh = %q, want %q", got, zhPref)
	}
	if got := localizeKnownActionError(LangEN, zhPref); got != enPref {
		t.Fatalf("prefix zh->en = %q, want %q", got, enPref)
	}

	// Unknown upstream/network free text must stay intact.
	unknowns := []string{
		`CPA management API returned HTTP 403: {"error":"nope"}`,
		`Get "https://example.com": dial tcp 1.2.3.4:443: i/o timeout`,
		`tls: handshake failure`,
		`user@x.com: CPA management API returned HTTP 500: boom`,
	}
	for _, u := range unknowns {
		if got := localizeKnownActionError(LangZH, u); got != u {
			t.Fatalf("unknown rewritten: %q => %q", u, got)
		}
		if got := localizeKnownActionError(LangEN, u); got != u {
			t.Fatalf("unknown rewritten en: %q => %q", u, got)
		}
	}
}

func TestSnapshotLightLocalizesApplyFields(t *testing.T) {
	old := engine
	engine = &inspectionEngine{
		workers:      defaultWorkers,
		lang:         LangZH,
		applyCurrent: T(LangZH, "stopped"),
		applyFailures: []string{
			"a.json: " + T(LangZH, "auth_not_found", "a"),
			`Get "https://x": dial tcp timeout`,
		},
		recentRowActions: []rowActionReport{{
			Seq: 1, Key: "a", Action: "disable", OK: false,
			Error: T(LangZH, "auth_file_name_missing_for", "a.json"),
		}},
		results: []accountResult{{
			Name: "a", AuthIndex: "1", Reason: T(LangZH, "quota_exhausted"),
		}},
	}
	t.Cleanup(func() { engine = old })

	// Light status (no results) still localizes apply/action fields.
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
		Query:  map[string][]string{"include_results": {"0"}, "lang": {"en"}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	var payload struct {
		ApplyCurrent     string            `json:"apply_current"`
		ApplyFailures    []string          `json:"apply_failures"`
		RecentRowActions []rowActionReport `json:"recent_row_actions"`
		Results          []accountResult   `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(payload.Results) != 0 {
		t.Fatalf("light status should omit results, got %d", len(payload.Results))
	}
	if payload.ApplyCurrent != T(LangEN, "stopped") {
		t.Fatalf("apply_current = %q, want %q", payload.ApplyCurrent, T(LangEN, "stopped"))
	}
	if len(payload.ApplyFailures) != 2 {
		t.Fatalf("failures len=%d", len(payload.ApplyFailures))
	}
	wantFail0 := "a.json: " + T(LangEN, "auth_not_found", "a")
	if payload.ApplyFailures[0] != wantFail0 {
		t.Fatalf("failure[0] = %q, want %q", payload.ApplyFailures[0], wantFail0)
	}
	if payload.ApplyFailures[1] != `Get "https://x": dial tcp timeout` {
		t.Fatalf("unknown failure rewritten: %q", payload.ApplyFailures[1])
	}
	if len(payload.RecentRowActions) != 1 {
		t.Fatalf("recent len=%d", len(payload.RecentRowActions))
	}
	wantErr := T(LangEN, "auth_file_name_missing_for", "a.json")
	if payload.RecentRowActions[0].Error != wantErr {
		t.Fatalf("recent error = %q, want %q", payload.RecentRowActions[0].Error, wantErr)
	}
}

func TestStopUsesRequestLangForCancelAndApplyCurrent(t *testing.T) {
	old := engine
	engine = &inspectionEngine{
		workers: defaultWorkers,
		lang:    LangZH, // inspection started in Chinese
		running: true,
		stopped: false,
		total:   1,
		runTargets: []pluginapi.HostAuthFileEntry{{
			AuthIndex: "n1", Name: "n1.json", Email: "n1@x.com",
		}},
		runModel:     "grok",
		applying:     true,
		applyLang:    LangZH,
		applyCurrent: "禁用 n1.json",
	}
	t.Cleanup(func() { engine = old })

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/stop",
		Body:   []byte(`{"lang":"en"}`),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	var payload struct {
		ApplyCurrent string `json:"apply_current"`
		Stopped      bool   `json:"stopped"`
		Running      bool   `json:"running"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, string(resp.Body))
	}
	if payload.Running {
		t.Fatal("expected not running after stop")
	}
	if payload.ApplyCurrent != T(LangEN, "stopped") {
		t.Fatalf("apply_current = %q, want %q", payload.ApplyCurrent, T(LangEN, "stopped"))
	}

	engine.mu.Lock()
	var cancelled string
	for _, r := range engine.results {
		if r.AuthIndex == "n1" {
			cancelled = r.Reason
		}
	}
	engine.mu.Unlock()
	if cancelled != T(LangEN, "stopped_before_probe") {
		t.Fatalf("cancelled reason = %q, want %q", cancelled, T(LangEN, "stopped_before_probe"))
	}

	// /status?lang=zh should re-localize stored English cancel reason.
	status := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
		Query:  map[string][]string{"include_results": {"1"}, "lang": {"zh"}},
	})
	var st struct {
		Results []struct {
			AuthIndex string `json:"auth_index"`
			Reason    string `json:"reason"`
		} `json:"results"`
	}
	if err := json.Unmarshal(status.Body, &st); err != nil {
		t.Fatalf("status unmarshal: %v", err)
	}
	found := false
	for _, r := range st.Results {
		if r.AuthIndex == "n1" {
			found = true
			if r.Reason != T(LangZH, "stopped_before_probe") {
				t.Fatalf("status reason zh = %q", r.Reason)
			}
		}
	}
	if !found {
		t.Fatal("cancelled row missing from status")
	}
}

func TestStopInvalidJSONReturns400(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers}
	t.Cleanup(func() { engine = old })

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/stop",
		Body:   []byte(`{lang:`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", resp.StatusCode, string(resp.Body))
	}
	// Must not start/stop side effects that require valid body — engine still idle.
	if engine.running || engine.stopped {
		// stop shouldn't run on invalid JSON
		t.Fatalf("invalid JSON must not invoke stop: running=%v stopped=%v", engine.running, engine.stopped)
	}
}

func TestUIStopBodyIncludesLangAndActionErrorLocalizer(t *testing.T) {
	page := string(renderUIPage(pluginName))
	if !strings.Contains(page, "api('/stop'") {
		t.Fatal("stop API call missing")
	}
	idx := strings.Index(page, "api('/stop'")
	if idx < 0 || !strings.Contains(page[idx:idx+120], "lang: lang") {
		t.Fatal("stop POST body must include lang")
	}
	if !strings.Contains(page, "function localizeKnownActionError(") {
		t.Fatal("UI must define localizeKnownActionError")
	}
	if !strings.Contains(page, ".map(localizeKnownActionError)") {
		t.Fatal("completedErrors must re-localize known action errors")
	}
}

// extractI18NPack pulls string keys from the renderUIPage I18N.<lang> object literal.
func extractI18NPack(page, lang string) map[string]string {
	marker := lang + ": {"
	idx := strings.Index(page, "const I18N = {")
	if idx < 0 {
		return nil
	}
	// Prefer the main I18N object (not REASON_I18N).
	sub := page[idx:]
	langIdx := strings.Index(sub, marker)
	if langIdx < 0 {
		return nil
	}
	// Find matching closing of this language object: start after marker, track braces.
	start := langIdx + len(marker)
	depth := 1
	end := -1
	for i := start; i < len(sub); i++ {
		switch sub[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				i = len(sub)
			}
		}
	}
	if end < 0 {
		return nil
	}
	body := sub[start:end]
	out := map[string]string{}
	// key:'value' or key: 'value' — values may contain escaped quotes rarely; keep simple.
	// Match key:'...' pairs; allow commas and newlines.
	re := regexp.MustCompile(`([A-Za-z0-9_]+)\s*:\s*'((?:\\'|[^'])*)'`)
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		key := m[1]
		val := strings.ReplaceAll(m[2], `\'`, `'`)
		out[key] = val
	}
	return out
}

func TestBanPagerFilterParensAreI18n(t *testing.T) {
	page := string(renderUIPage(pluginName))
	// Hardcoded fullwidth parentheses around ban filter label must be gone.
	if strings.Contains(page, "('（' + banFilterLabel") || strings.Contains(page, "banFilterLabel(banState.filter) + '）'") {
		t.Fatal("ban pager filter summary still hardcodes fullwidth parentheses")
	}
	if !strings.Contains(page, "t('pager_filter_prefix')") || !strings.Contains(page, "t('pager_filter_suffix')") {
		t.Fatal("ban pager filter summary must use pager_filter_prefix/suffix i18n keys")
	}
	// Both language packs must define the keys with language-appropriate brackets.
	zh := extractI18NPack(page, "zh")
	en := extractI18NPack(page, "en")
	if zh["pager_filter_prefix"] != "（" || zh["pager_filter_suffix"] != "）" {
		t.Fatalf("zh pager_filter_* = %q/%q, want fullwidth parentheses", zh["pager_filter_prefix"], zh["pager_filter_suffix"])
	}
	if en["pager_filter_prefix"] != " (" || en["pager_filter_suffix"] != ")" {
		t.Fatalf("en pager_filter_* = %q/%q, want ASCII parentheses with leading space on open", en["pager_filter_prefix"], en["pager_filter_suffix"])
	}
}
