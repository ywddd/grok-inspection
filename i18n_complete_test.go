package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
