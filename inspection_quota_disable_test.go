package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetEngineActionStateForQuotaTest(t *testing.T) {
	t.Helper()
	isolateActiveStore(t)
	engine.mu.Lock()
	engine.running = false
	engine.applying = false
	engine.applyDraining = false
	engine.actionInFlight = 0
	engine.shuttingDown = false
	engine.mu.Unlock()
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.stopped = false
	unbanJob.persistError = ""
	unbanJob.mu.Unlock()
}

func withQuotaTestResultsPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	useResultsStorePath(t, filepath.Join(dir, "results.json"))
	t.Cleanup(func() { engine.waitAsyncPersist() })
	return dir
}

func withQuotaBanConfig(t *testing.T, dir string, hours int) {
	t.Helper()
	oldCfg := loadedConfig()
	cfg := oldCfg
	cfg.PersistState = true
	cfg.StateFile = filepath.Join(dir, "bans.json")
	if hours > 0 {
		cfg.FallbackHours = hours
	}
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(oldCfg) })
}

func waitRowActionSeq(t *testing.T, seq uint64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		snap := engine.snapshot(false)
		if snap.ActionInFlight == 0 {
			for _, r := range snap.RecentRowActions {
				if r.Seq == seq {
					if !r.OK {
						t.Fatalf("row action failed: %+v", r)
					}
					return
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting row action")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitApplyIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		snap := engine.snapshot(false)
		if !snap.Applying && snap.ActionInFlight == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("apply timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type cpaStatusPatch struct {
	Name     string
	Disabled bool
}

func withCPAStatusRecorder(t *testing.T) *[]cpaStatusPatch {
	t.Helper()
	patches := &[]cpaStatusPatch{}
	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode patch: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		*patches = append(*patches, cpaStatusPatch{Name: body.Name, Disabled: body.Disabled})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return patches
}

func TestInspectionDisableBanErrorCodeExactQuotaOnly(t *testing.T) {
	quota := accountResult{
		Classification: "quota_exhausted",
		ErrorCode:      exhaustedErrorCode,
		Action:         "disable",
	}
	if got := inspectionDisableBanErrorCode(quota); got != exhaustedErrorCode {
		t.Fatalf("quota exact free-usage => %q, want %q", got, exhaustedErrorCode)
	}

	partial := accountResult{
		Classification: "quota_exhausted",
		ErrorCode:      "prefix-" + exhaustedErrorCode,
		Action:         "disable",
	}
	if got := inspectionDisableBanErrorCode(partial); got != manualInspectionBanErrorCode {
		t.Fatalf("partial code => %q, want manual", got)
	}

	noCode := accountResult{Classification: "quota_exhausted", Action: "disable"}
	if got := inspectionDisableBanErrorCode(noCode); got != manualInspectionBanErrorCode {
		t.Fatalf("missing error code => %q, want manual", got)
	}

	for _, item := range []accountResult{
		{Classification: "healthy", ErrorCode: exhaustedErrorCode},
		{Classification: "permission_denied", ErrorCode: permissionDeniedErrorCode},
		{Classification: "spending_limit", ErrorCode: spendingLimitErrorCode},
		{Classification: "reauth", ErrorCode: unauthorizedErrorCode, HTTPStatus: 401},
	} {
		if got := inspectionDisableBanErrorCode(item); got != manualInspectionBanErrorCode {
			t.Fatalf("%+v => %q, want manual", item, got)
		}
	}
}

func TestCollectCandidatesSuggestedDisableUsesQuotaBanReason(t *testing.T) {
	e := &inspectionEngine{results: []accountResult{
		{
			AuthIndex: "q1", Name: "q1.json", FileName: "q1.json",
			Classification: "quota_exhausted", Action: "disable",
			ErrorCode: exhaustedErrorCode, HTTPStatus: 429,
		},
		{
			AuthIndex: "h1", Name: "h1.json", FileName: "h1.json",
			Classification: "healthy", Action: "disable",
		},
	}}
	cands, err := e.collectCandidates(applyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates=%d want 2", len(cands))
	}
	by := map[string]accountResult{}
	for _, c := range cands {
		by[c.AuthIndex] = c
	}
	if by["q1"].BanErrorCode != exhaustedErrorCode {
		t.Fatalf("quota suggested ban code=%q", by["q1"].BanErrorCode)
	}
	if by["h1"].BanErrorCode != manualInspectionBanErrorCode {
		t.Fatalf("healthy suggested ban code=%q", by["h1"].BanErrorCode)
	}
}

func TestCollectCandidatesForceDisableKeepsExplicit402403Reason(t *testing.T) {
	e := &inspectionEngine{results: []accountResult{
		{
			AuthIndex: "p1", Name: "p1.json", FileName: "p1.json",
			Classification: "permission_denied", Action: "keep",
			ErrorCode: permissionDeniedErrorCode, HTTPStatus: 403,
		},
		{
			AuthIndex: "s1", Name: "s1.json", FileName: "s1.json",
			Classification: "spending_limit", Action: "keep",
			ErrorCode: spendingLimitErrorCode, HTTPStatus: 402,
		},
		{
			AuthIndex: "q1", Name: "q1.json", FileName: "q1.json",
			Classification: "quota_exhausted", Action: "disable",
			ErrorCode: exhaustedErrorCode, HTTPStatus: 429,
		},
	}}

	cands403, err := e.collectCandidates(applyRequest{
		AuthIndexes:  []string{"p1"},
		ForceAction:  "disable",
		BanErrorCode: permissionDeniedErrorCode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands403) != 1 || cands403[0].BanErrorCode != permissionDeniedErrorCode {
		t.Fatalf("403 explicit reason overwritten: %+v", cands403)
	}

	cands402, err := e.collectCandidates(applyRequest{
		AuthIndexes:  []string{"s1"},
		ForceAction:  "disable",
		BanErrorCode: spendingLimitErrorCode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands402) != 1 || cands402[0].BanErrorCode != spendingLimitErrorCode {
		t.Fatalf("402 explicit reason overwritten: %+v", cands402)
	}

	candsQuotaExplicit, err := e.collectCandidates(applyRequest{
		AuthIndexes:  []string{"q1"},
		ForceAction:  "disable",
		BanErrorCode: permissionDeniedErrorCode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candsQuotaExplicit) != 1 || candsQuotaExplicit[0].BanErrorCode != permissionDeniedErrorCode {
		t.Fatalf("explicit BanErrorCode lost on quota row: %+v", candsQuotaExplicit)
	}
}

func TestCollectCandidatesForceDisableNonQuotaStaysManual(t *testing.T) {
	e := &inspectionEngine{results: []accountResult{
		{AuthIndex: "h1", Name: "h1.json", FileName: "h1.json", Classification: "healthy", Action: "keep"},
		{AuthIndex: "r1", Name: "r1.json", FileName: "r1.json", Classification: "reauth", Action: "delete", ErrorCode: unauthorizedErrorCode, HTTPStatus: 401},
		{AuthIndex: "q1", Name: "q1.json", FileName: "q1.json", Classification: "quota_exhausted", Action: "disable", ErrorCode: exhaustedErrorCode, HTTPStatus: 429},
	}}
	cands, err := e.collectCandidates(applyRequest{
		AuthIndexes: []string{"h1", "r1", "q1"},
		ForceAction: "disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 3 {
		t.Fatalf("candidates=%d want 3", len(cands))
	}
	by := map[string]string{}
	for _, c := range cands {
		by[c.AuthIndex] = c.BanErrorCode
	}
	if by["h1"] != manualInspectionBanErrorCode || by["r1"] != manualInspectionBanErrorCode {
		t.Fatalf("non-quota force disable must stay manual: %+v", by)
	}
	if by["q1"] != exhaustedErrorCode {
		t.Fatalf("quota force disable ban=%q", by["q1"])
	}
}

func TestSyncInspectionBanQuotaUsesFallbackHours(t *testing.T) {
	store := newBanStore()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	old := loadedConfig()
	cfg := old
	cfg.FallbackHours = 24
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(old) })

	if !syncInspectionBan(store, nil, "quota.json", now, exhaustedErrorCode) {
		t.Fatal("expected ban recorded")
	}
	entry, ok := store.Get("quota.json")
	if !ok {
		t.Fatal("missing ban entry")
	}
	if entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("error code=%q", entry.ErrorCode)
	}
	wantReset := now.Add(24 * time.Hour)
	if !entry.ResetAt.Equal(wantReset) {
		t.Fatalf("reset_at=%s want %s", entry.ResetAt, wantReset)
	}
	if entry.ResetSource == manualInspectionBanResetSource {
		t.Fatalf("quota ban must auto-restore, got source=%q", entry.ResetSource)
	}
	if entry.ResetSource != "local_plus_fallback" {
		t.Fatalf("reset source=%q want local_plus_fallback", entry.ResetSource)
	}
}

func TestSyncInspectionBanManualStaysPermanent(t *testing.T) {
	store := newBanStore()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	if !syncInspectionBan(store, nil, "manual.json", now, manualInspectionBanErrorCode) {
		t.Fatal("expected manual ban")
	}
	entry, ok := store.Get("manual.json")
	if !ok {
		t.Fatal("missing")
	}
	if entry.ErrorCode != manualInspectionBanErrorCode || entry.ResetSource != manualInspectionBanResetSource {
		t.Fatalf("manual entry=%+v", entry)
	}
	if entry.ResetAt.Before(now.AddDate(50, 0, 0)) {
		t.Fatalf("manual ban should be permanent, reset_at=%s", entry.ResetAt)
	}
}

func TestSuggestedApplyQuotaDisableCreatesAutoRestoreBan(t *testing.T) {
	resetEngineActionStateForQuotaTest(t)
	dir := withQuotaTestResultsPath(t)
	withQuotaBanConfig(t, dir, 24)
	_ = withCPAStatusRecorder(t)

	engine.mu.Lock()
	engine.results = []accountResult{{
		AuthIndex: "q-apply", Name: "q-apply.json", FileName: "q-apply.json",
		Classification: "quota_exhausted", Action: "disable",
		ErrorCode: exhaustedErrorCode, HTTPStatus: 429, Disabled: false,
	}}
	engine.mu.Unlock()

	if err := engine.startApply(applyRequest{Lang: "zh"}, "test-pass", nil); err != nil {
		t.Fatal(err)
	}
	waitApplyIdle(t)
	engine.waitAsyncPersist()

	entry, ok := activeStore.Get("q-apply.json")
	if !ok {
		t.Fatal("suggested apply did not record ban")
	}
	if entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("error code=%q want exhausted", entry.ErrorCode)
	}
	if entry.ResetSource == manualInspectionBanResetSource {
		t.Fatalf("suggested quota disable must auto-restore, source=%q", entry.ResetSource)
	}
	if entry.ResetAt.After(time.Now().Add(48 * time.Hour)) {
		t.Fatalf("reset window too long: %s", entry.ResetAt)
	}
}

func TestSingleDisableQuotaCreatesAutoRestoreBan(t *testing.T) {
	resetEngineActionStateForQuotaTest(t)
	dir := withQuotaTestResultsPath(t)
	withQuotaBanConfig(t, dir, 24)
	_ = withCPAStatusRecorder(t)

	engine.mu.Lock()
	engine.results = []accountResult{{
		AuthIndex: "q-one", Name: "q-one.json", FileName: "q-one.json",
		Classification: "quota_exhausted", Action: "disable",
		ErrorCode: exhaustedErrorCode, HTTPStatus: 429, Disabled: false,
	}}
	engine.mu.Unlock()

	seq, action, err := engine.startAction(actionRequest{
		Lang: "zh", Name: "q-one.json", Disabled: true,
	}, "test-pass", nil)
	if err != nil {
		t.Fatal(err)
	}
	if action != "disable" || seq == 0 {
		t.Fatalf("action=%s seq=%d", action, seq)
	}
	waitRowActionSeq(t, seq)
	engine.waitAsyncPersist()

	entry, ok := activeStore.Get("q-one.json")
	if !ok {
		t.Fatal("single disable did not record ban")
	}
	if entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("error code=%q", entry.ErrorCode)
	}
	if entry.ResetSource == manualInspectionBanResetSource {
		t.Fatalf("single quota disable must auto-restore, source=%q", entry.ResetSource)
	}
}

func TestSingleDisableHealthyStaysManual(t *testing.T) {
	resetEngineActionStateForQuotaTest(t)
	_ = withQuotaTestResultsPath(t)
	_ = withCPAStatusRecorder(t)

	engine.mu.Lock()
	engine.results = []accountResult{{
		AuthIndex: "h-one", Name: "h-one.json", FileName: "h-one.json",
		Classification: "healthy", Action: "keep", Disabled: false,
	}}
	engine.mu.Unlock()

	seq, _, err := engine.startAction(actionRequest{
		Lang: "zh", Name: "h-one.json", Disabled: true,
	}, "test-pass", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitRowActionSeq(t, seq)
	engine.waitAsyncPersist()

	entry, ok := activeStore.Get("h-one.json")
	if !ok {
		t.Fatal("manual ban missing")
	}
	if entry.ErrorCode != manualInspectionBanErrorCode || entry.ResetSource != manualInspectionBanResetSource {
		t.Fatalf("healthy single disable must stay manual: %+v", entry)
	}
}

func TestLookupResultForDisablePrefersAuthIndexOverNameCollision(t *testing.T) {
	results := []accountResult{
		{
			AuthIndex: "idx-healthy", Name: "quota.json", FileName: "healthy.json",
			Classification: "healthy", Action: "keep",
		},
		{
			AuthIndex: "idx-quota", Name: "user@example.com", FileName: "quota.json",
			Classification: "quota_exhausted", Action: "disable",
			ErrorCode: exhaustedErrorCode, HTTPStatus: 429,
		},
	}
	item, ok := lookupResultForDisable(results, "quota.json", "idx-quota")
	if !ok {
		t.Fatal("expected match")
	}
	if item.AuthIndex != "idx-quota" {
		t.Fatalf("matched %+v, want auth-index preferred quota row", item)
	}
	if got := inspectionDisableBanErrorCode(item); got != exhaustedErrorCode {
		t.Fatalf("ban code=%q", got)
	}
}

func TestLookupResultForDisableStrictAuthIndexBeatsEarlierAlias(t *testing.T) {
	for _, tc := range []struct {
		name string
		row1 accountResult
	}{
		{
			name: "name-alias",
			row1: accountResult{
				AuthIndex: "other-1", Name: "idx-target", FileName: "other-1.json",
				Classification: "healthy",
			},
		},
		{
			name: "filename-alias",
			row1: accountResult{
				AuthIndex: "other-2", Name: "other-2@x", FileName: "idx-target",
				Classification: "healthy",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results := []accountResult{
				tc.row1,
				{
					AuthIndex: "idx-target", Name: "real@x", FileName: "real.json",
					Classification: "quota_exhausted", ErrorCode: exhaustedErrorCode,
				},
			}
			item, ok := lookupResultForDisable(results, "ignored.json", "idx-target")
			if !ok {
				t.Fatal("expected match")
			}
			if item.AuthIndex != "idx-target" || item.FileName != "real.json" {
				t.Fatalf("strict AuthIndex lost to earlier alias: %+v", item)
			}
		})
	}
}

func TestFindAuthFromResultsFileNameBeatsAuthIndexCollision(t *testing.T) {
	// Management strings are physical file names. If another row\'s AuthIndex equals
	// that file name, FileName must still win.
	engine.mu.Lock()
	old := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "real-file.json", Name: "other@x", FileName: "other.json"},
		{AuthIndex: "idx-real", Name: "display@x", FileName: "real-file.json"},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = old
		engine.mu.Unlock()
	})

	entry := findAuthFromResults("real-file.json")
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry.AuthIndex != "idx-real" || entry.Name != "real-file.json" {
		t.Fatalf("FileName must beat AuthIndex collision: %+v", entry)
	}
}

func TestSingleDisableAuthIndexCollisionUsesPhysicalFile(t *testing.T) {
	// First row Name/FileName equals target AuthIndex; second row is the real account.
	for _, tc := range []struct {
		name string
		row1 accountResult
	}{
		{
			name: "name-equals-authindex",
			row1: accountResult{
				AuthIndex: "idx-healthy", Name: "idx-quota", FileName: "healthy.json",
				Classification: "healthy", Action: "keep", Disabled: false,
			},
		},
		{
			name: "filename-equals-authindex",
			row1: accountResult{
				AuthIndex: "idx-healthy", Name: "healthy@x", FileName: "idx-quota",
				Classification: "healthy", Action: "keep", Disabled: false,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetEngineActionStateForQuotaTest(t)
			dir := withQuotaTestResultsPath(t)
			withQuotaBanConfig(t, dir, 24)
			patches := withCPAStatusRecorder(t)

			engine.mu.Lock()
			engine.results = []accountResult{
				tc.row1,
				{
					AuthIndex: "idx-quota", Name: "user@example.com", FileName: "quota-real.json",
					Classification: "quota_exhausted", Action: "disable",
					ErrorCode: exhaustedErrorCode, HTTPStatus: 429, Disabled: false,
				},
			}
			engine.mu.Unlock()

			seq, _, err := engine.startAction(actionRequest{
				Lang:      "zh",
				Name:      "collision-display",
				AuthIndex: "idx-quota",
				Disabled:  true,
			}, "test-pass", nil)
			if err != nil {
				t.Fatal(err)
			}
			waitRowActionSeq(t, seq)
			engine.waitAsyncPersist()

			if len(*patches) != 1 || (*patches)[0].Name != "quota-real.json" || !(*patches)[0].Disabled {
				t.Fatalf("CPA PATCH=%v want single disable of quota-real.json", *patches)
			}
			entry, ok := activeStore.Get("quota-real.json")
			if !ok {
				var keys []string
				for _, e := range activeStore.All() {
					keys = append(keys, e.AuthID)
				}
				t.Fatalf("ban key must be quota-real.json, keys=%v", keys)
			}
			if entry.ErrorCode != exhaustedErrorCode {
				t.Fatalf("ban=%+v", entry)
			}
			if entry.ResetSource == manualInspectionBanResetSource {
				t.Fatalf("must auto-restore, source=%q", entry.ResetSource)
			}
			if _, ok := activeStore.Get("healthy.json"); ok {
				t.Fatal("healthy.json must not be banned")
			}
			if _, ok := activeStore.Get("idx-quota"); ok {
				t.Fatal("AuthIndex must not become ban key when FileName exists")
			}
		})
	}
}

func TestSingleDisableFileNameBeatsOtherRowAuthIndex(t *testing.T) {
	// Row select via strict AuthIndex; Management target is physical FileName.
	// Another row\'s AuthIndex equals that FileName and must not steal the CPA PATCH.
	resetEngineActionStateForQuotaTest(t)
	dir := withQuotaTestResultsPath(t)
	withQuotaBanConfig(t, dir, 24)
	patches := withCPAStatusRecorder(t)

	engine.mu.Lock()
	engine.results = []accountResult{
		{
			AuthIndex: "quota-real.json", Name: "collider@x", FileName: "healthy.json",
			Classification: "healthy", Action: "keep", Disabled: false,
		},
		{
			AuthIndex: "idx-quota", Name: "user@example.com", FileName: "quota-real.json",
			Classification: "quota_exhausted", Action: "disable",
			ErrorCode: exhaustedErrorCode, HTTPStatus: 429, Disabled: false,
		},
	}
	engine.mu.Unlock()

	seq, _, err := engine.startAction(actionRequest{
		Lang:      "zh",
		Name:      "quota-real.json", // UI actionTargetName prefers file_name
		AuthIndex: "idx-quota",
		Disabled:  true,
	}, "test-pass", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitRowActionSeq(t, seq)
	engine.waitAsyncPersist()

	if len(*patches) != 1 || (*patches)[0].Name != "quota-real.json" || !(*patches)[0].Disabled {
		t.Fatalf("CPA PATCH=%v want disable quota-real.json", *patches)
	}
	entry, ok := activeStore.Get("quota-real.json")
	if !ok {
		var keys []string
		for _, e := range activeStore.All() {
			keys = append(keys, e.AuthID)
		}
		t.Fatalf("ban key must be quota-real.json, keys=%v", keys)
	}
	if entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("ban=%+v", entry)
	}
	if _, ok := activeStore.Get("healthy.json"); ok {
		t.Fatal("collider row must not be banned")
	}
}

func TestSingleDisableWithNameAndAuthIndexUsesQuotaReason(t *testing.T) {
	resetEngineActionStateForQuotaTest(t)
	dir := withQuotaTestResultsPath(t)
	withQuotaBanConfig(t, dir, 24)
	patches := withCPAStatusRecorder(t)

	engine.mu.Lock()
	engine.results = []accountResult{
		{
			AuthIndex: "idx-healthy", Name: "quota.json", FileName: "healthy.json",
			Classification: "healthy", Action: "keep", Disabled: false,
		},
		{
			AuthIndex: "idx-quota", Name: "user@example.com", FileName: "quota.json",
			Classification: "quota_exhausted", Action: "disable",
			ErrorCode: exhaustedErrorCode, HTTPStatus: 429, Disabled: false,
		},
	}
	engine.mu.Unlock()

	seq, _, err := engine.startAction(actionRequest{
		Lang:      "zh",
		Name:      "quota.json",
		AuthIndex: "idx-quota",
		Disabled:  true,
	}, "test-pass", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitRowActionSeq(t, seq)
	engine.waitAsyncPersist()

	if len(*patches) != 1 || (*patches)[0].Name != "quota.json" || !(*patches)[0].Disabled {
		t.Fatalf("CPA PATCH=%v want single disable of quota.json", *patches)
	}
	entry, ok := activeStore.Get("quota.json")
	if !ok {
		var keys []string
		for _, e := range activeStore.All() {
			keys = append(keys, e.AuthID)
		}
		t.Fatalf("canonical ban key quota.json missing, keys=%v", keys)
	}
	if entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("expected exhausted ban, got %+v", entry)
	}
	if entry.ResetSource == manualInspectionBanResetSource {
		t.Fatalf("must auto-restore, source=%q", entry.ResetSource)
	}
	if _, ok := activeStore.Get("healthy.json"); ok {
		t.Fatal("healthy.json must not receive the ban")
	}
}

func TestStartActionEnableDeleteDoNotRewriteTarget(t *testing.T) {
	resetEngineActionStateForQuotaTest(t)
	_ = withQuotaTestResultsPath(t)

	var patchNames []string
	var deletePaths []string
	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletePaths = append(deletePaths, r.URL.Path+"?"+r.URL.RawQuery)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		patchNames = append(patchNames, body.Name)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	engine.mu.Lock()
	engine.results = []accountResult{
		{AuthIndex: "idx-a", Name: "a.json", FileName: "a.json", Classification: "healthy", Action: "keep", Disabled: true},
		{AuthIndex: "idx-b", Name: "b.json", FileName: "b.json", Classification: "quota_exhausted", Action: "disable", ErrorCode: exhaustedErrorCode, Disabled: true},
	}
	engine.mu.Unlock()

	patchNames = nil
	seqEn, _, err := engine.startAction(actionRequest{
		Lang: "zh", Name: "a.json", AuthIndex: "idx-b", Disabled: false,
	}, "test-pass", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitRowActionSeq(t, seqEn)
	if len(patchNames) != 1 || patchNames[0] != "a.json" {
		t.Fatalf("enable rewritten target: patches=%v want [a.json]", patchNames)
	}

	deletePaths = nil
	engine.mu.Lock()
	engine.results = []accountResult{
		{AuthIndex: "idx-a", Name: "a.json", FileName: "a.json", Disabled: false},
		{AuthIndex: "idx-b", Name: "b.json", FileName: "b.json", Disabled: false},
	}
	engine.mu.Unlock()
	seqDel, _, err := engine.startAction(actionRequest{
		Lang: "zh", Name: "a.json", AuthIndex: "idx-b", Delete: true,
	}, "test-pass", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitRowActionSeq(t, seqDel)
	joined := strings.Join(deletePaths, " ")
	if !strings.Contains(joined, "a.json") || strings.Contains(joined, "b.json") {
		t.Fatalf("delete rewritten target: paths=%v", deletePaths)
	}
}

func TestInspectionQuotaBanRestoresAfterFallbackWindow(t *testing.T) {
	var enabled []string
	withCPAManagement(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name     string `json:"name"`
			Disabled bool   `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !body.Disabled {
			enabled = append(enabled, body.Name)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	store := newBanStore()
	banAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	old := loadedConfig()
	cfg := old
	cfg.FallbackHours = 24
	currentConfig.Store(cfg)
	t.Cleanup(func() { currentConfig.Store(old) })

	if !syncInspectionBan(store, nil, "restore-quota.json", banAt, exhaustedErrorCode) {
		t.Fatal("syncInspectionBan failed")
	}
	entry, ok := store.Get("restore-quota.json")
	if !ok || entry.ErrorCode != exhaustedErrorCode {
		t.Fatalf("ban missing/wrong: %+v", entry)
	}
	if !entry.ResetAt.Equal(banAt.Add(24 * time.Hour)) {
		t.Fatalf("reset_at=%s", entry.ResetAt)
	}

	now := entry.ResetAt.Add(time.Minute)
	restored, failed := restoreExpiredBans(store, now)
	if restored != 1 || failed != 0 {
		t.Fatalf("restored=%d failed=%d enabled=%v", restored, failed, enabled)
	}
	if len(enabled) != 1 || enabled[0] != "restore-quota.json" {
		t.Fatalf("enable PATCH calls=%v", enabled)
	}
	if _, ok := store.Get("restore-quota.json"); ok {
		t.Fatal("restored ban must be removed from store")
	}
}
