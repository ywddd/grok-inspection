package main

import (
	"testing"
	"time"
)

// Collision: account A's AuthIndex equals account B's FileName. Selecting A's
// scheduled target must resolve only to A (never also B) for apply + success count.
func TestResolveAccountResultByTargetIDAuthIndexBeatsFileNameCollision(t *testing.T) {
	results := []accountResult{
		{AuthIndex: "shared-token", Name: "a-display", FileName: "a.json", Classification: "quota_exhausted", Action: "disable", HTTPStatus: 429, ErrorCode: exhaustedErrorCode},
		{AuthIndex: "b-auth", Name: "b-display", FileName: "shared-token", Classification: "healthy", Action: "none", HTTPStatus: 200},
	}
	got, ok := resolveAccountResultByTargetID(results, "shared-token")
	if !ok {
		t.Fatal("expected resolve")
	}
	if got.AuthIndex != "shared-token" || got.FileName != "a.json" {
		t.Fatalf("resolved %#v want account A", got)
	}
	rows := collectAccountsByTargetIDs(results, []string{"shared-token"})
	if len(rows) != 1 || rows[0].FileName != "a.json" {
		t.Fatalf("collect=%#v want only A", rows)
	}
}

func TestCollectCandidatesScheduleCollisionDoesNotSelectB(t *testing.T) {
	engine.mu.Lock()
	old := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "shared-token", Name: "acct-a", FileName: "a.json", Classification: "permission_denied", Action: "disable", HTTPStatus: 403, ErrorCode: permissionDeniedErrorCode, Disabled: false},
		{AuthIndex: "acct-b", Name: "acct-b", FileName: "shared-token", Classification: "healthy", Action: "none", HTTPStatus: 200, Disabled: false},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = old
		engine.mu.Unlock()
	})

	cands, err := engine.collectCandidates(applyRequest{
		ForceAction: "disable",
		AuthIndexes: []string{"shared-token"},
		Lang:        "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("candidates=%d want 1 (A only), got %#v", len(cands), cands)
	}
	if cands[0].AuthIndex != "shared-token" || cands[0].FileName != "a.json" {
		t.Fatalf("got %#v", cands[0])
	}
}

func TestScheduledActionSuccessCountNoDoubleCountOnCollision(t *testing.T) {
	engine.mu.Lock()
	old := append([]accountResult(nil), engine.results...)
	pre := []accountResult{
		{AuthIndex: "shared-token", Name: "acct-a", FileName: "a.json", Classification: "quota_exhausted", Disabled: true, HTTPStatus: 429, ErrorCode: exhaustedErrorCode},
		{AuthIndex: "acct-b", Name: "acct-b", FileName: "shared-token", Classification: "healthy", Disabled: false, HTTPStatus: 200},
	}
	engine.results = append([]accountResult(nil), pre...)
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = old
		engine.mu.Unlock()
	})

	// Freeze identity at pre-action time (A), as production disposal does.
	idents := buildScheduledTargetIdentities(pre, []string{"shared-token"})
	if len(idents) != 1 || !idents[0].Resolved || idents[0].FileName != "a.json" {
		t.Fatalf("pre identities=%#v", idents)
	}

	// One target should yield success 1, never 2 (and never negative failed math).
	n := scheduledActionSuccessCountIdentities(idents, "disable")
	if n != 1 {
		t.Fatalf("disable success=%d want 1", n)
	}

	// Delete success counting: both rows still present -> 0 deleted for this target.
	n = scheduledActionSuccessCountIdentities(idents, "delete")
	if n != 0 {
		t.Fatalf("delete success=%d want 0 while A still present", n)
	}

	// Remove only A; B remains with FileName shared-token. Pre-captured identity of A
	// must count deleted=1, not re-resolve token onto B.
	engine.mu.Lock()
	engine.results = []accountResult{
		{AuthIndex: "acct-b", Name: "acct-b", FileName: "shared-token", Classification: "healthy", Disabled: false, HTTPStatus: 200},
	}
	engine.mu.Unlock()
	n = scheduledActionSuccessCountIdentities(idents, "delete")
	if n != 1 {
		t.Fatalf("delete success after A removed=%d want 1", n)
	}
	var disabled, deleted, failed int
	recordScheduledActionProgressIdentities(idents, "delete", &disabled, &deleted, &failed)
	if deleted != 1 || failed != 0 || disabled != 0 {
		t.Fatalf("progress deleted=%d failed=%d disabled=%d", deleted, failed, disabled)
	}
}

func TestScheduledDeleteCollision402403OnlyDeletesA(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		class  string
		code   string
		action string
	}{
		{"403", 403, "permission_denied", permissionDeniedErrorCode, scheduled403Delete},
		{"402", 402, "spending_limit", spendingLimitErrorCode, scheduled402Delete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine.mu.Lock()
			old := append([]accountResult(nil), engine.results...)
			token := "del-collide-" + tc.name
			pre := []accountResult{
				{AuthIndex: token, Name: "acct-a", FileName: "a-" + tc.name + ".json", Classification: tc.class, Action: "delete", HTTPStatus: tc.status, ErrorCode: tc.code, Disabled: true},
				{AuthIndex: "b-" + tc.name, Name: "acct-b", FileName: token, Classification: "healthy", Action: "none", HTTPStatus: 200, Disabled: false},
			}
			engine.results = append([]accountResult(nil), pre...)
			engine.mu.Unlock()
			t.Cleanup(func() {
				engine.mu.Lock()
				engine.results = old
				engine.mu.Unlock()
			})

			targets := scheduledTargetsExact(tc.status, tc.class, tc.code, "delete", nil)
			if len(targets) != 1 || targets[0] != token {
				t.Fatalf("targets=%v", targets)
			}
			cands, err := engine.collectCandidates(applyRequest{ForceAction: "delete", AuthIndexes: targets, Lang: "en"})
			if err != nil {
				t.Fatal(err)
			}
			if len(cands) != 1 || cands[0].AuthIndex != token || cands[0].FileName != "a-"+tc.name+".json" {
				t.Fatalf("cands=%#v want only A", cands)
			}

			idents := buildScheduledTargetIdentities(pre, targets)
			// Simulate successful delete of A only: B survivor keeps FileName=token.
			engine.mu.Lock()
			engine.results = []accountResult{pre[1]}
			engine.mu.Unlock()

			var disabled, deleted, failed int
			recordScheduledActionProgressIdentities(idents, tc.action, &disabled, &deleted, &failed)
			if deleted != 1 || failed != 0 {
				t.Fatalf("%s progress deleted=%d failed=%d want 1/0 (B must not steal success/fail)", tc.name, deleted, failed)
			}
			// B still present.
			engine.mu.Lock()
			got := append([]accountResult(nil), engine.results...)
			engine.mu.Unlock()
			if len(got) != 1 || got[0].AuthIndex != "b-"+tc.name {
				t.Fatalf("survivor rows=%#v want only B", got)
			}
		})
	}
}

func TestScheduledTargetsAndCollectAcrossCodesUseExclusiveResolve(t *testing.T) {
	// Shared selection mechanism for 429/401/402/403/recover-style force apply.
	cases := []struct {
		name   string
		status int
		class  string
		code   string
		action string
	}{
		{"429", 429, "quota_exhausted", exhaustedErrorCode, "disable"},
		{"401", 401, "reauth", unauthorizedErrorCode, "disable"},
		{"402", 402, "spending_limit", spendingLimitErrorCode, "disable"},
		{"403", 403, "permission_denied", permissionDeniedErrorCode, "delete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine.mu.Lock()
			old := append([]accountResult(nil), engine.results...)
			token := "collide-" + tc.name
			engine.results = []accountResult{
				{AuthIndex: token, Name: "target", FileName: "target-" + tc.name + ".json", Classification: tc.class, Action: tc.action, HTTPStatus: tc.status, ErrorCode: tc.code, Disabled: false},
				{AuthIndex: "other-" + tc.name, Name: "other", FileName: token, Classification: "healthy", Action: "none", HTTPStatus: 200, Disabled: false},
			}
			engine.mu.Unlock()
			t.Cleanup(func() {
				engine.mu.Lock()
				engine.results = old
				engine.mu.Unlock()
			})

			targets := scheduledTargetsExact(tc.status, tc.class, tc.code, tc.action, nil)
			if len(targets) != 1 || targets[0] != token {
				t.Fatalf("targets=%v want [%s]", targets, token)
			}
			cands, err := engine.collectCandidates(applyRequest{ForceAction: tc.action, AuthIndexes: targets, Lang: "en"})
			if err != nil {
				t.Fatal(err)
			}
			if len(cands) != 1 || cands[0].AuthIndex != token {
				t.Fatalf("cands=%#v", cands)
			}
		})
	}
}

func TestScheduledHealthyRecoverTargetCollisionExclusive(t *testing.T) {
	isolateActiveStore(t)
	engine.mu.Lock()
	old := append([]accountResult(nil), engine.results...)
	engine.results = []accountResult{
		{AuthIndex: "recover-token", Name: "rec-a", FileName: "rec-a.json", Classification: "healthy", Disabled: true, HTTPStatus: 200},
		{AuthIndex: "rec-b", Name: "rec-b", FileName: "recover-token", Classification: "healthy", Disabled: true, HTTPStatus: 200},
	}
	engine.mu.Unlock()
	t.Cleanup(func() {
		engine.mu.Lock()
		engine.results = old
		engine.mu.Unlock()
	})
	now := time.Now()
	// Only A has exclusive exact quota ban -> recover target.
	activeStore.Set(banEntry{
		AuthID: "recover-token", Provider: "xai", ErrorCode: exhaustedErrorCode,
		BannedAt: now.Add(-time.Hour), ResetAt: now.Add(-time.Minute), ResetSource: "local_plus_fallback", CpaSynced: true,
	})
	// B also disabled healthy but ban is permission -> not auto-recover target.
	activeStore.Set(banEntry{
		AuthID: "rec-b", Provider: "xai", ErrorCode: permissionDeniedErrorCode,
		BannedAt: now, ResetAt: now.AddDate(10, 0, 0), ResetSource: "manual_unban", CpaSynced: true,
	})

	targets := scheduledHealthyRecoverTargets(nil)
	if len(targets) != 1 || targets[0] != "recover-token" {
		t.Fatalf("recover targets=%v", targets)
	}
	cands, err := engine.collectCandidates(applyRequest{ForceAction: "enable", AuthIndexes: targets, Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].FileName != "rec-a.json" {
		t.Fatalf("recover cands=%#v want only A", cands)
	}
}
