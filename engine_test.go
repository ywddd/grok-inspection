package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestSlowRetryWorkersUsesHalfWithBounds(t *testing.T) {
	tests := map[int]int{
		1:  1,
		2:  1,
		3:  2,
		6:  3,
		16: 8,
	}
	for workers, want := range tests {
		if got := slowRetryWorkers(workers); got != want {
			t.Fatalf("slowRetryWorkers(%d) = %d, want %d", workers, got, want)
		}
	}
}

func TestInspectionRetriesTimeoutsAfterPrimaryPhase(t *testing.T) {
	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	defer func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	}()

	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "a", Name: "a.json", Provider: "xai"},
		{AuthIndex: "b", Name: "b.json", Provider: "xai"},
		{AuthIndex: "c", Name: "c.json", Provider: "xai"},
	}
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: files}, nil
	}

	var mu sync.Mutex
	calls := map[string]int{}
	retryStartedEarly := false
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		mu.Lock()
		calls[file.AuthIndex]++
		attempt := calls[file.AuthIndex]
		if file.AuthIndex == "a" && attempt == 2 && (calls["b"] == 0 || calls["c"] == 0) {
			retryStartedEarly = true
		}
		mu.Unlock()

		if file.AuthIndex == "a" && attempt == 1 {
			return accountResult{
				AuthIndex:      file.AuthIndex,
				Name:           file.Name,
				Classification: "probe_error",
				Action:         "keep",
				ErrorMessage:   "HTTP probe timeout (25s)",
			}
		}
		return accountResult{
			AuthIndex:      file.AuthIndex,
			Name:           file.Name,
			Classification: "healthy",
			Action:         "keep",
		}
	}

	storePath := filepath.Join(t.TempDir(), "results.json")
	setStoreFilePathForTest(storePath)
	defer setStoreFilePathForTest("")

	e := &inspectionEngine{
		running: true,
		runID:   1,
		workers: 2,
	}
	e.run(1, 2, false, false, false, nil)

	mu.Lock()
	defer mu.Unlock()
	if retryStartedEarly {
		t.Fatal("timeout retry started before all primary probes completed")
	}
	if calls["a"] != 2 || calls["b"] != 1 || calls["c"] != 1 {
		t.Fatalf("calls = %#v", calls)
	}
	snap := e.snapshot(true)
	if snap.Done != 3 || snap.Total != 3 {
		t.Fatalf("progress = %d/%d", snap.Done, snap.Total)
	}
	if snap.RetryTotal != 1 || snap.RetryDone != 1 || snap.RetryWorkers != 1 {
		t.Fatalf("retry progress = %d/%d workers=%d", snap.RetryDone, snap.RetryTotal, snap.RetryWorkers)
	}
	if snap.ProbePhase != "finished" {
		t.Fatalf("phase = %q", snap.ProbePhase)
	}
	if len(snap.Results) != 3 {
		t.Fatalf("results = %d", len(snap.Results))
	}
	for _, result := range snap.Results {
		if result.Classification != "healthy" {
			t.Fatalf("unexpected final result: %+v", result)
		}
	}
}

func TestInspectionKeepsSecondTimeoutAsFinalResult(t *testing.T) {
	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	defer func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	}()

	file := pluginapi.HostAuthFileEntry{AuthIndex: "a", Name: "a.json", Provider: "xai"}
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{file}}, nil
	}
	calls := 0
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		calls++
		return accountResult{
			AuthIndex:      file.AuthIndex,
			Name:           file.Name,
			Classification: "probe_error",
			Action:         "keep",
			ErrorMessage:   "account probe timeout",
		}
	}

	storePath := filepath.Join(t.TempDir(), "results.json")
	setStoreFilePathForTest(storePath)
	defer setStoreFilePathForTest("")

	e := &inspectionEngine{
		running: true,
		runID:   1,
		workers: 16,
	}
	e.run(1, 16, false, false, false, nil)

	snap := e.snapshot(true)
	if calls != 2 {
		t.Fatalf("calls = %d, want primary + one slow retry", calls)
	}
	if snap.Done != 1 || snap.Total != 1 || len(snap.Results) != 1 {
		t.Fatalf("snapshot = done %d total %d results %d", snap.Done, snap.Total, len(snap.Results))
	}
	if snap.Results[0].Classification != "probe_error" || snap.Results[0].ErrorMessage != "account probe timeout" {
		t.Fatalf("result = %+v", snap.Results[0])
	}
	if snap.RetryWorkers != 8 {
		t.Fatalf("retry workers = %d", snap.RetryWorkers)
	}
}

func TestStopDuringRetryReturnsImmediatelyAndDiscardsLateResult(t *testing.T) {
	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	defer func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	}()

	file := pluginapi.HostAuthFileEntry{AuthIndex: "a", Name: "a.json", Provider: "xai"}
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{file}}, nil
	}
	retryStarted := make(chan struct{})
	releaseRetry := make(chan struct{})
	calls := 0
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		calls++
		if calls == 1 {
			return accountResult{
				AuthIndex:      file.AuthIndex,
				Name:           file.Name,
				Classification: "probe_error",
				Action:         "keep",
				ErrorMessage:   "HTTP probe timeout (25s)",
			}
		}
		close(retryStarted)
		<-releaseRetry
		return accountResult{
			AuthIndex:      file.AuthIndex,
			Name:           file.Name,
			Classification: "healthy",
			Action:         "keep",
		}
	}

	storePath := filepath.Join(t.TempDir(), "results.json")
	setStoreFilePathForTest(storePath)
	defer setStoreFilePathForTest("")

	e := &inspectionEngine{
		running: true,
		runID:   1,
		workers: 2,
	}
	runDone := make(chan struct{})
	go func() {
		e.run(1, 2, false, false, false, nil)
		close(runDone)
	}()
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("retry phase did not start")
	}

	started := time.Now()
	e.stop()
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("stop took %s", elapsed)
	}
	stopped := e.snapshot(true)
	if stopped.Running || !stopped.Stopped || stopped.ProbePhase != "stopped" {
		t.Fatalf("stopped snapshot = %+v", stopped)
	}

	close(releaseRetry)
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("run did not drain after retry was released")
	}
	final := e.snapshot(true)
	if len(final.Results) != 1 || final.Results[0].Classification == "healthy" {
		t.Fatalf("late retry must not overwrite stopped result: %+v", final.Results)
	}
}

func TestCallCPAManagementUsesBearerPasswordAndJSON(t *testing.T) {
	// Isolate process-global management credential cache so prior tests that
	// remembered a page Bearer (e.g. "test-pass") cannot shadow MANAGEMENT_PASSWORD.
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-management-password" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBaseURL := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() {
		setCPAManagementBaseURL(oldBaseURL)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
	}()

	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-management-password")

	status, _, err := callCPAManagement(http.MethodPatch, "/status", []byte(`{"disabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
}

func TestResolveManagementPasswordPrefersRequestBearer(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword) }()
	_ = os.Setenv("MANAGEMENT_PASSWORD", "env-password")

	headers := http.Header{"Authorization": []string{"Bearer page-password"}}
	if got := resolveManagementPassword(headers); got != "page-password" {
		t.Fatalf("password = %q, want page-password", got)
	}
	// Plugin-level cache remembers page key for realtime auto-disable paths.
	if got := resolveManagementPassword(nil); got != "page-password" {
		t.Fatalf("cached password = %q, want page-password", got)
	}
	clearManagementCredentialCacheForTest()
	if got := resolveManagementPassword(nil); got != "env-password" {
		t.Fatalf("env password = %q, want env-password", got)
	}
}

func TestCallCPAManagementWithAuthUsesRequestPasswordWithoutEnv(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer page-password" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBaseURL := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() {
		setCPAManagementBaseURL(oldBaseURL)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
	}()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Unsetenv("MANAGEMENT_PASSWORD")
	_ = os.Unsetenv("CPA_MANAGEMENT_KEY")

	status, _, err := callCPAManagementWithAuth(http.MethodPatch, "/status", []byte(`{"disabled":true}`), "page-password", nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
}

// Issue #22: custom CPA port resolution for management dial-back.
//
// Priority: explicit base URL env > PORT/CPA_PORT > Host port > Origin port >
// cached derived port > 8317. Only ports are used; always dial 127.0.0.1.
// Hostnames from Origin/Forwarded are never used for the default base URL.
//
// Real paths:
//   - stock CPA today: Headers have Origin but usually no Host (r.Host not cloned)
//   - CPA with host injection: Headers include Host from r.Host
func TestResolveManagementBaseURLUsesRequestHostPortByDefault(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	clearDerivedManagementPortCacheForTest()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		clearDerivedManagementPortCacheForTest()
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	setCPAManagementBaseURL("http://127.0.0.1:8317")

	// Host without port: do not invent a port from Host alone. Origin may still
	// supply an explicit port (see Origin-port test); here Origin has a port so
	// the derived base follows Origin's port on loopback — never the hostname.
	headers := http.Header{
		"Origin":            []string{"https://attacker.example:4443"},
		"X-Forwarded-Proto": []string{"https"},
		"X-Forwarded-Host":  []string{"attacker.example:9999"},
		"Host":              []string{"attacker.example"},
	}
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:4443" {
		t.Fatalf("origin-port loopback = %q, want 127.0.0.1:4443 (not forwarded host)", got)
	}
	clearDerivedManagementPortCacheForTest()

	// Injected/present Host port wins over Origin port; hostname discarded.
	headers = http.Header{
		"Origin": []string{"https://attacker.example:4443"},
		"Host":   []string{"cpa.example.com:1109"},
	}
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:1109" {
		t.Fatalf("host-port base url = %q, want loopback custom port", got)
	}

	headers.Set("Host", "203.0.113.9:18080")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:18080" {
		t.Fatalf("ipv4 host-port base url = %q", got)
	}

	headers.Set("Host", "[2001:db8::1]:19090")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:19090" {
		t.Fatalf("ipv6 host-port base url = %q", got)
	}

	// Malformed Host ports fall through; without Origin port -> default.
	clearDerivedManagementPortCacheForTest()
	for _, host := range []string{"cpa.example.com:notaport", "cpa.example.com:0", "cpa.example.com:70000", "[::1]", "127.0.0.1"} {
		headers = http.Header{"Host": []string{host}}
		if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:8317" {
			t.Fatalf("host %q base url = %q, want safe default", host, got)
		}
	}

	// Explicit env still wins over Host/Origin port.
	headers = http.Header{
		"Host":   []string{"cpa.example.com:1109"},
		"Origin": []string{"https://cpa.example.com:1109"},
	}
	_ = os.Setenv("PORT", "6550")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:6550" {
		t.Fatalf("PORT env should beat Host port: %q", got)
	}
	_ = os.Unsetenv("PORT")
	_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", "http://127.0.0.1:9999")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:9999" {
		t.Fatalf("explicit management base url = %q", got)
	}
}

// Real CPA integration path today: ServeManagementHTTP clones r.Header only, so
// Host is absent while browser Origin carries the page URL (including custom port).
func TestResolveManagementBaseURLUsesOriginPortOnStockCPAPath(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	clearDerivedManagementPortCacheForTest()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		clearDerivedManagementPortCacheForTest()
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	setCPAManagementBaseURL("http://127.0.0.1:8317")

	// Headers shaped like a real ManagementRequest from stock CPA:
	// Authorization/Origin present, Host absent, Forwarded may be spoofed.
	headers := http.Header{
		"Authorization":     []string{"Bearer page-key"},
		"Origin":            []string{"http://192.168.1.4:1109"},
		"X-Forwarded-Host":  []string{"evil.example:443"},
		"X-Forwarded-Proto": []string{"https"},
	}
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:1109" {
		t.Fatalf("stock CPA origin-port path = %q, want loopback:1109", got)
	}

	// Async / header-less follow-up reuses the cached port only.
	if got := resolveManagementBaseURL(nil); got != "http://127.0.0.1:1109" {
		t.Fatalf("cached derived port = %q, want 1109 for async workers", got)
	}

	// Forwarded alone never derives a port.
	clearDerivedManagementPortCacheForTest()
	headers = http.Header{
		"X-Forwarded-Host": []string{"evil.example:443"},
	}
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:8317" {
		t.Fatalf("forwarded-only must stay default: %q", got)
	}

	// Origin without explicit port (reverse-proxy 443/80) does not guess 443/80.
	clearDerivedManagementPortCacheForTest()
	headers = http.Header{"Origin": []string{"https://cpa.example.com"}}
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:8317" {
		t.Fatalf("origin without port must not assume 443: %q", got)
	}
}

// Integration: stock CPA ManagementRequest shape has Origin (browser) but no Host
// (r.Host is not cloned into Headers). The dial must hit 127.0.0.1:<origin-port>,
// and a later header-less background call must reuse the cached port.
func TestCallCPAManagementDialsLoopbackOriginPortOnStockCPAHeaders(t *testing.T) {
	clearManagementCredentialCacheForTest()
	clearDerivedManagementPortCacheForTest()
	t.Cleanup(func() {
		clearManagementCredentialCacheForTest()
		clearDerivedManagementPortCacheForTest()
	})

	var mu sync.Mutex
	var hits []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.Host+" "+r.URL.Path)
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
		t.Fatalf("test server port empty: %s", server.URL)
	}
	wantHost := "127.0.0.1:" + port

	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		setCPAManagementDo(oldDo)
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	setCPAManagementBaseURL("http://127.0.0.1:8317")

	var dialHosts []string
	setCPAManagementDo(func(req *http.Request) (*http.Response, error) {
		dialHosts = append(dialHosts, req.URL.Host)
		return server.Client().Do(req)
	})

	// Stock CPA headers: Origin present, Host absent, Forwarded untrusted.
	headers := http.Header{
		"Origin":            []string{"http://cpa.example.lan:" + port},
		"X-Forwarded-Host":  []string{"evil.example:443"},
		"X-Forwarded-Proto": []string{"https"},
	}
	status, _, err := callCPAManagementWithAuth(
		http.MethodPatch,
		"/v0/management/auth-files/status",
		[]byte(`{"disabled":true}`),
		"page-key",
		headers,
	)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if len(dialHosts) != 1 || dialHosts[0] != wantHost {
		t.Fatalf("first dial host = %#v, want [%q]", dialHosts, wantHost)
	}

	// Background / async path: no headers, must use cached derived port.
	status, _, err = callCPAManagementWithAuth(
		http.MethodPatch,
		"/v0/management/auth-files/status",
		[]byte(`{"disabled":true}`),
		"page-key",
		nil,
	)
	if err != nil {
		t.Fatalf("cached dial: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("cached status = %d", status)
	}
	if len(dialHosts) != 2 || dialHosts[1] != wantHost {
		t.Fatalf("cached dial hosts = %#v, want two %q", dialHosts, wantHost)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hits) != 2 {
		t.Fatalf("server hits = %#v, want 2", hits)
	}
}

func TestNormalizeHTTPOriginRejectsNonOriginValues(t *testing.T) {
	tests := map[string]string{
		"https://cpa.example.com:1109":         "https://cpa.example.com:1109",
		"https://cpa.example.com/":             "https://cpa.example.com",
		"https://user@cpa.example.com":         "",
		"https://cpa.example.com/management":   "",
		"https://cpa.example.com?next=x":       "",
		"https://cpa.example.com?":             "",
		"https://cpa.example.com#fragment":     "",
		"https://a.example, https://b.example": "",
		"file:///tmp/cpa":                      "",
		"null":                                 "",
	}
	for input, want := range tests {
		if got := normalizeHTTPOrigin(input); got != want {
			t.Fatalf("normalizeHTTPOrigin(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCallCPAManagementRetriesOriginAfterUnreachablePORT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer page-password" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDo := getCPAManagementDo()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementDo(oldDo)
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Setenv("PORT", "65530")
	_ = os.Unsetenv("CPA_PORT")
	var calls []string
	setCPAManagementDo(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		if req.URL.Host == "127.0.0.1:65530" {
			return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNREFUSED}
		}
		return server.Client().Do(req)
	})

	status, _, err := callCPAManagementWithAuth(
		http.MethodPatch,
		"/v0/management/auth-files/status",
		[]byte(`{"disabled":true}`),
		"page-password",
		http.Header{"Origin": []string{server.URL}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "http://127.0.0.1:65530/") || !strings.HasPrefix(calls[1], server.URL+"/") {
		t.Fatalf("unexpected retry order: %#v", calls)
	}
}

func TestCallCPAManagementDoesNotLeakKeyToOriginWhenEnvConfigured(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldDo := getCPAManagementDo()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		setCPAManagementDo(oldDo)
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", "http://127.0.0.1:65531")
	var calls []string
	setCPAManagementDo(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNREFUSED}
	})

	_, _, err := callCPAManagementWithAuth(
		http.MethodDelete,
		"/v0/management/auth-files",
		nil,
		"page-password",
		http.Header{"Origin": []string{"https://attacker.example"}},
	)
	if err == nil {
		t.Fatal("expected configured management endpoint failure")
	}
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "http://127.0.0.1:65531/") {
		t.Fatalf("configured endpoint should not fall back to Origin: %#v", calls)
	}
}

func TestCallCPAManagementDoesNotRetryForwardedOrHostHeaders(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDo := getCPAManagementDo()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementDo(oldDo)
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Setenv("PORT", "65530")
	_ = os.Unsetenv("CPA_PORT")
	var calls []string
	setCPAManagementDo(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNREFUSED}
	})

	_, _, err := callCPAManagementWithAuth(
		http.MethodDelete,
		"/v0/management/auth-files",
		nil,
		"page-password",
		http.Header{
			"X-Forwarded-Proto": []string{"https"},
			"X-Forwarded-Host":  []string{"attacker.example"},
			"Host":              []string{"attacker.example"},
		},
	)
	if err == nil {
		t.Fatal("expected local management endpoint failure")
	}
	if len(calls) != 1 || !strings.HasPrefix(calls[0], "http://127.0.0.1:65530/") {
		t.Fatalf("untrusted forwarded/host headers must not be retried: %#v", calls)
	}
}

func TestCallCPAManagementDoesNotRetryOriginAfterHTTPError(t *testing.T) {
	localCalls := 0
	originCalls := 0
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		localCalls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer local.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		originCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer origin.Close()

	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		setCPAManagementDo(oldDo)
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	// Pin the primary endpoint via env so Origin's port cannot become the default
	// loopback base (issue #22). This test covers HTTP-error non-retry only.
	_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", local.URL)
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	setCPAManagementBaseURL(local.URL)
	setCPAManagementDo(local.Client().Do)
	clearDerivedManagementPortCacheForTest()

	_, _, err := callCPAManagementWithAuth(
		http.MethodPatch,
		"/v0/management/auth-files/status",
		[]byte(`{"disabled":true}`),
		"page-password",
		http.Header{"Origin": []string{origin.URL}},
	)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected local HTTP 401, got %v", err)
	}
	if localCalls != 1 || originCalls != 0 {
		t.Fatalf("HTTP errors must not retry Origin: local=%d origin=%d", localCalls, originCalls)
	}
}

func TestStartRejectsInvalidWorkers(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers}
	err := e.start(startRequest{Workers: 99})
	if err == nil {
		t.Fatal("expected error")
	}
	if statusFromError(err, 0) != http.StatusBadRequest {
		t.Fatalf("status = %d err=%v", statusFromError(err, 0), err)
	}
	// Message is localized; do not depend on English-only wording.
	msg := err.Error()
	if !(strings.Contains(msg, "workers") || strings.Contains(msg, "并发") || strings.Contains(msg, "Workers") || strings.Contains(msg, "1") && strings.Contains(msg, "16")) {
		t.Fatalf("err = %v", err)
	}
}

func TestIncrementalStartRequiresExistingResults(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers}
	err := e.start(startRequest{Workers: 2, Incremental: true})
	if err == nil || !(strings.Contains(err.Error(), "增量巡检") || strings.Contains(err.Error(), "Incremental") || strings.Contains(err.Error(), "incremental")) {
		t.Fatalf("err = %v", err)
	}
}

func TestStableIdentityPrefersAuthIndexNotEmail(t *testing.T) {
	// Same email must NOT cause skip when auth_index differs (re-import new token).
	known := knownResultKeys([]accountResult{
		{AuthIndex: "old-ai", FileName: "a.json", Email: "same@x.com", Name: "same@x.com"},
	})
	// New runtime index, same email/name → not known
	if entryIsKnown(known, pluginapi.HostAuthFileEntry{
		AuthIndex: "new-ai",
		Name:      "a.json",
		Email:     "same@x.com",
		Label:     "same@x.com",
	}) {
		t.Fatal("same email with different auth_index must not be treated as known")
	}
	// Same auth_index → known
	if !entryIsKnown(known, pluginapi.HostAuthFileEntry{AuthIndex: "old-ai", Name: "other.json"}) {
		t.Fatal("same auth_index should be known")
	}
	// No auth_index: file name+size+mtime must match
	known2 := knownResultKeys([]accountResult{
		{FileName: "b.json", FileSize: 10, FileModUnix: 100},
	})
	if !entryIsKnown(known2, pluginapi.HostAuthFileEntry{
		Name:    "b.json",
		Size:    10,
		ModTime: time.Unix(100, 0),
	}) {
		t.Fatal("matching file fingerprint should be known")
	}
	if entryIsKnown(known2, pluginapi.HostAuthFileEntry{
		Name:    "b.json",
		Size:    11, // rewritten file
		ModTime: time.Unix(100, 0),
	}) {
		t.Fatal("changed file size should force re-inspect")
	}
}

func TestStartActionReturnsSeqAndReportsOnStatus(t *testing.T) {
	old := engine
	engine = &inspectionEngine{workers: defaultWorkers}
	t.Cleanup(func() {
		engine.runWG.Wait()
		engine.waitAsyncPersist()
		engine = old
	})

	// Missing password will fail delete quickly; still records recent_row_actions.
	seq, action, err := engine.startAction(actionRequest{
		Name:   "missing.json",
		Delete: true,
	}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if seq == 0 || action != "delete" {
		t.Fatalf("seq=%d action=%q", seq, action)
	}
	deadline := time.Now().Add(2 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		snap := engine.snapshot(false)
		for _, a := range snap.RecentRowActions {
			if a.Seq == seq {
				found = true
				if a.OK {
					t.Fatal("expected failed action without management password")
				}
				if a.Error == "" {
					t.Fatal("expected error text on failed action")
				}
				break
			}
		}
		if found {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !found {
		t.Fatal("recent_row_actions never reported action_seq")
	}
}

func TestDeleteAuthFilesBatchBuildsNamesBody(t *testing.T) {
	// Smoke: empty input is a no-op.
	if fails := deleteAuthFilesBatch(nil, "x", nil, false); len(fails) != 0 {
		t.Fatalf("empty batch failures = %#v", fails)
	}
	// Missing file names should fail locally without calling management HTTP.
	fails := deleteAuthFilesBatch([]accountResult{
		{Name: "", AuthIndex: "", FileName: ""},
	}, "x", nil, false)
	if len(fails) != 1 || !strings.Contains(fails[0], "auth file name missing") {
		t.Fatalf("failures = %#v", fails)
	}
}

func TestApplyIsAsyncAndStatusStaysResponsive(t *testing.T) {
	dir := t.TempDir()
	setStoreFilePathForTest(dir + string(os.PathSeparator) + "results.json")
	t.Cleanup(func() { setStoreFilePathForTest("") })

	// Hold the CPA DELETE inside the handler so applying=true is observable.
	// Without a barrier, a fast/failing delete can finish before snapshot/status.
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			select {
			case <-entered:
			default:
				close(entered)
			}
			<-release
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "page-password")
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
		select {
		case <-release:
		default:
			close(release)
		}
	})

	old := engine
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{Name: "need-reauth", AuthIndex: "a1", FileName: "a1.json", Classification: "reauth", Action: "delete"},
		},
	}
	t.Cleanup(func() {
		engine.runWG.Wait()
		engine.waitAsyncPersist()
		engine = old
	})

	begin := time.Now()
	if err := engine.startApply(applyRequest{
		ForceAction: "delete",
		AuthIndexes: []string{"a1"},
	}, "page-password", nil); err != nil {
		t.Fatal(err)
	}
	if time.Since(begin) > 100*time.Millisecond {
		t.Fatalf("startApply should return immediately, took %s", time.Since(begin))
	}

	// Wait until the background delete is blocked in the CPA handler.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("delete handler never entered; cannot assert applying state")
	}

	snap := engine.snapshot(false)
	if !snap.Applying {
		close(release)
		t.Fatal("expected applying=true while delete is in flight")
	}
	if snap.IncludeResults {
		close(release)
		t.Fatal("light snapshot should set include_results=false")
	}
	if len(snap.Results) != 0 {
		close(release)
		t.Fatalf("light snapshot should omit results, got %d", len(snap.Results))
	}
	// status path is pure memory and must not wait on apply/delete work
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
	})
	if resp.StatusCode != http.StatusOK {
		close(release)
		t.Fatalf("status code = %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), `"applying":true`) {
		close(release)
		t.Fatalf("status body missing applying=true: %s", string(resp.Body))
	}

	close(release)
	engine.runWG.Wait()
	engine.waitAsyncPersist()
}

func TestShutdownWaitsForAsyncPersist(t *testing.T) {
	// Controllable barrier: lifecycle wait must not return while async save runs.
	entered := make(chan struct{})
	release := make(chan struct{})
	oldHook := persistAsyncBeforeSave
	persistAsyncBeforeSave = func() {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
	}
	t.Cleanup(func() {
		persistAsyncBeforeSave = oldHook
		select {
		case <-release:
		default:
			close(release)
		}
	})

	dir := t.TempDir()
	setStoreFilePathForTest(filepath.Join(dir, "results.json"))
	t.Cleanup(func() {
		setStoreFilePathForTest("")
	})

	e := &inspectionEngine{workers: defaultWorkers}
	e.mu.Lock()
	e.results = []accountResult{{Name: "persist-me", AuthIndex: "p1"}}
	e.persistLocked()
	e.mu.Unlock()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("async persist never reached before-save hook")
	}

	done := make(chan struct{})
	go func() {
		// Same order as shutdown after run/unban: wait async persist writers.
		e.waitAsyncPersist()
		close(done)
	}()

	select {
	case <-done:
		close(release)
		t.Fatal("waitAsyncPersist returned before async save finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waitAsyncPersist did not observe async save completion")
	}
}

func TestClassifyScopedStartRequiresExistingResults(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers}
	err := e.start(startRequest{Workers: 2, Classifications: []string{"quota_exhausted"}})
	if err == nil || !(strings.Contains(err.Error(), "分类巡检") || strings.Contains(err.Error(), "Category") || strings.Contains(err.Error(), "category")) {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyScopedRejectsWithIncremental(t *testing.T) {
	e := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{{AuthIndex: "a1", Classification: "quota_exhausted"}},
	}
	err := e.start(startRequest{Workers: 2, Incremental: true, Classifications: []string{"quota_exhausted"}})
	if err == nil || !(strings.Contains(err.Error(), "分类巡检") || strings.Contains(err.Error(), "Category") || strings.Contains(err.Error(), "category")) {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyScopedRejectsEmptyMatch(t *testing.T) {
	e := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{{AuthIndex: "a1", Classification: "healthy"}},
	}
	err := e.start(startRequest{Workers: 2, Classifications: []string{"reauth"}})
	if err == nil || !(strings.Contains(err.Error(), "当前分类") || strings.Contains(err.Error(), "No inspectable") || strings.Contains(err.Error(), "category")) {
		t.Fatalf("err = %v", err)
	}
}

func TestClassificationMatchesOther(t *testing.T) {
	want := stringSet([]string{"other"})
	if !classificationMatches("probe_error", want) {
		t.Fatal("probe_error should match other")
	}
	if !classificationMatches("model_unavailable", want) {
		t.Fatal("model_unavailable should match other")
	}
	if classificationMatches("healthy", want) {
		t.Fatal("healthy should not match other")
	}
	if classificationMatches("quota_exhausted", want) {
		t.Fatal("quota_exhausted should not match other")
	}
	wantQuota := stringSet([]string{"quota_exhausted"})
	if !classificationMatches("quota_exhausted", wantQuota) {
		t.Fatal("exact class should match")
	}
	if classificationMatches("reauth", wantQuota) {
		t.Fatal("other class should not match exact set")
	}
}

func TestNormalizeClassifications(t *testing.T) {
	got := normalizeClassifications([]string{" reauth ", "quota_exhausted", "reauth", ""})
	if len(got) != 2 || got[0] != "quota_exhausted" || got[1] != "reauth" {
		t.Fatalf("got=%v", got)
	}
}

func TestFindResultIndexAndResolveTargets(t *testing.T) {
	results := []accountResult{
		{AuthIndex: "a1", FileName: "a.json", Classification: "quota_exhausted"},
		{AuthIndex: "a2", FileName: "b.json", Classification: "healthy"},
	}
	if idx := findResultIndex(results, accountResult{AuthIndex: "a1"}); idx != 0 {
		t.Fatalf("idx=%d", idx)
	}
	if idx := findResultIndex(results, accountResult{FileName: "b.json"}); idx != 1 {
		t.Fatalf("idx=%d", idx)
	}
	if idx := findResultIndex(results, accountResult{AuthIndex: "missing"}); idx != -1 {
		t.Fatalf("idx=%d", idx)
	}

	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "a1", Name: "a.json", Provider: "xai"},
	}
	selected := []accountResult{
		{AuthIndex: "a1", FileName: "a.json", Classification: "quota_exhausted"},
		{AuthIndex: "gone", FileName: "gone.json", Classification: "quota_exhausted"},
	}
	targets, missing := resolveClassifyTargets(files, selected)
	if len(targets) != 1 || targets[0].AuthIndex != "a1" {
		t.Fatalf("targets=%+v", targets)
	}
	if len(missing) != 1 || missing[0].AuthIndex != "gone" {
		t.Fatalf("missing=%+v", missing)
	}
}

func TestUpsertResultReplacesByAuthIndex(t *testing.T) {
	e := &inspectionEngine{
		workers: defaultWorkers,
		runID:   7,
		results: []accountResult{
			{AuthIndex: "a1", FileName: "a.json", Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "a2", FileName: "b.json", Classification: "healthy", Action: "keep"},
		},
	}
	e.upsertResult(7, accountResult{
		AuthIndex:      "a1",
		FileName:       "a.json",
		Classification: "healthy",
		Action:         "keep",
		Reason:         "ok",
	})
	if len(e.results) != 2 {
		t.Fatalf("len=%d", len(e.results))
	}
	if e.results[0].Classification != "healthy" || e.results[0].Action != "keep" {
		t.Fatalf("row0=%+v", e.results[0])
	}
	if e.results[1].Classification != "healthy" {
		t.Fatalf("row1 should stay healthy: %+v", e.results[1])
	}
	if e.probeDone != 1 {
		t.Fatalf("probeDone=%d", e.probeDone)
	}
}

func TestResolveManagementBaseURLUsesHTTPSWhenTLSEnvSet(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldTLS := os.Getenv("CPA_TLS")
	oldDefault := getCPAManagementBaseURL()
	clearDerivedManagementPortCacheForTest()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		_ = os.Setenv("CPA_TLS", oldTLS)
		setCPAManagementBaseURL(oldDefault)
		clearDerivedManagementPortCacheForTest()
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	_ = os.Setenv("CPA_TLS", "true")
	setCPAManagementBaseURL("http://127.0.0.1:8317")

	if got := resolveManagementBaseURL(nil); got != "https://127.0.0.1:8317" {
		t.Fatalf("tls base url = %q", got)
	}
	_ = os.Setenv("PORT", "9443")
	if got := resolveManagementBaseURL(nil); got != "https://127.0.0.1:9443" {
		t.Fatalf("tls port base url = %q", got)
	}
	_ = os.Unsetenv("PORT")
	// TLS + custom Host port still stays on loopback https.
	headers := http.Header{"Host": []string{"cpa.example.com:10443"}}
	if got := resolveManagementBaseURL(headers); got != "https://127.0.0.1:10443" {
		t.Fatalf("tls host-port base url = %q", got)
	}
}

// Origin hostname is never used for the default base URL — only its port, and
// always against loopback. Full Origin URL remains a transport-failure retry
// path (#18) separate from this default resolution.
func TestResolveManagementBaseURLNeverUsesOriginHostname(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	clearDerivedManagementPortCacheForTest()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		clearDerivedManagementPortCacheForTest()
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	setCPAManagementBaseURL("http://127.0.0.1:8317")

	headers := http.Header{
		"Origin":           []string{"https://cpa.example.com:1109"},
		"X-Forwarded-Host": []string{"evil.example:443"},
	}
	got := resolveManagementBaseURL(headers)
	if got != "http://127.0.0.1:1109" {
		t.Fatalf("base url = %q, want loopback port from Origin only", got)
	}
	if strings.Contains(got, "cpa.example.com") || strings.Contains(got, "evil.example") {
		t.Fatalf("hostname leaked into default base url: %q", got)
	}
}

// Invalid PORT/CPA_PORT must not become a broken base URL that blocks Host/Origin.
func TestResolveManagementBaseURLIgnoresInvalidPORTEnv(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	clearDerivedManagementPortCacheForTest()
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		clearDerivedManagementPortCacheForTest()
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	setCPAManagementBaseURL("http://127.0.0.1:8317")

	headers := http.Header{
		"Host":   []string{"cpa.example.com:1109"},
		"Origin": []string{"http://cpa.example.com:1109"},
	}
	for _, bad := range []string{"", "abc", "0", "70000", ":-1", ":notaport"} {
		_ = os.Setenv("PORT", bad)
		_ = os.Unsetenv("CPA_PORT")
		if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:1109" {
			t.Fatalf("PORT=%q should fall through to Host port, got %q", bad, got)
		}
		clearDerivedManagementPortCacheForTest()
	}
	// Valid CPA_PORT still wins after TrimPrefix of leading colon.
	_ = os.Unsetenv("PORT")
	_ = os.Setenv("CPA_PORT", ":9444")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:9444" {
		t.Fatalf("valid CPA_PORT = %q", got)
	}

	// Garbage PORT must not shadow a later valid CPA_PORT.
	clearDerivedManagementPortCacheForTest()
	_ = os.Setenv("PORT", "not-a-port")
	_ = os.Setenv("CPA_PORT", "12001")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:12001" {
		t.Fatalf("valid CPA_PORT after garbage PORT = %q, want 12001", got)
	}
}

func TestCallCPAManagementRetriesHTTPSAfterPlainHTTPMismatch(t *testing.T) {
	clearManagementCredentialCacheForTest()
	t.Cleanup(clearManagementCredentialCacheForTest)
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tls-pass" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer tlsServer.Close()

	// Parse host:port from TLS server and build plain-http base that will fail protocol-wise.
	u := strings.TrimPrefix(tlsServer.URL, "https://")
	httpBase := "http://" + u

	oldBaseURL := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	oldCPABase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	defer func() {
		setCPAManagementBaseURL(oldBaseURL)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
		_ = os.Setenv("CPA_BASE_URL", oldCPABase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
	}()

	// Force resolve to plain http against the TLS listener.
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", httpBase)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "tls-pass")
	// Use real client that accepts the test cert via InsecureSkipVerify in plugin client.
	setCPAManagementDo(cpaManagementClient.Do)

	status, _, err := callCPAManagement(http.MethodPatch, "/v0/management/auth-files/status", []byte(`{"disabled":true}`))
	if err != nil {
		t.Fatalf("expected https retry success, got err=%v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
}

func TestShouldRetryManagementWithHTTPS(t *testing.T) {
	if !shouldRetryManagementWithHTTPS("http://127.0.0.1:8317", fmt.Errorf(`Patch "http://127.0.0.1:8317/x": EOF`)) {
		t.Fatal("expected retry on loopback EOF")
	}
	if shouldRetryManagementWithHTTPS("https://127.0.0.1:8317", fmt.Errorf("EOF")) {
		t.Fatal("should not retry when already https")
	}
	if shouldRetryManagementWithHTTPS("http://example.com:8317", fmt.Errorf("EOF")) {
		t.Fatal("should not retry non-loopback")
	}
}
