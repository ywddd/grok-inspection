package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestApplyInvalidJSONReturns400(t *testing.T) {
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/apply",
		Body:   []byte(`{not-json`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", resp.StatusCode, string(resp.Body))
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != false {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestBanStoreDeleteIfRespectsRevision(t *testing.T) {
	store := newBanStore()
	entry := banEntry{
		AuthID:    "a1",
		Provider:  "xai",
		ErrorCode: exhaustedErrorCode,
		BannedAt:  time.Unix(100, 0),
		ResetAt:   time.Unix(200, 0),
		CpaSynced: true,
	}
	store.Set(entry)
	got, ok := store.Get("a1")
	if !ok || got.Revision == 0 {
		t.Fatalf("expected revision, got %+v", got)
	}
	if store.DeleteIf("a1", got.Revision+1) {
		t.Fatal("DeleteIf should fail on mismatch")
	}
	if _, ok := store.Get("a1"); !ok {
		t.Fatal("entry deleted on mismatch")
	}
	if !store.DeleteIf("a1", got.Revision) {
		t.Fatal("DeleteIf should succeed on match")
	}
	if _, ok := store.Get("a1"); ok {
		t.Fatal("entry still present")
	}
}

func TestShouldTryFallbackSkipsBare429(t *testing.T) {
	if shouldTryFallback(http.StatusTooManyRequests, "unknown") {
		t.Fatal("bare 429 should not trigger chat fallback")
	}
	if !shouldTryFallback(http.StatusForbidden, "unknown") {
		t.Fatal("403 unknown should still allow fallback")
	}
}

func TestUnbanJobStatusIdle(t *testing.T) {
	st := unbanJobStatus()
	if st["running"] != false {
		t.Fatalf("status = %#v", st)
	}
}
