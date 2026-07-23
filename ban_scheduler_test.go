package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestRestoreExpiredGrokBanReEnables(t *testing.T) {
	var enabled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Name != "expired" || body.Disabled {
			t.Fatalf("body = %#v", body)
		}
		enabled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBaseURL := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-management-password")
	defer func() {
		freezeAndWaitBanDisposeIdleForTest(t)
		setCPAManagementBaseURL(oldBaseURL)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
		unfreezeBanDisposeWorkersForTest()
	}()

	store := newBanStore()
	store.Set(testEntry("expired", time.Unix(100, 0)))
	restored, failed := restoreExpiredBans(store, time.Unix(101, 0))
	if restored != 1 || failed != 0 {
		t.Fatalf("restore = %d failed=%d", restored, failed)
	}
	if _, ok := store.Get("expired"); ok {
		t.Fatal("expired ban was not removed")
	}
	if !enabled {
		t.Fatal("expired ban did not re-enable CPA auth")
	}
}

func TestHandleUsageRecordsExactGrokBans(t *testing.T) {
	isolateActiveStore(t)
	// Usage enqueues async dispose; freeze workers for assertions and race-safe dial restore.
	pauseBanDisposeWorkersForTest(t)
	resetBanDisposeQueueForTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPass) })
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)

	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "auth-1",
		Failed:   true,
		Failure:  pluginapi.UsageFailure{StatusCode: 429, Body: realGrok429Body},
	}
	if _, err := handleUsageRecord(record, defaultPluginConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if entry, ok := activeStore.Get("auth-1"); !ok {
		t.Fatal("exact Grok 429 was not stored")
	} else if entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("429 entry = %#v", entry)
	}

	record.AuthID = "auth-403"
	record.Failure = pluginapi.UsageFailure{StatusCode: 403, Body: realGrok403Body}
	if _, err := handleUsageRecord(record, defaultPluginConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if entry, ok := activeStore.Get("auth-403"); !ok {
		t.Fatal("exact Grok 403 was not stored")
	} else if entry.ErrorCode != permissionDeniedErrorCode || entry.ResetSource != "manual_unban" {
		t.Fatalf("403 entry = %#v", entry)
	}

	record.AuthID = "auth-401"
	record.Failure = pluginapi.UsageFailure{StatusCode: 401, Body: `{"error":"invalid credentials"}`}
	if _, err := handleUsageRecord(record, defaultPluginConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if entry, ok := activeStore.Get("auth-401"); !ok {
		t.Fatal("Grok 401 was not stored")
	} else if entry.ErrorCode != unauthorizedErrorCode || entry.ResetSource != "manual_unban" {
		t.Fatalf("401 entry = %#v", entry)
	}

	record.Failure.Body = `{"code":"rate_limit"}`
	record.AuthID = "auth-2"
	record.Failure.StatusCode = 429
	if _, err := handleUsageRecord(record, defaultPluginConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, ok := activeStore.Get("auth-2"); ok {
		t.Fatal("generic 429 was stored")
	}
}
