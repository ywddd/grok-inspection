package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestDispatchUnbanConflictReturns409Not400(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Disabled {
			select {
			case <-enableEntered:
			default:
				close(enableEntered)
			}
			<-releaseEnable
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "route-ec1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})

	type result struct {
		resp pluginapi.ManagementResponse
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{resp: dispatchManagement(pluginapi.ManagementRequest{
			Method:  http.MethodPost,
			Path:    "/v0/management/plugins/grok-inspection/unban",
			Body:    []byte(`{"auth_id":"route-ec1"}`),
			Headers: http.Header{"Authorization": []string{"Bearer test-pass"}},
		})}
	}()
	select {
	case <-enableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("enable not entered via /unban")
	}
	activeStore.Set(banEntry{
		AuthID: "route-ec1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(15 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: true,
	})
	close(releaseEnable)

	var out result
	select {
	case out = <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("/unban hung")
	}
	if out.resp.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d want 409 body=%s", out.resp.StatusCode, string(out.resp.Body))
	}
	var payload map[string]any
	if err := json.Unmarshal(out.resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != false {
		t.Fatalf("ok=%v want false", payload["ok"])
	}
	if payload["missing"] != false {
		t.Fatalf("missing=%v want false", payload["missing"])
	}
	if payload["enabled"] != false {
		t.Fatalf("enabled=%v want false", payload["enabled"])
	}
	if payload["removed"] != false {
		t.Fatalf("removed=%v want false", payload["removed"])
	}
	errStr, _ := payload["error"].(string)
	if !strings.Contains(errStr, "ban_conflict") && !strings.Contains(errStr, "concurrent ban") {
		t.Fatalf("error body=%q", errStr)
	}
}

func TestDispatchUnbanBusyStill409(t *testing.T) {
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	runID, err := claimUnbanSlot(1, "busy-holder", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { releaseUnbanSlot(runID) })

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    "/v0/management/plugins/grok-inspection/unban",
		Body:    []byte(`{"auth_id":"other"}`),
		Headers: http.Header{"Authorization": []string{"Bearer test-pass"}},
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("busy status=%d want 409 body=%s", resp.StatusCode, string(resp.Body))
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != false || payload["missing"] != false {
		t.Fatalf("payload=%v", payload)
	}
}
