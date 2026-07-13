package main

import (
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
