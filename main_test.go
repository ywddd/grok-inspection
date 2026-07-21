package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		`const LIVE_RESULTS_MS = 10000`,
		`async function syncFullResults(force, busyRunning)`,
		`gen !== lastResultsGen`,
		`await syncFullResults(!busy, !!(data && (data.running || data.applying || (data.unban && data.unban.running))))`,
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
		`document.documentElement.setAttribute('data-grok-theme', theme)`,
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

func TestResourcePageTreatsCPAWhiteThemeAsLight(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, marker := range []string{
		`const lightNames = ['light', 'white', 'day', 'bright', 'default'];`,
		`namedTheme(el.getAttribute && el.getAttribute(name))`,
		`elementTheme(doc.documentElement) || elementTheme(doc.body) || backgroundTheme(doc)`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing CPA white-theme marker %q", marker)
		}
	}
}

func TestResourcePageDialogMessagesUseEscapedNewlines(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, invalid := range []string{
		"个账号。\n后台异步执行",
		"全部账号。\n后台异步执行",
	} {
		if strings.Contains(page, invalid) {
			t.Fatalf("resource page contains a raw newline inside a JavaScript string: %q", invalid)
		}
	}
	for _, required := range []string{
		`个账号。\n后台异步执行`,
		`全部账号。\n后台异步执行`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("resource page missing escaped dialog newline %q", required)
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
		`snap.running || snap.applying`,
		`snap.probe_phase === 'retry'`,
		`超时复检`,
		`id="incrBtn"`,
		`增量巡检`,
		`id="filterRunBtn"`,
		`巡检当前分类`,
		`classificationsForFilter`,
		`mode === 'filter'`,
		`body.incremental = true`,
		`['other','异常'`,
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

func TestResourcePageAutobanListUX(t *testing.T) {
	page := string(renderUIPage(pluginName))
	required := []string{
		`禁用原因`,
		`function formatBanReason`,
		`function formatShanghaiTime`,
		`function formatResetSource`,
		`const esc = escapeHtml`,
		`Asia/Shanghai`,
		`额度用尽`,
		`权限被拒绝`,
		`认证失败`,
		`需手动解禁`,
		`定时自动恢复`,
		`当前没有自动禁用中的账号`,
		`banEnabledToggle`,
		`banSummary`,
		`data-ban-filter`,
		`banPermissionCount`,
		`banUnauthorizedCount`,
		`unbanCurrentFilter`,
		`setAutobanEnabled`,
		`账号巡检`,
		`实时自动禁用`,
		`Grok 账号巡检`,
		`if (hasManagementKey()) {`,
		`loadBans();`,
	}
	for _, marker := range required {
		if !strings.Contains(page, marker) {
			t.Fatalf("resource page missing autoban UX marker %q", marker)
		}
	}
	// Removed duplicate settings / rules card
	forbidden := []string{
		`规则说明`,
		`data-tab="settings"`,
		`id="panel-settings"`,
		`id="setWorkers"`,
		`只处理：`,
	}
	for _, marker := range forbidden {
		if strings.Contains(page, marker) {
			t.Fatalf("resource page should not contain removed UI %q", marker)
		}
	}
	// Column header renamed from 错误码
	if strings.Contains(page, `>错误码<`) {
		t.Fatal("ban table should use 禁用原因, not 错误码")
	}
}

func TestManagementBansCountMatchesList(t *testing.T) {
	isolateActiveStore(t)

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      "api-q.json",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Minute),
		ResetAt:     now.Add(time.Hour),
		ResetSource: "local_plus_fallback",
	})
	activeStore.Set(banEntry{
		AuthID:      "api-m.json",
		Provider:    "xai",
		ErrorCode:   permissionDeniedErrorCode,
		BannedAt:    now.Add(-time.Minute),
		ResetAt:     now.AddDate(50, 0, 0),
		ResetSource: "manual_unban",
	})
	activeStore.Set(banEntry{
		AuthID:      "api-u.json",
		Provider:    "xai",
		ErrorCode:   unauthorizedErrorCode,
		BannedAt:    now.Add(-time.Minute),
		ResetAt:     now.AddDate(50, 0, 0),
		ResetSource: "manual_unban",
	})

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/bans",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	var payload struct {
		Count         int                      `json:"count"`
		Bans          []map[string]interface{} `json:"bans"`
		Quota         int                      `json:"quota_count"`
		Permission    int                      `json:"permission_count"`
		Unauthorized  int                      `json:"unauthorized_count"`
		Manual        int                      `json:"manual_count"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, string(resp.Body))
	}
	if payload.Count != len(payload.Bans) {
		t.Fatalf("count=%d len(bans)=%d", payload.Count, len(payload.Bans))
	}
	if payload.Count != 3 || payload.Quota != 1 || payload.Permission != 1 || payload.Unauthorized != 1 || payload.Manual != 2 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestAutobanSettingsToggle(t *testing.T) {
	dir := t.TempDir()
	old := loadedConfig()
	cfg := old
	cfg.StateFile = filepath.Join(dir, "bans.json")
	cfg.Enabled = true
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(old) })

	off := false
	got, err := updateAutobanSettings(&off, nil)
	if err != nil {
		t.Fatalf("updateAutobanSettings() error = %v", err)
	}
	if got.Enabled {
		t.Fatal("enabled still true after toggle off")
	}
	if loadedConfig().Enabled {
		t.Fatal("loadedConfig().Enabled still true")
	}

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/autoban-settings",
		Body:   []byte(`{"enabled":true}`),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	if !loadedConfig().Enabled {
		t.Fatal("enabled not restored via management API")
	}
}
