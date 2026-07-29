package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

// clearBanDeleteDialPollutionForTest clears request-derived management port
// cache left by Origin-routed /ban-delete tests so later tests that rely on
// setCPAManagementBaseURL are not redirected to a closed httptest port.
func clearBanDeleteDialPollutionForTest(t *testing.T) {
	t.Helper()
	clearDerivedManagementPortCacheForTest()
	t.Cleanup(clearDerivedManagementPortCacheForTest)
}

func waitUnbanJobIdle(t *testing.T, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st := unbanJobStatus()
		if st["running"] == false {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := unbanJobStatus()
	t.Fatalf("unban/delete job still running: %#v", st)
	return st
}

func seedBanAndResult(t *testing.T, authID, fileName, errorCode string) {
	t.Helper()
	now := time.Now()
	activeStore.Set(banEntry{
		AuthID:      authID,
		Provider:    "xai",
		ErrorCode:   errorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})
	engine.mu.Lock()
	engine.results = append(engine.results, accountResult{
		AuthIndex:      authID,
		Name:           fileName,
		FileName:       fileName,
		Classification: "quota_exhausted",
		Action:         "disable",
		Disabled:       true,
	})
	engine.mu.Unlock()
}

func isolateEngineResults(t *testing.T) {
	t.Helper()
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	oldRunning := engine.running
	oldApplying := engine.applying
	oldDraining := engine.applyDraining
	oldInFlight := engine.actionInFlight
	engine.results = nil
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.running = oldRunning
		engine.applying = oldApplying
		engine.applyDraining = oldDraining
		engine.actionInFlight = oldInFlight
		engine.mu.Unlock()
	})
}

// Category delete must only DELETE that category, use physical file names, and
// clear both inspection results and ban store for successes.
func TestBanDeleteCategoryOnlyDeletesMatchingAccounts(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		mu      sync.Mutex
		deleted []string
		bodies  []string
		methods []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		methods = append(methods, r.Method+" "+r.URL.Path)
		bodies = append(bodies, string(body))
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/v0/management/auth-files") {
			var payload struct {
				Names []string `json:"names"`
			}
			_ = json.Unmarshal(body, &payload)
			deleted = append(deleted, payload.Names...)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			mu.Unlock()
			return
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		_ = os.Setenv("MANAGEMENT_PASSWORD", oldPass)
	})

	seedBanAndResult(t, "quota-alias-1", "quota-file-1.json", exhaustedErrorCode)
	seedBanAndResult(t, "perm-alias-1", "perm-file-1.json", permissionDeniedErrorCode)

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    "/v0/management/plugins/grok-inspection/ban-delete",
		Body:    []byte(`{"category":"quota"}`),
		Headers: http.Header{"Authorization": []string{"Bearer test-pass"}},
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d body=%s want 202", resp.StatusCode, string(resp.Body))
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true || payload["accepted"] != true {
		t.Fatalf("payload=%v", payload)
	}
	unban, _ := payload["unban"].(map[string]any)
	if unban == nil {
		t.Fatalf("missing unban job status in response: %v", payload)
	}
	if mode, _ := unban["mode"].(string); mode != "delete" {
		t.Fatalf("mode=%v want delete", unban["mode"])
	}

	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["mode"] != "delete" {
		t.Fatalf("final mode=%v want delete", st["mode"])
	}
	if st["deleted"] != 1 || st["failed"] != 0 || st["done"] != 1 || st["total"] != 1 {
		t.Fatalf("status=%#v want deleted=1 failed=0 done=1 total=1", st)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(deleted) != 1 || deleted[0] != "quota-file-1.json" {
		t.Fatalf("deleted names=%v want [quota-file-1.json]", deleted)
	}
	for _, m := range methods {
		if strings.Contains(m, "auth-files/status") {
			t.Fatalf("delete job must not enable/unban via status patch; methods=%v", methods)
		}
	}
	if _, ok := activeStore.Get("quota-alias-1"); ok {
		t.Fatal("quota ban should be removed")
	}
	if _, ok := activeStore.Get("perm-alias-1"); !ok {
		t.Fatal("permission ban must remain")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if item, ok := lookupAccountResultByFileFirst(engine.results, "quota-file-1.json"); ok {
		t.Fatalf("quota inspection result still present: %#v", item)
	}
	if _, ok := lookupAccountResultByFileFirst(engine.results, "perm-file-1.json"); !ok {
		t.Fatal("permission inspection result must remain")
	}
}

// All delete must batch by deleteBatchSize and expose progress while running.
func TestBanDeleteAllBatchesAndReportsProgress(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	const n = 55 // > deleteBatchSize(50)
	var (
		mu         sync.Mutex
		batchSizes []int
		entered    = make(chan struct{})
		release    = make(chan struct{})
		batches    atomic.Int32
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Names []string `json:"names"`
		}
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		batchSizes = append(batchSizes, len(payload.Names))
		mu.Unlock()
		b := batches.Add(1)
		if b == 1 {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		select {
		case <-release:
		default:
			close(release)
		}
	})

	now := time.Now()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("all-%03d", i)
		file := id + ".json"
		activeStore.Set(banEntry{
			AuthID: id, Provider: "xai", ErrorCode: exhaustedErrorCode,
			BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
		})
		engine.mu.Lock()
		engine.results = append(engine.results, accountResult{
			AuthIndex: id, Name: file, FileName: file, Classification: "quota_exhausted", Action: "disable", Disabled: true,
		})
		engine.mu.Unlock()
	}

	if err := startBanDeleteJob(nil, "all", "test-pass"); err != nil {
		t.Fatalf("startBanDeleteJob: %v", err)
	}

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first delete batch never started")
	}
	st := unbanJobStatus()
	if st["running"] != true || st["mode"] != "delete" {
		close(release)
		t.Fatalf("mid-job status=%#v", st)
	}
	if st["total"] != n {
		close(release)
		t.Fatalf("total=%v want %d", st["total"], n)
	}
	// Progress should advance for the in-flight/completed first batch eventually.
	// At minimum total/mode/running are visible for UI polling.
	close(release)
	st = waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != n || st["done"] != n || st["failed"] != 0 {
		t.Fatalf("final status=%#v", st)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(batchSizes) < 2 {
		t.Fatalf("batchSizes=%v want at least 2 batches", batchSizes)
	}
	if batchSizes[0] != deleteBatchSize {
		t.Fatalf("first batch size=%d want %d", batchSizes[0], deleteBatchSize)
	}
	if batchSizes[1] != n-deleteBatchSize {
		t.Fatalf("second batch size=%d want %d", batchSizes[1], n-deleteBatchSize)
	}
	if activeStore.Count() != 0 {
		t.Fatalf("ban store still has %d entries", activeStore.Count())
	}
}

// 207 multi-status: successes clear, failures keep ban+result, counters correct.
func TestBanDeletePartialFailure207(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"status":"multi","failed":[{"name":"fail-me.json","error":"busy"}]}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	seedBanAndResult(t, "ok-id", "ok-me.json", exhaustedErrorCode)
	seedBanAndResult(t, "fail-id", "fail-me.json", exhaustedErrorCode)

	if err := startBanDeleteJob([]string{"ok-id", "fail-id"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 1 || st["failed"] != 1 || st["done"] != 2 {
		t.Fatalf("status=%#v want deleted=1 failed=1 done=2", st)
	}
	fails, _ := st["failures"].([]string)
	if len(fails) == 0 || !strings.Contains(strings.Join(fails, "\n"), "fail-me") {
		t.Fatalf("failures=%v", fails)
	}
	if _, ok := activeStore.Get("ok-id"); ok {
		t.Fatal("ok-id ban should be cleared")
	}
	if _, ok := activeStore.Get("fail-id"); !ok {
		t.Fatal("fail-id ban must remain")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if _, ok := lookupAccountResultByFileFirst(engine.results, "ok-me.json"); ok {
		t.Fatal("ok result should be cleared")
	}
	if _, ok := lookupAccountResultByFileFirst(engine.results, "fail-me.json"); !ok {
		t.Fatal("fail result must remain")
	}
}

// Stop must not start later batches; busy slot stays held until in-flight batch ends.
func TestBanDeleteStopDoesNotStartNextBatch(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		batches atomic.Int32
		entered = make(chan struct{})
		release = make(chan struct{})
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		b := batches.Add(1)
		if b == 1 {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		select {
		case <-release:
		default:
			close(release)
		}
	})

	now := time.Now()
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("stop-del-%02d", i)
		file := id + ".json"
		activeStore.Set(banEntry{
			AuthID: id, Provider: "xai", ErrorCode: exhaustedErrorCode,
			BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
		})
		engine.mu.Lock()
		engine.results = append(engine.results, accountResult{
			AuthIndex: id, Name: file, FileName: file,
		})
		engine.mu.Unlock()
	}

	if err := startBanDeleteJob(nil, "all", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first batch not entered")
	}
	stopUnbanJob()
	st := unbanJobStatus()
	if st["running"] != true {
		close(release)
		t.Fatalf("after stop, should still be draining busy: %#v", st)
	}
	if err := startBanDeleteJob(nil, "all", "test-pass"); err == nil || !strings.Contains(err.Error(), "busy") {
		close(release)
		t.Fatalf("expected busy while draining, err=%v", err)
	}
	if err := startUnbanJob(nil, "all", "test-pass"); err == nil || !strings.Contains(err.Error(), "busy") {
		close(release)
		t.Fatalf("unban must also be busy while delete drains, err=%v", err)
	}
	close(release)
	st = waitUnbanJobIdle(t, 5*time.Second)
	if batches.Load() != 1 {
		t.Fatalf("batches=%d want 1 (no second batch after stop)", batches.Load())
	}
	// In-flight first batch (50) must still count after stop; second batch never runs.
	if st["done"] != 50 || st["deleted"] != 50 || st["failed"] != 0 {
		t.Fatalf("after stop+drain status=%#v want done=50 deleted=50 failed=0", st)
	}
	if activeStore.Count() != 10 {
		t.Fatalf("remaining bans=%d want 10 (second batch not deleted)", activeStore.Count())
	}
}

func TestBanDeleteConflictsWithInspectionAndApply(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

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
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		select {
		case <-release:
		default:
			close(release)
		}
	})

	seedBanAndResult(t, "busy-del", "busy-del.json", exhaustedErrorCode)
	if err := startBanDeleteJob([]string{"busy-del"}, "", "test-pass"); err != nil {
		t.Fatalf("start delete: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("delete not entered")
	}
	if err := engine.start(startRequest{Workers: 1, Lang: "en"}); err == nil || !isBusyErr(err) {
		close(release)
		t.Fatalf("inspection should be busy during delete, err=%v", err)
	}
	if err := engine.startApply(applyRequest{ForceAction: "disable", AuthIndexes: []string{"busy-del"}}, "test-pass", nil); err == nil || !isBusyErr(err) {
		close(release)
		t.Fatalf("apply should be busy during delete, err=%v", err)
	}
	close(release)
	_ = waitUnbanJobIdle(t, 5*time.Second)
}

func TestBanDeleteAPIErrorParityWithUnbanAll(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	// invalid JSON
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/ban-delete",
		Body:   []byte(`{`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid json status=%d", resp.StatusCode)
	}
	var bad map[string]any
	_ = json.Unmarshal(resp.Body, &bad)
	if bad["ok"] != false || bad["error"] != "invalid JSON body" {
		t.Fatalf("invalid json body=%v", bad)
	}

	// no accounts
	resp = dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/ban-delete",
		Body:   []byte(`{"category":"quota"}`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no accounts status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	_ = json.Unmarshal(resp.Body, &bad)
	if bad["ok"] != false || !strings.Contains(fmt.Sprint(bad["error"]), "no accounts") {
		t.Fatalf("no accounts body=%v", bad)
	}

	// busy: hold a single unban claim with at least one deletable target present
	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "busy-target", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	runID, err := claimUnbanSlot(1, "hold", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { releaseUnbanSlot(runID) })
	resp = dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/ban-delete",
		Body:   []byte(`{"category":"all"}`),
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("busy status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
}

func TestBanDeleteUsesOriginRouteLikeUnban(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	clearBanDeleteDialPollutionForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		mu          sync.Mutex
		authHeaders []string
		callLog     []string
		originHits  int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		originHits++
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	installUnreachableDefaultPORTWithOriginDial(t, server.Client().Do, &callLog, &mu)

	seedBanAndResult(t, "origin-del-1", "origin-del-1.json", exhaustedErrorCode)
	headers := http.Header{
		"Authorization": []string{"Bearer page-password"},
		"Cookie":        []string{"session=should-not-propagate"},
		"Origin":        []string{server.URL},
	}
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    "/v0/management/plugins/grok-inspection/ban-delete",
		Body:    []byte(`{"auth_ids":["origin-del-1"]}`),
		Headers: headers,
	})
	headers.Set("Authorization", "Bearer mutated")
	headers.Set("Origin", "https://attacker.example")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 1 || st["failed"] != 0 {
		t.Fatalf("status=%#v", st)
	}
	mu.Lock()
	defer mu.Unlock()
	if originHits < 1 {
		t.Fatalf("origin not hit; calls=%v", callLog)
	}
	for _, a := range authHeaders {
		if a != "Bearer page-password" {
			t.Fatalf("auth=%q", a)
		}
	}
}

func TestBanDeleteUIContract(t *testing.T) {
	page := string(renderUIPage(pluginName))
	for _, marker := range []string{
		`id="banDeleteFilterBtn"`,
		`id="banDeleteAllBtn"`,
		`id="banStopBtn"`,
		`/ban-delete`,
		`ban_delete_filter`,
		`ban_delete_all`,
		`ban_stop`,
		`delete_filter_confirm`,
		`unban.mode`,
		`delete_running`,
		`delete_deleted_sep`,
		`deleteAllBanned._busy`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("missing UI marker %q", marker)
		}
	}
	if strings.Contains(page, "+ ' · del '") || strings.Contains(page, "+ \" · del \"") {
		t.Fatal("progress still hardcodes 'del' literal")
	}
	if strings.Contains(page, "t('delete_all_confirm_body')") {
		t.Fatal("unused delete_all_confirm_body still referenced")
	}
	zh := extractI18NPack(page, "zh")
	en := extractI18NPack(page, "en")
	for _, key := range []string{
		"ban_delete_filter", "ban_delete_all", "ban_stop",
		"delete_filter_confirm_title", "delete_filter_confirm_body_prefix",
		"delete_filter_confirm_body_suffix",
		"delete_all_confirm_title", "delete_all_confirm_body_prefix", "delete_all_confirm_body_suffix",
		"delete_running", "delete_progress_complete_fail", "delete_deleted_sep",
		"delete_in_progress", "delete_start_failed",
		"delete_filter_empty", "delete_filter_started_prefix",
		"ban_filter_hint_all",
	} {
		if strings.TrimSpace(zh[key]) == "" {
			t.Fatalf("zh %s missing", key)
		}
		if strings.TrimSpace(en[key]) == "" {
			t.Fatalf("en %s missing", key)
		}
	}
	if _, ok := zh["delete_all_confirm_body"]; ok {
		t.Fatal("unused zh delete_all_confirm_body key should be removed")
	}
	if _, ok := en["delete_all_confirm_body"]; ok {
		t.Fatal("unused en delete_all_confirm_body key should be removed")
	}
	zhSuffix := zh["delete_filter_confirm_body_suffix"] + zh["delete_all_confirm_body_suffix"]
	enSuffix := en["delete_filter_confirm_body_suffix"] + en["delete_all_confirm_body_suffix"]
	if strings.Contains(strings.ToLower(zhSuffix), "irreversible") {
		t.Fatalf("zh confirm must not embed irreversible: %q", zhSuffix)
	}
	if !strings.Contains(zhSuffix, "不可恢复") {
		t.Fatalf("zh confirm must say 不可恢复: %q", zhSuffix)
	}
	if strings.Contains(enSuffix, "不可恢复") {
		t.Fatalf("en confirm must not embed 不可恢复: %q", enSuffix)
	}
	if !strings.Contains(strings.ToLower(enSuffix), "irreversible") {
		t.Fatalf("en confirm must say irreversible: %q", enSuffix)
	}
	if !strings.Contains(zh["ban_filter_hint_all"], "删除") || !strings.Contains(zh["ban_filter_hint_all"], "解禁") {
		t.Fatalf("zh ban_filter_hint_all should cover unban and delete: %q", zh["ban_filter_hint_all"])
	}
	if !strings.Contains(strings.ToLower(en["ban_filter_hint_all"]), "delete") || !strings.Contains(strings.ToLower(en["ban_filter_hint_all"]), "unban") {
		t.Fatalf("en ban_filter_hint_all should cover unban and delete: %q", en["ban_filter_hint_all"])
	}
}

func TestUnbanStatusStillCompatibleWithoutMode(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	// Idle status must keep legacy keys.
	st := unbanJobStatus()
	for _, key := range []string{"running", "stopped", "done", "total", "enabled", "missing", "failed", "current", "failures", "persist_error"} {
		if _, ok := st[key]; !ok {
			t.Fatalf("legacy key %s missing from %#v", key, st)
		}
	}
	// mode may be present; deleted should be numeric-compatible (0).
	if st["deleted"] != 0 && st["deleted"] != nil {
		// allow 0 only when idle after reset
		if v, ok := st["deleted"].(int); ok && v != 0 {
			t.Fatalf("idle deleted=%v", st["deleted"])
		}
	}
}

// Route must return 202 without waiting on a blocked host auth list; worker
// keeps the shared busy slot until lookup/delete finish.
func TestBanDeleteReturns202WhileHostListBlocked(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)
	clearBanDeleteDialPollutionForTest(t)

	entered := make(chan struct{})
	release := make(chan struct{})
	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{{
			Name: "blocked-host.json", AuthIndex: "blocked-host", ID: "blocked-host.json",
		}}}, nil
	}
	t.Cleanup(func() {
		callHostAuthListFn = oldList
		select {
		case <-release:
		default:
			close(release)
		}
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	// Ban id is not present in inspection results -> worker must host-list.
	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "blocked-host", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})

	started := time.Now()
	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    "/v0/management/plugins/grok-inspection/ban-delete",
		Body:    []byte(`{"auth_ids":["blocked-host"]}`),
		Headers: http.Header{"Authorization": []string{"Bearer test-pass"}},
	})
	if time.Since(started) > 500*time.Millisecond {
		close(release)
		t.Fatalf("ban-delete handler blocked on host list for %v", time.Since(started))
	}
	if resp.StatusCode != http.StatusAccepted {
		close(release)
		t.Fatalf("status=%d body=%s want 202", resp.StatusCode, string(resp.Body))
	}
	st := unbanJobStatus()
	if st["running"] != true || st["mode"] != "delete" {
		close(release)
		t.Fatalf("slot should stay running during blocked lookup: %#v", st)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("host list not entered")
	}
	// Still busy before release.
	if err := startBanDeleteJob([]string{"blocked-host"}, "", "test-pass"); err == nil || !strings.Contains(err.Error(), "busy") {
		close(release)
		t.Fatalf("expected busy during lookup, err=%v", err)
	}
	close(release)
	st = waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 1 || st["failed"] != 0 {
		t.Fatalf("final status=%#v", st)
	}
	if _, ok := activeStore.Get("blocked-host"); ok {
		t.Fatal("ban should be cleared after successful delete")
	}
}

func TestBanDeleteDuplicateAliasesSharePhysicalFile207Failure(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"status":"multi","failed":[{"name":"shared.json","error":"locked"}]}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	now := time.Now()
	for _, id := range []string{"alias-a", "alias-b"} {
		activeStore.Set(banEntry{
			AuthID: id, Provider: "xai", ErrorCode: exhaustedErrorCode,
			BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
		})
	}
	// Both aliases resolve to the same physical file via inspection results.
	engine.mu.Lock()
	engine.results = []accountResult{
		{AuthIndex: "alias-a", Name: "shared.json", FileName: "shared.json", Disabled: true},
		{AuthIndex: "alias-b", Name: "shared.json", FileName: "shared.json", Disabled: true},
	}
	engine.mu.Unlock()

	if err := startBanDeleteJob([]string{"alias-a", "alias-b"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 0 || st["failed"] != 2 || st["done"] != 2 {
		t.Fatalf("status=%#v want deleted=0 failed=2 done=2", st)
	}
	if _, ok := activeStore.Get("alias-a"); !ok {
		t.Fatal("alias-a ban must remain")
	}
	if _, ok := activeStore.Get("alias-b"); !ok {
		t.Fatal("alias-b ban must remain")
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if len(engine.results) != 2 {
		t.Fatalf("results len=%d want 2 retained", len(engine.results))
	}
}

func TestBanDeleteReportsResultsPersistError(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	// Pre-existing persistError must not be blamed on this job if results save ok.
	engine.mu.Lock()
	engine.persistError = "stale-old-persist-error"
	engine.persistStatusSeq = 1
	engine.mu.Unlock()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	// Force results.json save to fail: path under a regular file.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked-file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	setStoreFilePathForTest(filepath.Join(blocker, "results.json"))
	t.Cleanup(func() { setStoreFilePathForTest("") })

	seedBanAndResult(t, "persist-del", "persist-del.json", exhaustedErrorCode)
	if err := startBanDeleteJob([]string{"persist-del"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	pe, _ := st["persist_error"].(string)
	if strings.TrimSpace(pe) == "" {
		t.Fatalf("expected persist_error, status=%#v", st)
	}
	if strings.Contains(pe, "stale-old-persist-error") {
		t.Fatalf("must not reuse stale engine persistError: %q", pe)
	}
	if st["failed"] == 0 {
		t.Fatalf("persist failure must surface as failed, status=%#v", st)
	}
}

func TestBanDeleteHostListFailureKeepsLocalState(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var listCalls atomic.Int32
	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		listCalls.Add(1)
		return authListResponse{}, fmt.Errorf("host list unavailable")
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	var deleteCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalls.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "only-in-ban", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	// No inspection results -> requires host list.
	if err := startBanDeleteJob([]string{"only-in-ban"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if listCalls.Load() != 1 {
		t.Fatalf("host list calls=%d want 1", listCalls.Load())
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("DELETE calls=%d want 0 when lookup failed", deleteCalls.Load())
	}
	if st["deleted"] != 0 || st["failed"] != 1 {
		t.Fatalf("status=%#v want deleted=0 failed=1", st)
	}
	if _, ok := activeStore.Get("only-in-ban"); !ok {
		t.Fatal("ban must remain when host list failed")
	}
}

func TestBanDeleteItemFailureMessagesUnmappedFailClosed(t *testing.T) {
	chunk := []accountResult{
		{AuthIndex: "a", Name: "a.json", FileName: "a.json"},
		{AuthIndex: "b", Name: "b.json", FileName: "b.json"},
	}
	// CPA failure names a file that is not in the chunk -> fail-closed both rows.
	msgs := banDeleteItemFailureMessages(chunk, []string{"mystery-file.json: remote boom"})
	if len(msgs) != 2 || msgs[0] == "" || msgs[1] == "" {
		t.Fatalf("msgs=%v want both failed", msgs)
	}
	for _, m := range msgs {
		if !strings.Contains(m, "mystery-file.json") && !strings.Contains(m, "remote boom") {
			t.Fatalf("failure should carry unmapped detail: %q", m)
		}
	}
}

func TestBanDeleteItemFailureMessagesEmptyNameNeverSuccess(t *testing.T) {
	chunk := []accountResult{
		{}, // completely empty aliases / file name
		{AuthIndex: "ok-row", Name: "ok.json", FileName: "ok.json"},
	}
	// No batch fails: empty-name row still cannot be success.
	msgs := banDeleteItemFailureMessages(chunk, nil)
	if msgs[0] == "" {
		t.Fatal("empty-name row must not count as deleted success")
	}
	if msgs[1] != "" {
		t.Fatalf("named row with no failures should succeed, got %q", msgs[1])
	}
	// Unmapped failure + empty name: both fail (fail-closed).
	msgs = banDeleteItemFailureMessages(chunk, []string{": auth file name missing", "zzz: other"})
	if msgs[0] == "" || msgs[1] == "" {
		t.Fatalf("fail-closed msgs=%v", msgs)
	}
}

func TestBanDeleteItemFailureMessagesMapped207StillPrecise(t *testing.T) {
	chunk := []accountResult{
		{AuthIndex: "ok", Name: "ok.json", FileName: "ok.json"},
		{AuthIndex: "bad", Name: "bad.json", FileName: "bad.json"},
	}
	msgs := banDeleteItemFailureMessages(chunk, []string{"bad.json: locked"})
	if msgs[0] != "" {
		t.Fatalf("ok row should succeed, got %q", msgs[0])
	}
	if msgs[1] == "" || !strings.Contains(msgs[1], "bad.json") {
		t.Fatalf("bad row msgs=%q", msgs[1])
	}
}

func TestBanDeleteUnmappedMultiStatusFailClosedIntegration(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		// Failure references a name that does not match either account file.
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"status":"multi","failed":[{"name":"not-in-chunk.json","error":"ghost failure"}]}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	seedBanAndResult(t, "u1", "u1.json", exhaustedErrorCode)
	seedBanAndResult(t, "u2", "u2.json", exhaustedErrorCode)
	if err := startBanDeleteJob([]string{"u1", "u2"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 0 || st["failed"] != 2 || st["done"] != 2 {
		t.Fatalf("status=%#v want deleted=0 failed=2 done=2 (fail-closed)", st)
	}
	if _, ok := activeStore.Get("u1"); !ok {
		t.Fatal("u1 ban must remain under unmapped failure fail-closed accounting")
	}
	if _, ok := activeStore.Get("u2"); !ok {
		t.Fatal("u2 ban must remain under unmapped failure fail-closed accounting")
	}
}

func TestPersistSyncReturnsOwnErrorDespiteNewerSuccess(t *testing.T) {
	engine.mu.Lock()
	engine.persistError = ""
	engine.persistStatusSeq = 0
	engine.persistSeq = 0
	engine.mu.Unlock()
	t.Cleanup(func() {
		persistSaveHook = nil
		engine.mu.Lock()
		engine.persistError = ""
		engine.mu.Unlock()
	})

	var phase atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	persistSaveHook = func(seq uint64, save func() error) error {
		n := phase.Add(1)
		if n == 1 {
			// First persist (job-like): block until a newer success is recorded.
			close(started)
			<-release
			return fmt.Errorf("job-persist-boom")
		}
		// Newer concurrent persist succeeds and would clear engine.persistError.
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.persistSync()
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first persist did not enter hook")
	}
	// Concurrent newer persist succeeds and clears shared persistError.
	if err := engine.persistSync(); err != nil {
		t.Fatalf("newer persist: %v", err)
	}
	engine.mu.Lock()
	shared := engine.persistError
	engine.mu.Unlock()
	if shared != "" {
		t.Fatalf("shared persistError after newer success = %q want empty", shared)
	}
	close(release)
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "job-persist-boom") {
		t.Fatalf("persistSync must return this call's error, got %v", err)
	}
}

func TestBanDeletePersistUsesSyncReturnNotEngineField(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
		persistSaveHook = nil
	})

	var saves atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	persistSaveHook = func(seq uint64, save func() error) error {
		n := saves.Add(1)
		if n == 1 {
			close(started)
			<-release
			return fmt.Errorf("delete-job-persist-fail")
		}
		return nil
	}

	seedBanAndResult(t, "p-sync", "p-sync.json", exhaustedErrorCode)
	if err := startBanDeleteJob([]string{"p-sync"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("job persist not entered")
	}
	// Pollute shared field with a successful newer persist while job save is in flight.
	if err := engine.persistSync(); err != nil {
		close(release)
		t.Fatalf("concurrent persist: %v", err)
	}
	engine.mu.Lock()
	if engine.persistError != "" {
		engine.mu.Unlock()
		close(release)
		t.Fatalf("shared persistError=%q after concurrent success", engine.persistError)
	}
	engine.mu.Unlock()
	close(release)
	st := waitUnbanJobIdle(t, 5*time.Second)
	pe, _ := st["persist_error"].(string)
	if !strings.Contains(pe, "delete-job-persist-fail") {
		t.Fatalf("job must surface its own persist error, status=%#v", st)
	}
	if st["failed"] == 0 {
		t.Fatalf("persist failure must bump failed, status=%#v", st)
	}
}

func TestBanDeleteEmptyBodyRejected(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var deletes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "keep-me", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})

	resp := dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/ban-delete",
		Body:   []byte(`{}`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	if deletes.Load() != 0 {
		t.Fatalf("DELETE calls=%d want 0", deletes.Load())
	}
	if _, ok := activeStore.Get("keep-me"); !ok {
		t.Fatal("account must remain")
	}

	resp = dispatchManagement(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   "/v0/management/plugins/grok-inspection/ban-delete",
		Body:   []byte(`{"category":"not-a-real-cat"}`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown category status=%d body=%s", resp.StatusCode, string(resp.Body))
	}
	if deletes.Load() != 0 {
		t.Fatal("DELETE must not run for unknown category")
	}
}

func TestBanDeleteExplicitCategoryAllStillWorks(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})
	seedBanAndResult(t, "all-1", "all-1.json", exhaustedErrorCode)
	if err := startBanDeleteJob(nil, "all", "test-pass"); err != nil {
		t.Fatal(err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 1 {
		t.Fatalf("status=%#v", st)
	}
}

func TestResolveBanDeleteFromResultsCollisionIsAmbiguous(t *testing.T) {
	results := []accountResult{
		{AuthIndex: "shared-token", Name: "acct-a", FileName: "a.json"},
		{AuthIndex: "b", Name: "acct-b", FileName: "shared-token"},
	}
	_, ambig, ok := resolveBanDeleteFromResults(results, "shared-token")
	if ok || !ambig {
		t.Fatalf("ok=%v ambig=%v want ambiguous refusal", ok, ambig)
	}
	// Unique AuthIndex still works.
	item, ambig, ok := resolveBanDeleteFromResults(results, "b")
	if !ok || ambig || item.FileName != "shared-token" {
		t.Fatalf("unique b => %#v ambig=%v ok=%v", item, ambig, ok)
	}
	// Unique physical file name works (manual-sync style AuthID).
	item, ambig, ok = resolveBanDeleteFromResults(results, "a.json")
	if !ok || ambig || item.AuthIndex != "shared-token" {
		t.Fatalf("unique a.json => %#v ambig=%v ok=%v", item, ambig, ok)
	}
}

func TestBanDeleteAmbiguousAuthIndexFileNameCollisionSkips(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var (
		mu      sync.Mutex
		deleted []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			deleted = append(deleted, string(body))
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	// Ban pool only has AuthID=shared-token (usage-style). Results contain a collision:
	// A.AuthIndex=shared-token / B.FileName=shared-token.
	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "shared-token", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	engine.mu.Lock()
	engine.results = []accountResult{
		{AuthIndex: "shared-token", Name: "acct-a", FileName: "a.json", Disabled: true},
		{AuthIndex: "b", Name: "acct-b", FileName: "shared-token", Disabled: false},
	}
	engine.mu.Unlock()

	if err := startBanDeleteJob([]string{"shared-token"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 0 || st["failed"] != 1 {
		t.Fatalf("status=%#v want deleted=0 failed=1", st)
	}
	mu.Lock()
	nDel := len(deleted)
	bodies := append([]string(nil), deleted...)
	mu.Unlock()
	if nDel != 0 {
		t.Fatalf("CPA DELETE must not run on ambiguous identity; bodies=%v", bodies)
	}
	if _, ok := activeStore.Get("shared-token"); !ok {
		t.Fatal("ambiguous ban must be retained")
	}
	// Neither inspection row should be cleared.
	engine.mu.Lock()
	n := len(engine.results)
	engine.mu.Unlock()
	if n != 2 {
		t.Fatalf("results cleared on ambiguous skip: n=%d", n)
	}
}

func TestBanDeleteHostListCollisionSkips(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var deletes atomic.Int32
	oldList := callHostAuthListFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "shared-token", Name: "a.json", ID: "a.json"},
			{AuthIndex: "b", Name: "shared-token", ID: "shared-token"},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = oldList })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "shared-token", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	// No inspection results -> host list path.
	if err := startBanDeleteJob([]string{"shared-token"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 0 || st["failed"] != 1 {
		t.Fatalf("status=%#v want failed skip", st)
	}
	if deletes.Load() != 0 {
		t.Fatalf("DELETE calls=%d want 0", deletes.Load())
	}
	if _, ok := activeStore.Get("shared-token"); !ok {
		t.Fatal("ban must remain")
	}
}

func TestBanDeleteUniquePhysicalFileNameStillDeletes(t *testing.T) {
	isolateActiveStore(t)
	isolateUnbanJob(t)
	isolateEngineResults(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var gotNames []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			gotNames = append(gotNames, string(body))
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	oldBase := getCPAManagementBaseURL()
	oldDo := getCPAManagementDo()
	setCPAManagementBaseURL(server.URL)
	setCPAManagementDo(server.Client().Do)
	t.Cleanup(func() {
		setCPAManagementBaseURL(oldBase)
		setCPAManagementDo(oldDo)
	})

	// Manual-sync style: ban AuthID is the physical file name, unique in results.
	seedBanAndResult(t, "only-file.json", "only-file.json", exhaustedErrorCode)
	// Ensure AuthIndex differs so this is a file-name identity.
	engine.mu.Lock()
	if len(engine.results) == 1 {
		engine.results[0].AuthIndex = "auth-only"
		engine.results[0].FileName = "only-file.json"
		engine.results[0].Name = "only-file.json"
	}
	engine.mu.Unlock()
	activeStore.Clear()
	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "only-file.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(time.Hour), ResetSource: "local_plus_fallback", CpaSynced: true,
	})

	if err := startBanDeleteJob([]string{"only-file.json"}, "", "test-pass"); err != nil {
		t.Fatalf("start: %v", err)
	}
	st := waitUnbanJobIdle(t, 5*time.Second)
	if st["deleted"] != 1 || st["failed"] != 0 {
		t.Fatalf("status=%#v", st)
	}
	mu.Lock()
	joined := strings.Join(gotNames, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "only-file.json") {
		t.Fatalf("DELETE body missing file name: %s", joined)
	}
}
