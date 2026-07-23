package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

// Issue #21: UI can load autoban status (CPA gateway authenticated the page key),
// but background auto-disable / restore-retry logged:
//   retry disable auth in CPA failed ... error="CPA management password is unavailable"
//
// Released v0.1.14 lacked an in-memory Management Key cache for async paths.
// Baseline 987722d remembers the key from Management route headers so dispose /
// restore retry can reuse it without MANAGEMENT_PASSWORD env.

func issue21ClearEnvPassword(t *testing.T) {
	t.Helper()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	oldKey := os.Getenv("CPA_MANAGEMENT_KEY")
	_ = os.Unsetenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("CPA_MANAGEMENT_KEY")
	t.Cleanup(func() {
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
		_ = os.Setenv("CPA_MANAGEMENT_KEY", oldKey)
	})
}

func issue21MockManagementServer(t *testing.T) (baseURL string, lastAuth *atomic.Value) {
	t.Helper()
	lastAuth = &atomic.Value{}
	lastAuth.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	oldBase, oldDo := getCPAManagementBaseURL(), getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})
	return server.URL, lastAuth
}

// RED: without env password and without a remembered page key, background disable fails.
func TestIssue21BackgroundDisableFailsWithoutCachedKey(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	issue21ClearEnvPassword(t)
	_, _ = issue21MockManagementServer(t)

	err := disableAuthInCPA("issue21-no-key")
	if err == nil {
		t.Fatal("expected disable to fail without management password")
	}
	if !strings.Contains(err.Error(), "CPA management password is unavailable") {
		t.Fatalf("error = %v, want password unavailable", err)
	}
}

// GREEN: loading /bans with page Authorization remembers the key for dispose + restore-retry.
func TestIssue21BansStatusLoadEnablesBackgroundDisable(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	issue21ClearEnvPassword(t)
	isolateActiveStore(t)
	_, lastAuth := issue21MockManagementServer(t)

	// Simulate authenticated UI poll of auto-ban status (page has Management Key).
	headers := http.Header{}
	headers.Set("Authorization", "Bearer ui-page-key-issue21")
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    managementRoutePrefix + "/bans",
		Headers: headers,
	})
	if resp.StatusCode != http.StatusOK && resp.StatusCode != 0 {
		t.Fatalf("GET /bans status = %d body=%s", resp.StatusCode, string(resp.Body))
	}
	if got := cpaManagementPasswordOrCached(); got != "ui-page-key-issue21" {
		t.Fatalf("cache after /bans = %q, want ui-page-key-issue21", got)
	}

	// Background dispose path (Usage -> queue worker) has no request headers.
	if err := disableAuthInCPA("issue21-dispose"); err != nil {
		t.Fatalf("disableAuthInCPA after /bans: %v", err)
	}
	if got := lastAuth.Load().(string); got != "Bearer ui-page-key-issue21" {
		t.Fatalf("dispose Authorization = %q", got)
	}

	// restoreExpiredBans retry-disable path (log: retry disable auth in CPA failed).
	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      "issue21-retry",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now,
		ResetAt:     now.Add(2 * time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   false,
	})
	restored, failed := restoreExpiredBans(activeStore, now)
	if restored != 0 || failed != 0 {
		t.Fatalf("restoreExpiredBans restored=%d failed=%d (unexpected for unexpired)", restored, failed)
	}
	entry, ok := activeStore.Get("issue21-retry")
	if !ok {
		t.Fatal("ban entry missing after retry disable")
	}
	if !entry.CpaSynced {
		t.Fatalf("expected CPA synced after retry disable, sync_error=%q", entry.CpaSyncError)
	}
	if got := lastAuth.Load().(string); got != "Bearer ui-page-key-issue21" {
		t.Fatalf("retry-disable Authorization = %q", got)
	}
}

// GREEN: X-Management-Key (case variants) is remembered the same way as Bearer.
func TestIssue21XManagementKeyHeaderIsCachedForBackgroundDisable(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	issue21ClearEnvPassword(t)
	_, lastAuth := issue21MockManagementServer(t)

	headers := http.Header{}
	// Non-canonical key as some gateways preserve original casing in maps.
	headers["x-management-key"] = []string{"x-key-issue21"}
	_ = dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    managementRoutePrefix + "/bans",
		Headers: headers,
	})
	if got := cpaManagementPasswordOrCached(); got != "x-key-issue21" {
		t.Fatalf("cache after X-Management-Key = %q", got)
	}
	if err := disableAuthInCPA("issue21-xkey"); err != nil {
		t.Fatalf("disableAuthInCPA: %v", err)
	}
	if got := lastAuth.Load().(string); got != "Bearer x-key-issue21" {
		t.Fatalf("Authorization = %q", got)
	}
}

// GREEN: host RPC JSON (embedded ManagementRequest + host_callback_id) still populates cache.
func TestIssue21RPCJSONManagementRequestRemembersKey(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	issue21ClearEnvPassword(t)

	// Host marshals rpcManagementRequest without lowercase json tags on embedded fields.
	raw, err := json.Marshal(map[string]any{
		"Method": http.MethodGet,
		"Path":   "/v0/management" + managementRoutePrefix + "/bans",
		"Headers": map[string][]string{
			"Authorization": {"Bearer rpc-json-key"},
		},
		"host_callback_id": "cb-issue21",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, errHandle := handleManagement(raw)
	if errHandle != nil {
		t.Fatalf("handleManagement: %v", errHandle)
	}
	if len(out) == 0 {
		t.Fatal("empty handleManagement response")
	}
	if got := cpaManagementPasswordOrCached(); got != "rpc-json-key" {
		t.Fatalf("cache after RPC JSON = %q", got)
	}
}

// GREEN: applyBanDispose uses the cached page key (Usage-triggered auto-disable).
func TestIssue21ApplyBanDisposeUsesCachedKey(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	issue21ClearEnvPassword(t)
	isolateActiveStore(t)
	_, lastAuth := issue21MockManagementServer(t)

	rememberManagementCredential("dispose-cached-key")
	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      "issue21-apply",
		Provider:    "xai",
		ErrorCode:   permissionDeniedErrorCode,
		BannedAt:    now,
		ResetAt:     now.Add(24 * time.Hour),
		ResetSource: "manual_unban",
		CpaSynced:   false,
	})
	entry, _ := activeStore.Get("issue21-apply")
	if err := applyBanDisposeForTest(entry.AuthID, entry.Revision); err != nil {
		t.Fatalf("applyBanDispose: %v", err)
	}
	entry, ok := activeStore.Get("issue21-apply")
	if !ok || !entry.CpaSynced {
		t.Fatalf("after dispose: ok=%v entry=%+v", ok, entry)
	}
	if got := lastAuth.Load().(string); got != "Bearer dispose-cached-key" {
		t.Fatalf("Authorization = %q", got)
	}
}
