package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestRestoreExpiredBansReEnablesAndDropsEntry(t *testing.T) {
	var enabled []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Disabled {
			t.Fatalf("expected enable, got disable for %s", body.Name)
		}
		enabled = append(enabled, body.Name)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		cpaManagementBaseURL = oldBase
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	store := newBanStore()
	now := time.Unix(1000, 0)
	store.Set(banEntry{
		AuthID:      "expired-quota",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-2 * time.Hour),
		ResetAt:     now.Add(-time.Minute),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})
	store.Set(banEntry{
		AuthID:      "manual-keep",
		Provider:    "xai",
		ErrorCode:   permissionDeniedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(-time.Minute),
		ResetSource: "manual_unban",
		CpaSynced:   true,
	})
	store.Set(banEntry{
		AuthID:      "still-active",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})

	restored, failed := restoreExpiredBans(store, now)
	if restored != 1 || failed != 0 {
		t.Fatalf("restored=%d failed=%d", restored, failed)
	}
	if len(enabled) != 1 || enabled[0] != "expired-quota" {
		t.Fatalf("enabled calls = %#v", enabled)
	}
	if _, ok := store.Get("expired-quota"); ok {
		t.Fatal("expired quota ban not removed")
	}
	if _, ok := store.Get("manual-keep"); !ok {
		t.Fatal("manual ban was removed")
	}
	if _, ok := store.Get("still-active"); !ok {
		t.Fatal("active ban was removed")
	}
}

func TestRestoreExpiredBansKeepsEntryOnEnableFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"no"}`))
	}))
	defer server.Close()
	oldBase := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		cpaManagementBaseURL = oldBase
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	store := newBanStore()
	now := time.Unix(2000, 0)
	store.Set(banEntry{
		AuthID:      "broken",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(-time.Second),
		ResetSource: "date_plus_fallback",
			CpaSynced:   true,
	})
	restored, failed := restoreExpiredBans(store, now)
	if restored != 0 || failed != 1 {
		t.Fatalf("restored=%d failed=%d", restored, failed)
	}
	if _, ok := store.Get("broken"); !ok {
		t.Fatal("failed restore must keep ban for retry")
	}
}

func TestBanStoreLoadKeepsExpiredForRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bans.json")
	now := time.Unix(3000, 0)
	store := newBanStore()
	store.Set(banEntry{
		AuthID:      "expired",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-2 * time.Hour),
		ResetAt:     now.Add(-time.Minute),
		ResetSource: "local_plus_fallback",
			CpaSynced:   true,
	})
	if err := store.Save(path); err != nil {
		t.Fatal(err)
	}
	reloaded := newBanStore()
	if err := reloaded.Load(path, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get("expired"); !ok {
		t.Fatal("expired ban dropped on load; restore would never run")
	}
	if got := reloaded.Expired(now); len(got) != 1 || got[0] != "expired" {
		t.Fatalf("Expired() = %#v", got)
	}
}

func TestRestoreExpiredBansWorksWhenAutobanDisabled(t *testing.T) {
	// Cooldown recovery must not depend on autoban_enabled.
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.Enabled = false
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })

	var enabled []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Disabled {
			t.Fatalf("expected enable, got disable for %s", body.Name)
		}
		enabled = append(enabled, body.Name)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		cpaManagementBaseURL = oldBase
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	store := newBanStore()
	now := time.Unix(5000, 0)
	store.Set(banEntry{
		AuthID:      "quota-off",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(-time.Minute),
		ResetSource: "local_plus_fallback",
			CpaSynced:   true,
	})
	restored, failed := restoreExpiredBans(store, now)
	if restored != 1 || failed != 0 {
		t.Fatalf("restored=%d failed=%d want 1/0 with autoban disabled", restored, failed)
	}
	if len(enabled) != 1 || enabled[0] != "quota-off" {
		t.Fatalf("enabled calls = %#v", enabled)
	}
	if _, ok := store.Get("quota-off"); ok {
		t.Fatal("restored ban should be removed from store")
	}
}

func TestRestoreExpiredBansDropsMissingAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"auth file not found"}`))
	}))
	defer server.Close()

	oldBase := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		cpaManagementBaseURL = oldBase
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	store := newBanStore()
	now := time.Unix(6000, 0)
	store.Set(banEntry{
		AuthID:      "gone.json",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(-time.Second),
		ResetSource: "date_plus_fallback",
			CpaSynced:   true,
	})
	restored, failed := restoreExpiredBans(store, now)
	if restored != 1 || failed != 0 {
		t.Fatalf("restored=%d failed=%d want drop-as-success 1/0", restored, failed)
	}
	if _, ok := store.Get("gone.json"); ok {
		t.Fatal("missing auth ban must be dropped")
	}
}

func TestBanStatusCountMatchesBansList(t *testing.T) {
	isolateActiveStore(t)

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      "q1.json",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(2 * time.Hour),
		ResetSource: "local_plus_fallback",
			CpaSynced:   true,
	})
	activeStore.Set(banEntry{
		AuthID:      "m1.json",
		Provider:    "xai",
		ErrorCode:   permissionDeniedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.AddDate(100, 0, 0),
		ResetSource: "manual_unban",
			CpaSynced:   true,
	})
	// expired free-usage appears in bans list (pending_restore) so operators can act on it
	activeStore.Set(banEntry{
		AuthID:      "expired.json",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-2 * time.Hour),
		ResetAt:     now.Add(-time.Minute),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})

	st := banStatus()
	bans, ok := st["bans"].([]map[string]any)
	if !ok {
		t.Fatalf("bans type = %T", st["bans"])
	}
	count, ok := st["count"].(int)
	if !ok {
		t.Fatalf("count type = %T", st["count"])
	}
	if count != 3 {
		t.Fatalf("count=%d want 3 (same as bans list, including pending_restore)", count)
	}
	if count != len(bans) {
		t.Fatalf("count=%d len(bans)=%d must match", count, len(bans))
	}
	if len(bans) != 3 {
		t.Fatalf("len(bans)=%d want 3 including pending_restore", len(bans))
	}
	quota, _ := st["quota_count"].(int)
	manual, _ := st["manual_count"].(int)
	if quota != 2 || manual != 1 {
		t.Fatalf("quota=%d manual=%d", quota, manual)
	}
	pending, _ := st["pending_restore"].(int)
	if pending != 1 {
		t.Fatalf("pending_restore=%d want 1", pending)
	}
}

func TestHandleUsageSkipsWhenAutobanDisabled(t *testing.T) {
	isolateActiveStore(t)

	cfg := defaultPluginConfig()
	cfg.Enabled = false
	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "should-not-ban",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: http.StatusTooManyRequests,
			Body:       realGrok429Body,
		},
	}
	if _, err := handleUsageRecord(record, cfg, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok := activeStore.Get("should-not-ban"); ok {
		t.Fatal("disabled autoban must not record bans")
	}
}

func TestRestoreLoopSourceIndependentOfEnabled(t *testing.T) {
	raw, err := os.ReadFile("ban_scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start := strings.Index(src, "func startBanRestoreLoop()")
	if start < 0 {
		t.Fatal("startBanRestoreLoop not found")
	}
	// next func after loop
	end := strings.Index(src[start+1:], "\nfunc ")
	if end < 0 {
		end = len(src) - start
	}
	body := src[start : start+end]
	if strings.Contains(body, "loadedConfig().Enabled") {
		t.Fatal("restore loop must not gate ticks on loadedConfig().Enabled")
	}
	if !strings.Contains(body, "restoreExpiredBans(activeStore") {
		t.Fatal("restore loop must call restoreExpiredBans")
	}
}


func TestRestoreExpiredBansDropsUnsyncedWhenAuthNotFound(t *testing.T) {
	// Manual 401/403 bans that never synced must not retry forever when CPA says
	// the auth file is already gone.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"auth file not found"}`))
	}))
	defer server.Close()

	oldBase := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		cpaManagementBaseURL = oldBase
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	store := newBanStore()
	now := time.Unix(7000, 0)
	store.Set(banEntry{
		AuthID:      "manual-gone.json",
		Provider:    "xai",
		ErrorCode:   permissionDeniedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(24 * time.Hour), // still active → disable-retry path
		ResetSource: "manual_unban",
		CpaSynced:   false,
	})
	store.Set(banEntry{
		AuthID:      "auth-gone.json",
		Provider:    "xai",
		ErrorCode:   unauthorizedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(24 * time.Hour),
		ResetSource: "manual_unban",
		CpaSynced:   false,
	})

	restored, failed := restoreExpiredBans(store, now)
	if restored != 0 || failed != 0 {
		t.Fatalf("restored=%d failed=%d want 0/0 (drop via disable retry, not enable)", restored, failed)
	}
	if _, ok := store.Get("manual-gone.json"); ok {
		t.Fatal("permission ban must be dropped when auth file not found")
	}
	if _, ok := store.Get("auth-gone.json"); ok {
		t.Fatal("401 ban must be dropped when auth file not found")
	}
}

func TestRestoreExpiredBansKeepsUnsyncedOnOtherDisableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream temporary"}`))
	}))
	defer server.Close()

	oldBase := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	defer func() {
		cpaManagementBaseURL = oldBase
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	}()

	store := newBanStore()
	now := time.Unix(8000, 0)
	store.Set(banEntry{
		AuthID:      "keep-retry.json",
		Provider:    "xai",
		ErrorCode:   unauthorizedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(24 * time.Hour),
		ResetSource: "manual_unban",
		CpaSynced:   false,
	})

	restored, failed := restoreExpiredBans(store, now)
	if restored != 0 || failed != 0 {
		t.Fatalf("restored=%d failed=%d want 0/0", restored, failed)
	}
	if _, ok := store.Get("keep-retry.json"); !ok {
		t.Fatal("non-not-found disable errors must keep the ban for the next tick")
	}
	if entry, ok := store.Get("keep-retry.json"); !ok || entry.CpaSynced {
		t.Fatalf("entry should remain unsynced: ok=%v synced=%v", ok, entry.CpaSynced)
	}
}