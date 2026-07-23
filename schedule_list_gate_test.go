package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"grok-inspection/cpasdk/pluginapi"
)

func TestScheduledInspectionSkipsAutoActionsWhenAuthListFails(t *testing.T) {
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPass) })

	var mgmtHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mgmtHits.Add(1)
		t.Errorf("unexpected management mutation during list-failure schedule: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)

	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{}, errors.New("simulated auth list failure")
	}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		t.Errorf("probe must not run when list fails: %s", file.AuthIndex)
		return accountResult{AuthIndex: file.AuthIndex}
	}
	t.Cleanup(func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	})

	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	oldSchedule := engine.schedule
	engine.results = []accountResult{
		{
			AuthIndex: "stale-403", Name: "stale-403.json", FileName: "stale-403.json",
			HTTPStatus: 403, Classification: "permission_denied", ErrorCode: permissionDeniedErrorCode,
			Disabled: false, Action: "disable",
		},
		{
			AuthIndex: "stale-402", Name: "stale-402.json", FileName: "stale-402.json",
			HTTPStatus: 402, Classification: "spending_limit", ErrorCode: spendingLimitErrorCode,
			Disabled: false, Action: "disable",
		},
	}
	engine.schedule = persistedInspectionSchedule{
		Enabled:                true,
		IntervalMinutes:        60,
		Workers:                1,
		PermissionDeniedAction: scheduled403Disable,
		SpendingLimitAction:    scheduled402Delete,
	}
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.stopped = false
	engine.shuttingDown = false
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.schedule = oldSchedule
		engine.mu.Unlock()
	})

	cfg := inspectionScheduleSnapshot()
	runScheduledInspection(cfg)

	if mgmtHits.Load() != 0 {
		t.Fatalf("management mutations=%d want 0", mgmtHits.Load())
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()
	var found403, found402, foundListErr bool
	for _, item := range engine.results {
		switch item.AuthIndex {
		case "stale-403":
			found403 = true
			if item.Disabled {
				t.Fatalf("stale-403 was disabled on list failure: %+v", item)
			}
		case "stale-402":
			found402 = true
			if item.Disabled {
				t.Fatalf("stale-402 mutated on list failure: %+v", item)
			}
		case "__system_list_error__":
			foundListErr = true
		}
	}
	if !found403 || !found402 {
		t.Fatalf("stale results must be preserved for UI: 403=%v 402=%v results=%+v", found403, found402, engine.results)
	}
	if !foundListErr {
		t.Fatal("expected __system_list_error__ row after list failure")
	}

	sch := engine.schedule
	if sch.LastStatus != "failed" {
		t.Fatalf("LastStatus=%q want failed", sch.LastStatus)
	}
	if sch.LastMatched != 0 || sch.LastDisabled != 0 || sch.LastDeleted != 0 || sch.LastFailed != 0 {
		t.Fatalf("stats must be zero on list failure: matched=%d disabled=%d deleted=%d failed=%d",
			sch.LastMatched, sch.LastDisabled, sch.LastDeleted, sch.LastFailed)
	}
	if sch.LastMatched403+sch.LastMatched402 != 0 || sch.LastDisabled403+sch.LastDeleted402 != 0 {
		t.Fatalf("per-code stats must be zero: %+v", sch)
	}
	if strings.TrimSpace(sch.LastError) == "" {
		t.Fatal("LastError must carry list failure reason")
	}
}

func TestScheduledInspectionAutoActionsAfterSuccessfulList(t *testing.T) {
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	oldPass := os.Getenv("MANAGEMENT_PASSWORD")
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPass) })

	var mgmtHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			mgmtHits.Add(1)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)

	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "fresh-403", Name: "fresh-403.json", Provider: "xai", Disabled: false},
			{AuthIndex: "fresh-ok", Name: "fresh-ok.json", Provider: "xai", Disabled: false},
		}}, nil
	}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		if file.AuthIndex == "fresh-403" {
			return accountResult{
				AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name,
				HTTPStatus: 403, Classification: "permission_denied", ErrorCode: permissionDeniedErrorCode,
				Disabled: false, Action: "disable",
			}
		}
		return accountResult{
			AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name,
			HTTPStatus: 200, Classification: "ok", Action: "keep", Disabled: false,
		}
	}
	t.Cleanup(func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
	})

	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	oldSchedule := engine.schedule
	engine.results = []accountResult{{
		AuthIndex: "stale-402-should-clear", Name: "stale.json", FileName: "stale.json",
		HTTPStatus: 402, Classification: "spending_limit", ErrorCode: spendingLimitErrorCode,
		Disabled: false, Action: "disable",
	}}
	engine.schedule = persistedInspectionSchedule{
		Enabled:                true,
		IntervalMinutes:        60,
		Workers:                2,
		PermissionDeniedAction: scheduled403Disable,
		SpendingLimitAction:    scheduled402Disable,
	}
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.stopped = false
	engine.shuttingDown = false
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.schedule = oldSchedule
		engine.mu.Unlock()
	})

	cfg := inspectionScheduleSnapshot()
	runScheduledInspection(cfg)

	if mgmtHits.Load() < 1 {
		t.Fatalf("expected management disable after successful list+403, hits=%d", mgmtHits.Load())
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()
	for _, item := range engine.results {
		if item.AuthIndex == "stale-402-should-clear" {
			t.Fatal("full list success must clear prior results before auto-actions")
		}
	}
	sch := engine.schedule
	if sch.LastStatus != "completed" && sch.LastStatus != "completed_with_errors" {
		t.Fatalf("LastStatus=%q want completed*", sch.LastStatus)
	}
	if sch.LastMatched403 < 1 || sch.LastDisabled403 < 1 {
		t.Fatalf("expected 403 match/disable stats, schedule=%+v", sch)
	}
	if sch.LastMatched402 != 0 {
		t.Fatalf("stale 402 must not match after successful full list: matched402=%d", sch.LastMatched402)
	}
}

func TestFinishedRunOutcomeIsTokenizedByRunID(t *testing.T) {
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	engine.mu.Lock()
	engine.lastFinishedRunID = 42
	engine.lastFinishedListOK = true
	engine.lastFinishedListError = ""
	engine.lastFinishedFullInspect = true
	engine.mu.Unlock()

	ok, errMsg, full, found := engine.finishedRunOutcome(42)
	if !found || !ok || !full || errMsg != "" {
		t.Fatalf("expected match for 42: ok=%v err=%q full=%v found=%v", ok, errMsg, full, found)
	}
	ok, _, _, found = engine.finishedRunOutcome(99)
	if found || ok {
		t.Fatal("mismatched runID must not report found/listOK")
	}
}
