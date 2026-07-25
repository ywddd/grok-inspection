package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func withIsolatedManagementEnv(t *testing.T) {
	t.Helper()
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	clearDerivedManagementPortCacheForTest()
	clearManagementCredentialCacheForTest()
	t.Cleanup(func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		setCPAManagementDo(oldDo)
		clearDerivedManagementPortCacheForTest()
		clearManagementCredentialCacheForTest()
	})
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	setCPAManagementBaseURL("http://127.0.0.1:8317")
}

// Issue #22 follow-up: saving schedule never dials CPA management itself.
// Port cache must still warm from the trusted ManagementRequest so the first
// headless schedule/autoban action dials the custom port, not 8317.
func TestDispatchScheduleSaveWarmsPortCacheForHeadlessCall(t *testing.T) {
	withIsolatedManagementEnv(t)

	var mu sync.Mutex
	var dialHosts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		dialHosts = append(dialHosts, r.Host)
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer page-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port := u.Port()
	if port == "" {
		t.Fatalf("empty server port: %s", server.URL)
	}
	wantHost := "127.0.0.1:" + port

	setCPAManagementDo(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		dialHosts = append(dialHosts, "do:"+req.URL.Host)
		mu.Unlock()
		return server.Client().Do(req)
	})

	// POST /schedule only persists config; it must not require a management dial.
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/schedule",
		Body:   []byte(`{"enabled":false,"interval_minutes":60}`),
		Headers: http.Header{
			"Authorization": []string{"Bearer page-key"},
			"Origin":        []string{"http://192.168.1.4:" + port},
			// Spoofed forwarded must not become the dial target.
			"X-Forwarded-Host": []string{"evil.example:443"},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schedule save status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	mu.Lock()
	if len(dialHosts) != 0 {
		t.Fatalf("schedule save should not dial management: %#v", dialHosts)
	}
	mu.Unlock()

	// Headless background call after schedule save must reuse warmed port.
	status, _, err := callCPAManagementWithAuth(
		http.MethodPatch,
		"/v0/management/auth-files/status",
		[]byte(`{"disabled":true}`),
		"page-key",
		nil,
	)
	if err != nil {
		t.Fatalf("headless dial after schedule save: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dialHosts) == 0 || !strings.HasSuffix(dialHosts[len(dialHosts)-1], wantHost) {
		t.Fatalf("headless dial hosts = %#v, want host ending with %q", dialHosts, wantHost)
	}
	for _, h := range dialHosts {
		if strings.Contains(h, "8317") {
			t.Fatalf("must not fall back to 8317: %#v", dialHosts)
		}
	}
}

// Issue #22 follow-up: CPA host injection may provide Host without Origin.
// Unban must keep Host in the detached route context and dial that loopback port.
func TestDispatchUnbanUsesHostPortWithoutOrigin(t *testing.T) {
	withIsolatedManagementEnv(t)
	isolateActiveStore(t)

	var mu sync.Mutex
	var dialHosts []string
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		dialHosts = append(dialHosts, r.Host)
		if r.Header.Get("Authorization") == "Bearer page-key" {
			sawAuth = true
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port := u.Port()
	if port == "" {
		t.Fatalf("empty server port: %s", server.URL)
	}
	wantHost := "127.0.0.1:" + port

	setCPAManagementDo(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		dialHosts = append(dialHosts, "do:"+req.URL.Host)
		mu.Unlock()
		return server.Client().Do(req)
	})

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      "host-only-unban",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now,
		ResetAt:     now.Add(24 * time.Hour),
		ResetSource: "manual_unban",
		CpaSynced:   true,
	})

	// Live request headers include secrets; detached route context must drop them.
	liveHeaders := http.Header{
		"Authorization": []string{"Bearer page-key"},
		"Cookie":        []string{"session=secret"},
		"Host":          []string{"cpa.example.com:" + port},
		// No Origin on purpose.
	}
	// Sanity: route helper keeps Host, drops secrets.
	route := managementRouteHeaders(liveHeaders)
	if route.Get("Host") == "" {
		t.Fatal("managementRouteHeaders dropped Host")
	}
	if route.Get("Authorization") != "" || route.Get("Cookie") != "" {
		t.Fatalf("route headers leaked secrets: %#v", route)
	}
	if route.Get("Origin") != "" {
		t.Fatalf("unexpected Origin: %#v", route)
	}

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    "/v0/management/plugins/grok-inspection/unban",
		Body:    []byte(`{"auth_id":"host-only-unban"}`),
		Headers: liveHeaders,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unban status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["ok"] != true {
		t.Fatalf("unban body = %#v", body)
	}
	if _, ok := activeStore.Get("host-only-unban"); ok {
		t.Fatal("local ban should be removed")
	}

	mu.Lock()
	defer mu.Unlock()
	if !sawAuth {
		t.Fatal("enable PATCH missing Authorization")
	}
	if len(dialHosts) == 0 {
		t.Fatal("no management dials")
	}
	for _, h := range dialHosts {
		if strings.Contains(h, "8317") {
			t.Fatalf("must not fall back to 8317: %#v", dialHosts)
		}
		if strings.HasPrefix(h, "do:") && h != "do:"+wantHost {
			t.Fatalf("dial host = %q, want do:%s (hosts=%#v)", h, wantHost, dialHosts)
		}
	}
}

func TestManagementRouteHeadersDropsSecretsAndKeepsHostOrigin(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer secret"},
		"Cookie":        []string{"a=b"},
		"Host":          []string{"cpa.local:1109"},
		"Origin":        []string{"https://cpa.local:1109/path"},
		"X-Forwarded-Host": []string{"evil:443"},
	}
	// Origin with path must be rejected by normalize; only Host remains.
	got := managementRouteHeaders(headers)
	if got.Get("Host") != "cpa.local:1109" {
		t.Fatalf("host = %q", got.Get("Host"))
	}
	if got.Get("Origin") != "" {
		t.Fatalf("invalid origin must not be kept: %q", got.Get("Origin"))
	}
	if got.Get("Authorization") != "" || got.Get("Cookie") != "" || got.Get("X-Forwarded-Host") != "" {
		t.Fatalf("leaked fields: %#v", got)
	}

	headers = http.Header{
		"Host":   []string{"cpa.local:1109"},
		"Origin": []string{"https://cpa.local:1109"},
	}
	got = managementRouteHeaders(headers)
	if got.Get("Host") != "cpa.local:1109" || got.Get("Origin") != "https://cpa.local:1109" {
		t.Fatalf("route = %#v", got)
	}
}
