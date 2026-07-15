package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestCallCPAManagementUsesBearerPasswordAndJSON(t *testing.T) {
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

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
	}()

	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
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
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword) }()
	_ = os.Setenv("MANAGEMENT_PASSWORD", "env-password")

	headers := http.Header{"Authorization": []string{"Bearer page-password"}}
	if got := resolveManagementPassword(headers); got != "page-password" {
		t.Fatalf("password = %q, want page-password", got)
	}
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

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
	}()
	cpaManagementBaseURL = server.URL
	cpaManagementDo = server.Client().Do
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

func TestResolveManagementBaseURLIgnoresRequestHostPort(t *testing.T) {
	oldBase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	oldPort := os.Getenv("PORT")
	oldCPAPort := os.Getenv("CPA_PORT")
	oldDefault := cpaManagementBaseURL
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		cpaManagementBaseURL = oldDefault
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	cpaManagementBaseURL = "http://127.0.0.1:8317"

	headers := http.Header{"Host": []string{"cpa.example.com:1109"}}
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:8317" {
		t.Fatalf("base url = %q, want default local management port", got)
	}

	_ = os.Setenv("CPA_BASE_URL", "http://127.0.0.1:9999")
	if got := resolveManagementBaseURL(headers); got != "http://127.0.0.1:9999" {
		t.Fatalf("env base url = %q", got)
	}
}

func TestStartRejectsInvalidWorkers(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers}
	err := e.start(startRequest{Workers: 99})
	if err == nil || !strings.Contains(err.Error(), "workers must") {
		t.Fatalf("err = %v", err)
	}
}

func TestIncrementalStartRequiresExistingResults(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers}
	err := e.start(startRequest{Workers: 2, Incremental: true})
	if err == nil || !strings.Contains(err.Error(), "增量巡检") {
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

	old := engine
	engine = &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{Name: "need-reauth", AuthIndex: "a1", FileName: "a1.json", Classification: "reauth", Action: "delete"},
		},
	}
	t.Cleanup(func() {
		engine.runWG.Wait()
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
	snap := engine.snapshot(false)
	if !snap.Applying {
		t.Fatal("expected applying=true")
	}
	if snap.IncludeResults {
		t.Fatal("light snapshot should set include_results=false")
	}
	if len(snap.Results) != 0 {
		t.Fatalf("light snapshot should omit results, got %d", len(snap.Results))
	}
	// status path is pure memory and must not wait on apply/delete work
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/management/plugins/grok-inspection/status",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), `"applying":true`) {
		t.Fatalf("status body missing applying=true: %s", string(resp.Body))
	}
	engine.runWG.Wait()
}


func TestClassifyScopedStartRequiresExistingResults(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers}
	err := e.start(startRequest{Workers: 2, Classifications: []string{"quota_exhausted"}})
	if err == nil || !strings.Contains(err.Error(), "分类巡检") {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyScopedRejectsWithIncremental(t *testing.T) {
	e := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{{AuthIndex: "a1", Classification: "quota_exhausted"}},
	}
	err := e.start(startRequest{Workers: 2, Incremental: true, Classifications: []string{"quota_exhausted"}})
	if err == nil || !strings.Contains(err.Error(), "分类巡检") {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyScopedRejectsEmptyMatch(t *testing.T) {
	e := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{{AuthIndex: "a1", Classification: "healthy"}},
	}
	err := e.start(startRequest{Workers: 2, Classifications: []string{"reauth"}})
	if err == nil || !strings.Contains(err.Error(), "当前分类") {
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
	oldDefault := cpaManagementBaseURL
	defer func() {
		_ = os.Setenv("CPA_BASE_URL", oldBase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
		_ = os.Setenv("PORT", oldPort)
		_ = os.Setenv("CPA_PORT", oldCPAPort)
		_ = os.Setenv("CPA_TLS", oldTLS)
		cpaManagementBaseURL = oldDefault
	}()
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Unsetenv("CPA_MANAGEMENT_BASE_URL")
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CPA_PORT")
	_ = os.Setenv("CPA_TLS", "true")
	cpaManagementBaseURL = "http://127.0.0.1:8317"

	if got := resolveManagementBaseURL(nil); got != "https://127.0.0.1:8317" {
		t.Fatalf("tls base url = %q", got)
	}
	_ = os.Setenv("PORT", "9443")
	if got := resolveManagementBaseURL(nil); got != "https://127.0.0.1:9443" {
		t.Fatalf("tls port base url = %q", got)
	}
}

func TestCallCPAManagementRetriesHTTPSAfterPlainHTTPMismatch(t *testing.T) {
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

	oldBaseURL := cpaManagementBaseURL
	oldDo := cpaManagementDo
	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	oldCPABase := os.Getenv("CPA_BASE_URL")
	oldMgmt := os.Getenv("CPA_MANAGEMENT_BASE_URL")
	defer func() {
		cpaManagementBaseURL = oldBaseURL
		cpaManagementDo = oldDo
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword)
		_ = os.Setenv("CPA_BASE_URL", oldCPABase)
		_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", oldMgmt)
	}()

	// Force resolve to plain http against the TLS listener.
	_ = os.Unsetenv("CPA_BASE_URL")
	_ = os.Setenv("CPA_MANAGEMENT_BASE_URL", httpBase)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "tls-pass")
	// Use real client that accepts the test cert via InsecureSkipVerify in plugin client.
	cpaManagementDo = cpaManagementClient.Do

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

