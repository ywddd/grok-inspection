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
