package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestEnableWithoutConcurrentBanReturnsNil(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })

	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "ok1", Name: "ok1.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = callHostAuthList })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "ok1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	engine.mu.Lock()
	old := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex: "ok1", Name: "ok1.json", FileName: "ok1.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = old
		engine.mu.Unlock()
	})

	if err := setAuthDisabled("ok1.json", false, "test-pass", nil, true); err != nil {
		t.Fatalf("normal enable: %v", err)
	}
	if _, ok := activeStore.Get("ok1.json"); ok {
		t.Fatal("ban should be cleared on normal enable")
	}
	engine.mu.Lock()
	row := engine.results[0]
	engine.mu.Unlock()
	if row.Disabled {
		t.Fatalf("row should be enabled: %+v", row)
	}
}

func TestEnableCASConflictFailsRowActionReport(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	enableEntered := make(chan struct{})
	releaseEnable := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "status") {
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
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "ra1", Name: "ra1.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = callHostAuthList })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "ra1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	engine.mu.Lock()
	old := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex: "ra1", Name: "ra1.json", FileName: "ra1.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = old
		engine.mu.Unlock()
	})

	seq, action, err := engine.startAction(actionRequest{Lang: "en", Name: "ra1.json", Disabled: false}, "test-pass", nil)
	if err != nil {
		t.Fatal(err)
	}
	if action != "enable" {
		t.Fatalf("action=%s", action)
	}
	select {
	case <-enableEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("enable not entered")
	}
	activeStore.Set(banEntry{
		AuthID: "ra1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now, ResetAt: now.Add(20 * time.Minute),
		ResetSource: "header_absolute", CpaSynced: false,
	})
	close(releaseEnable)

	deadline := time.Now().Add(5 * time.Second)
	var rep *rowActionReport
	for time.Now().Before(deadline) {
		engine.mu.Lock()
		for i := range engine.recentRowActions {
			if engine.recentRowActions[i].Seq == seq {
				r := engine.recentRowActions[i]
				rep = &r
				break
			}
		}
		engine.mu.Unlock()
		if rep != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rep == nil {
		t.Fatal("row action report missing")
	}
	if rep.OK {
		t.Fatalf("row action must not report OK on conflict: %+v", rep)
	}
	if rep.Error == "" || !strings.Contains(rep.Error, "ban_conflict") {
		t.Fatalf("row action error missing conflict: %+v", rep)
	}
	// Localized for UI
	zh := localizeKnownActionError(LangZH, rep.Error)
	if zh == rep.Error || strings.Contains(zh, "ban_conflict") {
		t.Fatalf("zh localize should map conflict, got %q", zh)
	}
}

func TestEnableCASConflictFailsBulkApply(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var enableCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "status") {
			var body struct {
				Disabled bool `json:"disabled"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body.Disabled {
				enableCount.Add(1)
				// Plant newer ban before enable returns so CAS sees it.
				now := time.Now()
				activeStore.Set(banEntry{
					AuthID: "bk1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
					BannedAt: now, ResetAt: now.Add(30 * time.Minute),
					ResetSource: "header_absolute", CpaSynced: false,
				})
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	installCPAManagementDialForTest(t, server.URL, server.Client().Do)
	_ = os.Setenv("MANAGEMENT_PASSWORD", "test-pass")
	t.Cleanup(func() { _ = os.Unsetenv("MANAGEMENT_PASSWORD") })
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: []pluginapi.HostAuthFileEntry{
			{AuthIndex: "bk1", Name: "bk1.json", Provider: "xai", Disabled: true},
		}}, nil
	}
	t.Cleanup(func() { callHostAuthListFn = callHostAuthList })

	now := time.Now()
	activeStore.Set(banEntry{
		AuthID: "bk1.json", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour),
		ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	engine.mu.Lock()
	old := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex: "bk1", Name: "bk1.json", FileName: "bk1.json",
		Disabled: true, Classification: "quota_exhausted", Action: "enable",
	}}
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.applyFailures = nil
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = old
		engine.mu.Unlock()
	})

	if err := engine.startApply(applyRequest{
		Lang: "en", ForceAction: "enable", AuthIndexes: []string{"bk1.json"},
	}, "test-pass", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := engine.snapshot(false)
		if !snap.Applying {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap := engine.snapshot(false)
	if snap.Applying {
		t.Fatal("apply still running")
	}
	if len(snap.ApplyFailures) == 0 {
		t.Fatal("bulk apply must record conflict as failure")
	}
	joined := strings.Join(snap.ApplyFailures, "; ")
	if !strings.Contains(joined, "ban_conflict") && !errors.Is(errors.New(joined), errBanSupersededByNewerRevision) {
		// failures are strings; check conflict marker
		if !strings.Contains(joined, "concurrent ban") {
			t.Fatalf("apply failures missing conflict: %v", snap.ApplyFailures)
		}
	}
	if enableCount.Load() < 1 {
		t.Fatal("expected enable PATCH")
	}
	if _, ok := activeStore.Get("bk1.json"); !ok {
		t.Fatal("newer ban must remain")
	}
	engine.mu.Lock()
	row := engine.results[0]
	engine.mu.Unlock()
	if !row.Disabled {
		t.Fatalf("row must stay disabled after conflict re-disable: %+v", row)
	}
}

func TestEnableCASConflictRedisableUsesRequestOriginAndPassword(t *testing.T) {
	isolateActiveStore(t)
	pauseBanDisposeWorkersForTest(t)
	rearmEngineAfterShutdownForTest()
	t.Cleanup(rearmEngineAfterShutdownForTest)

	var enableHits atomic.Int32
	var disableHits atomic.Int32
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer page-password" {
			t.Errorf("authorization=%q want page password", got)
		}
		var body struct {
			Disabled bool `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Disabled {
			disableHits.Add(1)
		} else {
			enableHits.Add(1)
			activeStore.Set(banEntry{
				AuthID:      "origin-enable-conflict.json",
				Provider:    "xai",
				ErrorCode:   unauthorizedErrorCode,
				BannedAt:    now,
				ResetAt:     now.AddDate(10, 0, 0),
				ResetSource: "manual_unban",
				CpaSynced:   false,
			})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	installUnreachableDefaultPORTWithOriginDial(t, server.Client().Do, nil, nil)

	oldPassword := os.Getenv("MANAGEMENT_PASSWORD")
	_ = os.Setenv("MANAGEMENT_PASSWORD", "env-password-must-not-win")
	t.Cleanup(func() { _ = os.Setenv("MANAGEMENT_PASSWORD", oldPassword) })

	activeStore.Set(banEntry{
		AuthID:      "origin-enable-conflict.json",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.Add(time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   true,
	})
	engine.mu.Lock()
	oldResults := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{{
		AuthIndex:      "origin-enable-conflict",
		Name:           "origin-enable-conflict.json",
		FileName:       "origin-enable-conflict.json",
		Disabled:       true,
		Classification: "quota_exhausted",
		Action:         "enable",
	}}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = oldResults
		engine.mu.Unlock()
	})

	headers := http.Header{
		"Authorization": []string{"Bearer page-password"},
		"Origin":        []string{server.URL},
	}
	err := setAuthDisabled("origin-enable-conflict.json", false, "page-password", headers, true)
	if !errors.Is(err, errBanSupersededByNewerRevision) {
		t.Fatalf("enable conflict error=%v want %v", err, errBanSupersededByNewerRevision)
	}
	if enableHits.Load() != 1 || disableHits.Load() != 1 {
		t.Fatalf("origin hits enable=%d disable=%d want 1/1", enableHits.Load(), disableHits.Load())
	}
	entry, ok := activeStore.Get("origin-enable-conflict.json")
	if !ok {
		t.Fatal("newer ban must remain")
	}
	if !entry.CpaSynced || entry.CpaSyncError != "" {
		t.Fatalf("re-disabled ban sync state=%+v", entry)
	}
}

func TestLocalizeBanConflictSuperseded(t *testing.T) {
	raw := errBanSupersededByNewerRevision.Error()
	zh := localizeKnownActionError(LangZH, raw)
	en := localizeKnownActionError(LangEN, raw)
	if zh == raw || strings.Contains(zh, "ban_conflict") {
		t.Fatalf("zh localize failed: %q", zh)
	}
	if en != T(LangEN, "ban_conflict_superseded") {
		t.Fatalf("en localize = %q want %q", en, T(LangEN, "ban_conflict_superseded"))
	}
	// Round-trip zh message
	if got := localizeKnownActionError(LangEN, zh); got != en {
		t.Fatalf("zh->en = %q want %q", got, en)
	}
	// Prefixed bulk form
	pref := "bk1.json: " + raw
	if got := localizeKnownActionError(LangZH, pref); !strings.HasPrefix(got, "bk1.json: ") || strings.Contains(got, "ban_conflict") {
		t.Fatalf("prefixed zh = %q", got)
	}
}
