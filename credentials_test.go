package main

import (
	"strings"
	"testing"
)

func TestParseAccountsContent(t *testing.T) {
	content := strings.Join([]string{
		"# comment",
		"",
		"alice@x.ai----pass1----sso1",
		"bob@x.ai----pass2",
		"bad-line-without-sep",
		"----nopass----sso",
		"carol@x.ai——pass3——sso3", // fullwidth dash normalized
	}, "\n")
	accepted, skipped := parseAccountsContent(content)
	if len(accepted) != 3 {
		t.Fatalf("accepted=%d want 3: %+v", len(accepted), accepted)
	}
	if skipped != 2 {
		t.Fatalf("skipped=%d want 2", skipped)
	}
	if accepted[0].Email != "alice@x.ai" || accepted[0].Password != "pass1" || accepted[0].SSO != "sso1" {
		t.Fatalf("alice line: %+v", accepted[0])
	}
	if accepted[1].Email != "bob@x.ai" || accepted[1].Password != "pass2" || accepted[1].SSO != "" {
		t.Fatalf("bob line: %+v", accepted[1])
	}
	if accepted[2].Email != "carol@x.ai" || accepted[2].Password != "pass3" {
		t.Fatalf("carol line: %+v", accepted[2])
	}
}

func TestCredentialStoreExactMatchAndEligible(t *testing.T) {
	store := &credentialStore{byEmail: map[string]credentialLine{}}
	summary, err := store.replaceFromContent(strings.Join([]string{
		"match@x.ai----p----s",
		"healthy@x.ai----p----s",
		"missing@x.ai----p----s",
		"match@x.ai----p2----s2", // last wins
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if summary.Uploaded != 4 || summary.Unique != 3 {
		t.Fatalf("summary uploaded/unique = %d/%d", summary.Uploaded, summary.Unique)
	}
	line, ok := store.lookupExact("match@x.ai")
	if !ok || line.Password != "p2" {
		t.Fatalf("last-wins password: %+v ok=%v", line, ok)
	}
	// Exact match only — different case must not hit.
	if _, ok := store.lookupExact("Match@x.ai"); ok {
		t.Fatal("case-insensitive match must not succeed")
	}

	results := []accountResult{
		{AuthIndex: "1", Name: "xai-match@x.ai.json", FileName: "xai-match@x.ai.json", Email: "match@x.ai", Classification: "permission_denied", HTTPStatus: 403},
		{AuthIndex: "2", Name: "healthy", FileName: "xai-healthy@x.ai.json", Email: "healthy@x.ai", Classification: "healthy", HTTPStatus: 200},
		{AuthIndex: "3", Name: "other", FileName: "xai-other@x.ai.json", Email: "other@x.ai", Classification: "reauth", HTTPStatus: 401},
	}
	eligible := store.matchedEligible(results)
	if len(eligible) != 1 || eligible[0].Email != "match@x.ai" {
		t.Fatalf("eligible=%+v", eligible)
	}
	sum := store.summaryAgainst(results)
	if sum.Matched != 2 || sum.Eligible != 1 || sum.Unmatched != 1 {
		t.Fatalf("summaryAgainst: matched=%d eligible=%d unmatched=%d", sum.Matched, sum.Eligible, sum.Unmatched)
	}
}

func TestIsReauthEligible(t *testing.T) {
	cases := []struct {
		r    accountResult
		want bool
	}{
		{accountResult{Classification: "reauth"}, true},
		{accountResult{Classification: "permission_denied", HTTPStatus: 403}, true},
		{accountResult{Classification: "permission_denied", HTTPStatus: 402}, false},
		{accountResult{Classification: "healthy", HTTPStatus: 401}, true},
		{accountResult{Classification: "quota_exhausted", HTTPStatus: 429}, false},
	}
	for _, tc := range cases {
		if got := isReauthEligible(tc.r); got != tc.want {
			t.Fatalf("isReauthEligible(%+v)=%v want %v", tc.r, got, tc.want)
		}
	}
}

func TestMatchResultEmailExact(t *testing.T) {
	r := accountResult{Email: "a@x.ai", Name: "xai-a@x.ai.json", FileName: "xai-a@x.ai.json"}
	if !matchResultEmail(r, "a@x.ai") {
		t.Fatal("email field should match")
	}
	if matchResultEmail(r, "A@x.ai") {
		t.Fatal("must be exact case match")
	}
	r2 := accountResult{FileName: "xai-b@x.ai.json"}
	if !matchResultEmail(r2, "b@x.ai") {
		t.Fatal("xai-<email>.json should match")
	}
}
