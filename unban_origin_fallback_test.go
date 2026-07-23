package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

// installUnreachableDefaultPORTWithOriginDial points primary management dial at an
// unreachable loopback PORT while allowing Origin-target traffic via doOrigin.
func installUnreachableDefaultPORTWithOriginDial(t *testing.T, doOrigin func(*http.Request) (*http.Response, error), callLog *[]string, logMu *sync.Mutex) {
	t.Helper()
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	t.Cleanup(func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		setCPAManagementBaseURL(oldDefault)
		setCPAManagementDo(oldDo)
	})
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Setenv("PORT", "65530")
	_ = os.Unsetenv("CPA_PORT")
	// Keep default base on loopback so resolveManagementBaseURL uses PORT, not a reachable override.
	setCPAManagementBaseURL("http://127.0.0.1:8317")
	setCPAManagementDo(func(req *http.Request) (*http.Response, error) {
		if logMu != nil && callLog != nil {
			logMu.Lock()
			*callLog = append(*callLog, req.URL.String())
			logMu.Unlock()
		}
		if req.URL.Host == "127.0.0.1:65530" {
			return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: syscall.ECONNREFUSED}
		}
		return doOrigin(req)
	})
}

// Issue #18: single-account /unban must Origin-fallback when default PORT is unreachable.
func TestDispatchUnbanUsesOriginWhenDefaultPortUnreachable(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		mu           sync.Mutex
		originHits   int
		authHeaders  []string
		requestHosts []string
		callLog      []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		originHits++
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		requestHosts = append(requestHosts, r.Host)
		mu.Unlock()
		if r.Method != http.MethodPatch || !strings.HasSuffix(r.URL.Path, "/v0/management/auth-files/status") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	installUnreachableDefaultPORTWithOriginDial(t, server.Client().Do, &callLog, &mu)

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "origin-unban-1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})

	// Mutate-prone map: after dispatch returns, flip secrets/origin to prove
	// the unban path must not keep a live reference to the request headers.
	headers := http.Header{
		"Authorization": []string{"Bearer page-password"},
		"Cookie":        []string{"session=should-not-propagate"},
		"Origin":        []string{server.URL},
	}

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    "/v0/management/plugins/grok-inspection/unban",
		Body:    []byte(`{"auth_id":"origin-unban-1"}`),
		Headers: headers,
	})
	headers.Set("Authorization", "Bearer mutated-after-request")
	headers.Set("Origin", "https://attacker.example")
	headers.Set("Cookie", "session=mutated")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", resp.StatusCode, string(resp.Body))
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["enabled"] != true || payload["removed"] != true {
		t.Fatalf("payload=%v", payload)
	}
	if _, ok := activeStore.Get("origin-unban-1"); ok {
		t.Fatal("local ban should be removed after successful unban")
	}

	mu.Lock()
	defer mu.Unlock()
	if originHits < 1 {
		t.Fatalf("origin server never hit; calls=%#v", callLog)
	}
	for _, auth := range authHeaders {
		if auth != "Bearer page-password" {
			t.Fatalf("authorization=%q want Bearer page-password (must use resolved password, not raw header map)", auth)
		}
	}
	originHost := strings.TrimPrefix(strings.TrimPrefix(server.URL, "https://"), "http://")
	for _, host := range requestHosts {
		if host != originHost {
			t.Fatalf("request host=%q want origin host %q", host, originHost)
		}
	}
	sawLoopbackFail := false
	sawOrigin := false
	for _, u := range callLog {
		if strings.HasPrefix(u, "http://127.0.0.1:65530/") {
			sawLoopbackFail = true
		}
		if strings.HasPrefix(u, server.URL+"/") {
			sawOrigin = true
		}
	}
	if !sawLoopbackFail || !sawOrigin {
		t.Fatalf("expected loopback refuse then Origin success; calls=%#v", callLog)
	}
}

// Issue #18: async /unban-all must snapshot Origin and complete via Origin server.
func TestDispatchUnbanAllUsesOriginWhenDefaultPortUnreachable(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		mu           sync.Mutex
		originHits   int
		authHeaders  []string
		requestHosts []string
		callLog      []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		originHits++
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		requestHosts = append(requestHosts, r.Host)
		mu.Unlock()
		if r.Method != http.MethodPatch || !strings.HasSuffix(r.URL.Path, "/v0/management/auth-files/status") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	installUnreachableDefaultPORTWithOriginDial(t, server.Client().Do, &callLog, &mu)

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "origin-unban-all-1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	activeStore.Set(banEntry{
		AuthID: "origin-unban-all-2", Provider: "xai", ErrorCode: unauthorizedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.AddDate(10, 0, 0),
		ResetSource: "manual_unban", CpaSynced: true,
	})

	headers := http.Header{
		"Authorization": []string{"Bearer page-password"},
		"Cookie":        []string{"session=should-not-propagate"},
		"Origin":        []string{server.URL},
	}

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    "/v0/management/plugins/grok-inspection/unban-all",
		Body:    []byte(`{"auth_ids":["origin-unban-all-1","origin-unban-all-2"]}`),
		Headers: headers,
	})
	// Mutate request headers after handler returns; async worker must already hold a snapshot.
	headers.Set("Authorization", "Bearer mutated-after-request")
	headers.Set("Origin", "https://attacker.example")
	headers.Del("Cookie")

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d want 202 body=%s", resp.StatusCode, string(resp.Body))
	}

	done := make(chan struct{})
	go func() {
		unbanJob.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("unban-all job did not finish")
	}

	st := unbanJobStatus()
	if st["running"] != false {
		t.Fatalf("job still running: %#v", st)
	}
	if st["enabled"] != 2 || st["failed"] != 0 || st["done"] != 2 {
		t.Fatalf("job status=%#v want enabled=2 failed=0 done=2", st)
	}
	if _, ok := activeStore.Get("origin-unban-all-1"); ok {
		t.Fatal("origin-unban-all-1 still banned")
	}
	if _, ok := activeStore.Get("origin-unban-all-2"); ok {
		t.Fatal("origin-unban-all-2 still banned")
	}

	mu.Lock()
	defer mu.Unlock()
	if originHits < 2 {
		t.Fatalf("origin hits=%d want >=2; calls=%#v", originHits, callLog)
	}
	for _, auth := range authHeaders {
		if auth != "Bearer page-password" {
			t.Fatalf("authorization=%q want Bearer page-password", auth)
		}
	}
	originHost := strings.TrimPrefix(strings.TrimPrefix(server.URL, "https://"), "http://")
	for _, host := range requestHosts {
		if host != originHost {
			t.Fatalf("request host=%q want origin host %q", host, originHost)
		}
	}
	sawLoopbackFail := false
	sawOrigin := false
	for _, u := range callLog {
		if strings.HasPrefix(u, "http://127.0.0.1:65530/") {
			sawLoopbackFail = true
		}
		if strings.HasPrefix(u, server.URL+"/") {
			sawOrigin = true
		}
	}
	if !sawLoopbackFail || !sawOrigin {
		t.Fatalf("expected loopback refuse then Origin success; calls=%#v", callLog)
	}
}

// Issue #18 follow-up: concurrent newer ban re-disable must also Origin-fallback.
func TestDispatchUnbanConflictRedisableUsesOriginWhenDefaultPortUnreachable(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		mu            sync.Mutex
		callLog       []string
		authHeaders   []string
		disabledFlags []bool
		requestHosts  []string
	)
	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		disabledFlags = append(disabledFlags, body.Disabled)
		requestHosts = append(requestHosts, r.Host)
		mu.Unlock()
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
	installUnreachableDefaultPORTWithOriginDial(t, server.Client().Do, &callLog, &mu)

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "origin-conflict-1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	oldEntry, _ := activeStore.Get("origin-conflict-1")

	type result struct {
		resp pluginapi.ManagementResponse
	}
	ch := make(chan result, 1)
	go func() {
		ch <- result{resp: dispatchManagement(pluginapi.ManagementRequest{
			Method: http.MethodPost,
			Path:   "/v0/management/plugins/grok-inspection/unban",
			Body:   []byte(`{"auth_id":"origin-conflict-1"}`),
			Headers: http.Header{
				"Authorization": []string{"Bearer page-password"},
				"Cookie":        []string{"session=should-not-propagate"},
				"Origin":        []string{server.URL},
			},
		})}
	}()

	select {
	case <-enableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("enable not entered via /unban")
	}

	// Concurrent newer ban while enable is in-flight.
	activeStore.Set(banEntry{
		AuthID: "origin-conflict-1", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(15 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: true,
	})
	newEntry, _ := activeStore.Get("origin-conflict-1")
	if newEntry.Revision == oldEntry.Revision {
		t.Fatal("expected newer ban revision")
	}
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
	if payload["ok"] != false || payload["missing"] != false || payload["enabled"] != false || payload["removed"] != false {
		t.Fatalf("payload=%v", payload)
	}
	errStr, _ := payload["error"].(string)
	if !strings.Contains(errStr, "ban_conflict") && !strings.Contains(errStr, "concurrent ban") {
		t.Fatalf("error body=%q", errStr)
	}

	kept, ok := activeStore.Get("origin-conflict-1")
	if !ok {
		t.Fatal("newer ban must be retained")
	}
	if kept.Revision != newEntry.Revision {
		t.Fatalf("kept rev=%d want %d", kept.Revision, newEntry.Revision)
	}
	if !kept.CpaSynced || kept.CpaSyncError != "" {
		t.Fatalf("re-disable via Origin must mark CPA synced: synced=%v err=%q", kept.CpaSynced, kept.CpaSyncError)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(disabledFlags) < 2 {
		t.Fatalf("want enable+redisable CPA calls, got disabledFlags=%v calls=%#v", disabledFlags, callLog)
	}
	sawEnable := false
	sawRedisable := false
	for _, d := range disabledFlags {
		if !d {
			sawEnable = true
		} else {
			sawRedisable = true
		}
	}
	if !sawEnable || !sawRedisable {
		t.Fatalf("missing enable/redisable pair: %v", disabledFlags)
	}
	for _, auth := range authHeaders {
		if auth != "Bearer page-password" {
			t.Fatalf("authorization=%q want Bearer page-password", auth)
		}
	}
	originHost := strings.TrimPrefix(strings.TrimPrefix(server.URL, "https://"), "http://")
	for _, host := range requestHosts {
		if host != originHost {
			t.Fatalf("request host=%q want origin %q", host, originHost)
		}
	}
	sawLoopback := false
	sawOrigin := false
	for _, u := range callLog {
		if strings.HasPrefix(u, "http://127.0.0.1:65530/") {
			sawLoopback = true
		}
		if strings.HasPrefix(u, server.URL+"/") {
			sawOrigin = true
		}
	}
	if !sawLoopback || !sawOrigin {
		t.Fatalf("expected loopback refuse then Origin success; calls=%#v", callLog)
	}
}

// Batch job path: concurrent newer ban re-disable also uses Origin + password.
func TestStartUnbanJobConflictRedisableUsesOriginWhenDefaultPortUnreachable(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		mu            sync.Mutex
		callLog       []string
		authHeaders   []string
		disabledFlags []bool
	)
	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Disabled bool `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		disabledFlags = append(disabledFlags, body.Disabled)
		mu.Unlock()
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
	installUnreachableDefaultPORTWithOriginDial(t, server.Client().Do, &callLog, &mu)

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "origin-job-conflict", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	oldEntry, _ := activeStore.Get("origin-job-conflict")

	originHeaders := managementOriginOnlyHeaders(http.Header{"Origin": []string{server.URL}})
	if err := startUnbanJobWithOrigin([]string{"origin-job-conflict"}, "", "page-password", originHeaders); err != nil {
		t.Fatalf("startUnbanJobWithOrigin: %v", err)
	}

	select {
	case <-enableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("batch enable not entered")
	}
	activeStore.Set(banEntry{
		AuthID: "origin-job-conflict", Provider: "xai", ErrorCode: unauthorizedErrorCode,
		BannedAt: now, ResetAt: now.AddDate(10, 0, 0),
		ResetSource: "manual_unban", CpaSynced: true,
	})
	newEntry, _ := activeStore.Get("origin-job-conflict")
	if newEntry.Revision == oldEntry.Revision {
		t.Fatal("expected newer ban revision")
	}
	close(releaseEnable)

	done := make(chan struct{})
	go func() {
		unbanJob.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("unban job hung")
	}

	st := unbanJobStatus()
	if st["running"] != false {
		t.Fatalf("still running: %#v", st)
	}
	if st["failed"] != 1 || st["enabled"] != 0 || st["done"] != 1 {
		t.Fatalf("job status=%#v want failed=1 enabled=0 done=1", st)
	}
	failures, _ := st["failures"].([]string)
	joined := strings.Join(failures, " | ")
	if !strings.Contains(joined, "ban_conflict") && !strings.Contains(joined, "concurrent ban") {
		t.Fatalf("failures must surface conflict, got %q", joined)
	}

	kept, ok := activeStore.Get("origin-job-conflict")
	if !ok {
		t.Fatal("newer ban must be retained")
	}
	if kept.Revision != newEntry.Revision {
		t.Fatalf("kept rev=%d want %d", kept.Revision, newEntry.Revision)
	}
	if !kept.CpaSynced || kept.CpaSyncError != "" {
		t.Fatalf("re-disable via Origin must mark CPA synced: synced=%v err=%q", kept.CpaSynced, kept.CpaSyncError)
	}

	mu.Lock()
	defer mu.Unlock()
	sawEnable, sawRedisable := false, false
	for _, d := range disabledFlags {
		if !d {
			sawEnable = true
		} else {
			sawRedisable = true
		}
	}
	if !sawEnable || !sawRedisable {
		t.Fatalf("missing enable/redisable: %v calls=%#v", disabledFlags, callLog)
	}
	for _, auth := range authHeaders {
		if auth != "Bearer page-password" {
			t.Fatalf("authorization=%q", auth)
		}
	}
}
