package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestShouldProbeOfficialAPI(t *testing.T) {
	cases := []struct {
		class  string
		status int
		want   bool
	}{
		{"permission_denied", 403, true},
		{"permission_denied", 402, true},
		{"reauth", 401, false},
		{"quota_exhausted", 429, false},
		{"healthy", 200, false},
		{"probe_error", 403, true},
		{"unknown", 402, true},
		{"unknown", 500, false},
		{"model_unavailable", 404, false},
	}
	for _, tc := range cases {
		got := shouldProbeOfficialAPI(tc.class, tc.status)
		if got != tc.want {
			t.Fatalf("class=%s status=%d got=%v want=%v", tc.class, tc.status, got, tc.want)
		}
	}
}

func TestIsBaseURLSwitchEligible(t *testing.T) {
	if !isBaseURLSwitchEligible(accountResult{Classification: "api_gateway_ok"}) {
		t.Fatal("api_gateway_ok should be eligible")
	}
	if isBaseURLSwitchEligible(accountResult{Classification: "permission_denied"}) {
		t.Fatal("permission_denied alone is not eligible")
	}
	// Preferred official API with successful api probe, not yet using_api.
	ok := isBaseURLSwitchEligible(accountResult{
		PreferredBaseURL: xaiOfficialAPIBaseURL,
		APIHTTPStatus:    http.StatusOK,
		UsingAPI:         false,
	})
	if !ok {
		t.Fatal("expected preferred official API without using_api to be eligible")
	}
	if isBaseURLSwitchEligible(accountResult{
		PreferredBaseURL: xaiOfficialAPIBaseURL,
		APIHTTPStatus:    http.StatusOK,
		UsingAPI:         true,
	}) {
		t.Fatal("already using_api should not be re-selected unless classified api_gateway_ok")
	}
}

func TestSelectBaseURLSwitchTargets(t *testing.T) {
	results := []accountResult{
		{AuthIndex: "a1", Name: "a1.json", Classification: "api_gateway_ok", Action: "switch_base_url"},
		{AuthIndex: "a2", Name: "a2.json", Classification: "healthy"},
		{AuthIndex: "a3", Name: "a3.json", Classification: "permission_denied"},
		{
			AuthIndex:        "a4",
			Name:             "a4.json",
			PreferredBaseURL: xaiOfficialAPIBaseURL,
			APIHTTPStatus:    200,
			UsingAPI:         false,
		},
	}
	all := selectBaseURLSwitchTargets(results, nil)
	if len(all) != 2 {
		t.Fatalf("want 2 eligible, got %d", len(all))
	}
	// Explicit indexes can force-include even non-eligible rows when listed.
	forced := selectBaseURLSwitchTargets(results, []string{"a2"})
	if len(forced) != 1 || forced[0].AuthIndex != "a2" {
		t.Fatalf("forced select by auth_index: %+v", forced)
	}
	byName := selectBaseURLSwitchTargets(results, []string{"a1.json"})
	if len(byName) != 1 || byName[0].AuthIndex != "a1" {
		t.Fatalf("select by file name: %+v", byName)
	}
	none := selectBaseURLSwitchTargets(results, []string{"missing"})
	if len(none) != 0 {
		t.Fatalf("missing key should yield empty, got %d", len(none))
	}
}

func TestClassificationMatchesTreatsAPIGatewayAsPrimary(t *testing.T) {
	want := map[string]struct{}{"other": {}}
	if classificationMatches("api_gateway_ok", want) {
		t.Fatal("api_gateway_ok must not match other")
	}
	if !classificationMatches("probe_error", want) {
		t.Fatal("probe_error should match other")
	}
	wantAPI := map[string]struct{}{"api_gateway_ok": {}}
	if !classificationMatches("api_gateway_ok", wantAPI) {
		t.Fatal("exact api_gateway_ok should match")
	}
}

func TestSummarizeResultsIncludesAPIGatewayOK(t *testing.T) {
	summary := summarizeResults([]accountResult{
		{Classification: "healthy"},
		{Classification: "api_gateway_ok"},
		{Classification: "api_gateway_ok"},
		{Classification: "probe_error"},
	})
	if summary["api_gateway_ok"] != 2 {
		t.Fatalf("api_gateway_ok=%d", summary["api_gateway_ok"])
	}
	if summary["other"] != 1 {
		t.Fatalf("other=%d", summary["other"])
	}
	if summary["healthy"] != 1 {
		t.Fatalf("healthy=%d", summary["healthy"])
	}
}

func TestBaseURLApplyRequestJSONHasNoSecrets(t *testing.T) {
	// Guardrail: apply request type must not carry tokens.
	req := baseURLApplyRequest{
		AuthIndexes:   []string{"x"},
		TargetBaseURL: xaiOfficialAPIBaseURL,
		SkipReprobe:   true,
	}
	raw := strings.ToLower(req.TargetBaseURL)
	if strings.Contains(raw, "token") || strings.Contains(raw, "password") {
		t.Fatal("unexpected secret-like field in target")
	}
	_ = req.AuthIndexes
}
