package main

import (
	"net/http"
	"strings"
	"testing"

	"grok-inspection/cpasdk/pluginapi"
)

func TestManagementStatusReturnsJSON(t *testing.T) {
	response := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
	})

	if got := response.Headers.Get("content-type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q, want application/json", got)
	}
}

func TestManagementStatusLightOmitsResults(t *testing.T) {
	old := engine
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{Name: "a", AuthIndex: "1", FileName: "a.json", Classification: "healthy", Action: "keep"},
			{Name: "b", AuthIndex: "2", FileName: "b.json", Classification: "reauth", Action: "delete"},
		},
		resultsGen: 9,
	}
	t.Cleanup(func() { engine = old })

	light := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
		Query:  map[string][]string{"include_results": {"0"}},
	})
	body := string(light.Body)
	if strings.Contains(body, `"results":[`) {
		t.Fatalf("light status should omit results array: %s", body)
	}
	if !strings.Contains(body, `"include_results":false`) {
		t.Fatalf("light status missing include_results=false: %s", body)
	}
	if !strings.Contains(body, `"results_gen":9`) {
		t.Fatalf("light status missing results_gen: %s", body)
	}

	full := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
		Query:  map[string][]string{"include_results": {"1"}},
	})
	if !strings.Contains(string(full.Body), `"results":[`) {
		t.Fatalf("full status should include results: %s", string(full.Body))
	}
}

func TestResourceStatusReturnsHTML(t *testing.T) {
	response := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/grok-inspection/status",
	})

	if got := response.Headers.Get("content-type"); !strings.Contains(got, "text/html") {
		t.Fatalf("content-type = %q, want text/html", got)
	}
}

func TestResourcePageDoesNotPollWithoutManagementKey(t *testing.T) {
	page := string(renderUIPage(pluginName))
	guard := "if (!keyInput.value.trim())"
	refresh := "async function refresh(opts)"
	refreshIndex := strings.Index(page, refresh)
	guardIndex := strings.Index(page, guard)

	if refreshIndex < 0 || guardIndex < refreshIndex {
		t.Fatalf("refresh must guard management requests with %q", guard)
	}
	if !strings.Contains(page, `include_results=0`) || !strings.Contains(page, `refresh({ light: true })`) {
		t.Fatal("page should light-poll /status without full results during jobs")
	}
	for _, marker := range []string{
		`const LIVE_RESULTS_MS = 2400`,
		`async function syncFullResults(force)`,
		`gen !== lastResultsGen`,
		`await syncFullResults(!busy)`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("page missing live result synchronization marker %q", marker)
		}
	}
}

func TestResourcePageHasMobileScopedHostThemeStyles(t *testing.T) {
	page := string(renderUIPage(pluginName))
	required := []string{
		`class="wrap grok-inspection-page"`,
		`.grok-inspection-page`,
		`@media (max-width:760px)`,
		`html[data-grok-theme="dark"]`,
		`function detectHostTheme()`,
		`setAttribute('data-grok-theme', detectHostTheme() || 'light')`,
		`grid-template-columns:repeat(2,minmax(0,1fr))`,
		`grid-column:1 / -1`,
		`min-width:0`,
	}
	for _, marker := range required {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing mobile/dark-mode marker %q", marker)
		}
	}
}

func TestResourcePageShowsManagementKeyPrompt(t *testing.T) {
	page := string(renderUIPage(pluginName))
	required := []string{
		`请输入 CPA Management Key`,
		`const hasManagementKey = () => !!keyInput.value.trim();`,
		`$('runBtn').disabled = !hasManagementKey() ||`,
		`'请输入 CPA Management Key 后加载巡检状态'`,
		`cli-proxy-auth`,
		`extractKeyFromPanelStorage`,
		`id="error"`,
		`id="progress"`,
	}
	for _, marker := range required {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing management-key UX marker %q", marker)
		}
	}
	// Error toast should sit with progress (not only under the table).
	progressIdx := strings.Index(page, `id="progress"`)
	errorIdx := strings.Index(page, `id="error"`)
	if progressIdx < 0 || errorIdx < 0 || errorIdx < progressIdx {
		t.Fatal("error element should appear after progress in the status bar")
	}
}

func TestResourcePageHasExportAndBatchOps(t *testing.T) {
	page := string(renderUIPage(pluginName))
	required := []string{
		`id="workers"`,
		`value="6"`,
		`parseWorkersStrict`,
		`id="batchExportBtn"`,
		`id="batchDisableBtn"`,
		`id="batchEnableBtn"`,
		`id="batchDeleteBtn"`,
		`id="confirmModal"`,
		`function confirmDialog`,
		`当前分类：`,
		`force_action: action`,
		`filteredRowsForAction`,
		`批量禁用`,
		`批量启用`,
		`批量删除`,
			`批量导出`,
				`function stopPolling()`,
				`function startPolling()`,
				`function syncPolling(snap)`,
				`snap.running || snap.applying || snap.reauthing || snap.baseurl_applying`,
				`id="incrBtn"`,
				`增量巡检`,
				`id="filterRunBtn"`,
				`巡检当前分类`,
				`classificationsForFilter`,
				`mode === 'filter'`,
				`id="credUploadBtn"`,
				`id="reauthBtn"`,
				`id="baseurlBtn"`,
				`/credentials`,
				`/reauth/start`,
				`/baseurl/apply`,
				`重新登录匹配账号`,
				`应用 api.x.ai base_url`,
				`api_gateway_ok`,
				`body.incremental = true`,
				`['other','异常'`,
				`['api_gateway_ok','API可用'`,
			}
	for _, marker := range required {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing marker %q", marker)
		}
	}
	if strings.Contains(page, `setInterval(refresh, 1500)`) {
		t.Fatal("page must not permanently poll /status every 1.5s when idle")
	}
	// Duplicate filter button row should be gone; cards are the only category UI.
	if strings.Contains(page, `id="filters"`) {
		t.Fatal("duplicate filter button row should be removed; use summary cards only")
	}
}

func TestApplyAcceptedAsync(t *testing.T) {
	// Without candidates, apply returns conflict quickly (no hang).
	response := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/apply",
		Body:   []byte(`{}`),
	})
	if response.StatusCode != http.StatusConflict && response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", response.StatusCode, string(response.Body))
	}
}
