package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestClassifyPermissionDenied(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: 403,
		ChatCode:   "permission-denied",
		ChatError:  "Access to the chat endpoint is denied",
		Disabled:   false,
	})
	if got.Classification != "permission_denied" || got.Action != "disable" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifySpendingLimit402Separately(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusPaymentRequired,
		ChatCode:   spendingLimitErrorCode,
		ChatError:  "You have run out of credits or need a Grok subscription",
	})
	if got.Classification != "spending_limit" || got.Action != "disable" {
		t.Fatalf("got %+v", got)
	}

	disabled := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusPaymentRequired,
		ChatCode:   spendingLimitErrorCode,
		Disabled:   true,
	})
	if disabled.Classification != "spending_limit" || disabled.Action != "keep" {
		t.Fatalf("disabled account got %+v", disabled)
	}
}

func TestClassifyUnknown402IsNotPermissionDenied(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusPaymentRequired,
		ChatCode:   "payment-required",
		ChatError:  "payment required",
	})
	if got.Classification == "permission_denied" || got.Classification == "spending_limit" {
		t.Fatalf("unknown 402 must not be permission_denied/spending_limit: %+v", got)
	}
	if got.Action != "keep" {
		t.Fatalf("unknown 402 action=%q want keep", got.Action)
	}
}

func TestClassifyReauthRecommendsDelete(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusUnauthorized,
		ChatError:  "Invalid or expired credentials",
	})
	if got.Classification != "reauth" || got.Action != "delete" {
		t.Fatalf("got %+v", got)
	}
}

func TestShouldInspectOnlyDisabled(t *testing.T) {
	if !shouldInspectEntry("xai", "xai-a.json", "xai", true, "", false, true) {
		t.Fatal("expected disabled xai account")
	}
	if shouldInspectEntry("xai", "xai-b.json", "xai", false, "", false, true) {
		t.Fatal("expected enabled account skipped")
	}
	if shouldInspectEntry("codex", "c.json", "codex", true, "", true, true) {
		t.Fatal("expected non-xai skipped")
	}
}

func TestPickModel(t *testing.T) {
	body := `{"data":[{"id":"grok-3"},{"id":"grok-4.5-build-free"}]}`
	if got := pickModel(body); got != "grok-4.5-build-free" {
		t.Fatalf("got %s", got)
	}
}

func TestXAIInspectionHeadersMatchCLIProxyIdentity(t *testing.T) {
	headers := xaiInspectionHeaders("test-token", true)

	if got := headers.Get("X-XAI-Token-Auth"); got != "xai-grok-cli" {
		t.Fatalf("X-XAI-Token-Auth = %q", got)
	}
	if got := headers.Get("x-grok-client-version"); got != "0.2.93" {
		t.Fatalf("x-grok-client-version = %q", got)
	}
	if got := headers.Get("User-Agent"); got != "xai-grok-workspace/0.2.93" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := headers.Get("x-grok-client-identifier"); got != "" {
		t.Fatalf("unexpected x-grok-client-identifier = %q", got)
	}
	if got := headers.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestResolveProbeOutcomeKeepsPrimaryQuotaWhenFallbackHealthy(t *testing.T) {
	primary := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusTooManyRequests,
		Body:       `{"code":"free-usage-exhausted","error":"Included free usage has been exhausted"}`,
	}, false, LangZH)
	fallback := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusOK,
		Body:       `{"choices":[{"message":{"content":"pong"}}]}`,
	}, false, LangZH)

	got := resolveProbeOutcome(primary, fallback, LangZH)
	if got.Classified.Classification != "quota_exhausted" || got.Classified.Action != "disable" {
		t.Fatalf("got %+v", got)
	}
	if got.Response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", got.Response.StatusCode)
	}
	if !strings.Contains(got.Classified.Reason, "结果不一致") && !strings.Contains(got.Classified.Reason, "disagree") && !strings.Contains(got.Classified.Reason, "fallback") {
		t.Fatalf("reason = %q", got.Classified.Reason)
	}
}

func TestResolveProbeOutcomeUsesFallbackForAmbiguousPrimary(t *testing.T) {
	primary := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       `{"error":"temporary upstream failure"}`,
	}, false, LangZH)
	fallback := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusOK,
		Body:       `{"choices":[{"message":{"content":"pong"}}]}`,
	}, false, LangZH)

	got := resolveProbeOutcome(primary, fallback, LangZH)
	if got.Classified.Classification != "healthy" {
		t.Fatalf("got %+v", got)
	}
}

func TestClassifyBare429IsProbeErrorNotQuota(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusTooManyRequests,
		ChatError:  "rate limited",
	})
	if got.Classification != "probe_error" || got.Action != "keep" {
		t.Fatalf("bare 429 should be probe_error/keep, got %+v", got)
	}
}

func TestClassifyFreeUsageExhaustedIsQuota(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusTooManyRequests,
		ChatCode:   "free-usage-exhausted",
		ChatError:  "Included free usage has been exhausted",
	})
	if got.Classification != "quota_exhausted" || got.Action != "disable" {
		t.Fatalf("free-usage should be quota_exhausted/disable, got %+v", got)
	}
}

func TestResolveProbeOutcomeUsesFallbackWhenPrimaryBare429(t *testing.T) {
	primary := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusTooManyRequests,
		Body:       `{"error":"too many requests"}`,
	}, false, LangZH)
	fallback := newProbeOutcome(apiCallResponse{
		StatusCode: http.StatusOK,
		Body:       `{"choices":[{"message":{"content":"pong"}}]}`,
	}, false, LangZH)
	got := resolveProbeOutcome(primary, fallback, LangZH)
	if got.Classified.Classification != "healthy" {
		t.Fatalf("bare 429 + healthy fallback should be healthy, got %+v", got)
	}
}

func TestClassifyGenericQuotaTextIsNotQuotaExhausted(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusTooManyRequests,
		ChatCode:   "rate_limit",
		ChatError:  "usage_limit_reached / quota exhausted",
	})
	if got.Classification == "quota_exhausted" {
		t.Fatalf("generic rate-limit text must not be quota_exhausted: %+v", got)
	}
	if got.Classification != "probe_error" || got.Action != "keep" {
		t.Fatalf("want probe_error/keep, got %+v", got)
	}
}

func TestIsFreeUsageExhaustedOnly(t *testing.T) {
	if !isFreeUsageExhausted("free-usage-exhausted", "") {
		t.Fatal("code free-usage-exhausted")
	}
	if !isFreeUsageExhausted("subscription:free-usage-exhausted", "no") {
		t.Fatal("subscription free-usage-exhausted")
	}
	if !isFreeUsageExhausted("", "Included free usage has been exhausted") {
		t.Fatal("old official message")
	}
	if !isFreeUsageExhausted("", "You've used all the included free usage for model grok-4.5-build-free for now") {
		t.Fatal("current official message")
	}
	if isFreeUsageExhausted("rate_limit", "too many requests") {
		t.Fatal("generic rate limit must not match")
	}
	if isFreeUsageExhausted("", "quota exhausted") {
		t.Fatal("bare quota exhausted must not match")
	}
}

func TestClassifyRealGrokFreeUsagePayload(t *testing.T) {
	body := `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage for model grok-4.5-build-free for now. Usage resets over a rolling 24-hour window — tokens (actual/limit): 2012994/2000000. Upgrade to a Grok subscription for higher limits: https://grok.com/supergrok"}`
	parsed := extractError(body)
	if parsed.Code != "subscription:free-usage-exhausted" {
		t.Fatalf("code = %q", parsed.Code)
	}
	if !isFreeUsageExhausted(parsed.Code, parsed.Message) {
		t.Fatalf("should match free usage, code=%q msg=%q", parsed.Code, parsed.Message)
	}
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusTooManyRequests,
		ChatCode:   parsed.Code,
		ChatError:  parsed.Message,
	})
	if got.Classification != "quota_exhausted" || got.Action != "disable" {
		t.Fatalf("got %+v", got)
	}
}

func TestShouldTryFallback(t *testing.T) {
	// Definitive classifications never need the second hop.
	for _, class := range []string{"healthy", "quota_exhausted", "permission_denied", "reauth"} {
		if shouldTryFallback(403, class) {
			t.Fatalf("class %s should not fallback", class)
		}
	}
	// Bare 429 must NOT use chat fallback (budget / abandoned host overlap).
	if shouldTryFallback(429, "probe_error") {
		t.Fatal("bare 429 probe_error should not try fallback")
	}
	if !shouldTryFallback(500, "probe_error") {
		t.Fatal("5xx should try fallback")
	}
}
func TestIsProbeTimeoutErr(t *testing.T) {
	if !isProbeTimeoutErr(fmt.Errorf("HTTP 探测超时（25s）: POST x")) {
		t.Fatal("chinese timeout")
	}
	if !isProbeTimeoutErr(fmt.Errorf("HTTP probe timed out (25s): POST x")) {
		t.Fatal("chinese timeout")
	}
	if !isProbeTimeoutErr(fmt.Errorf("context deadline exceeded: timeout")) {
		t.Fatal("english timeout")
	}
	if isProbeTimeoutErr(fmt.Errorf("permission-denied")) {
		t.Fatal("non-timeout should not match")
	}
}

func TestExtractErrorDoesNotDumpSuccessBody(t *testing.T) {
	body := `{"id":"abc","model":"grok-4.5","object":"response","output":[{"type":"message"}]}`
	got := extractError(body)
	if got.Message != "" || got.Code != "" {
		t.Fatalf("success body must not become error fields: %+v", got)
	}
}

func TestExtractErrorKeepsRealError(t *testing.T) {
	body := `{"code":"permission-denied","error":"Access denied"}`
	got := extractError(body)
	if got.Code != "permission-denied" || got.Message != "Access denied" {
		t.Fatalf("got %+v", got)
	}
}

func TestIsSpendingLimitBlockedExactOnly(t *testing.T) {
	if !isSpendingLimitBlocked(spendingLimitErrorCode, "") {
		t.Fatal("exact spending-limit code must match")
	}
	if !isSpendingLimitBlocked("Personal-Team-Blocked:Spending-Limit", "ignored") {
		t.Fatal("exact code match must be case-insensitive")
	}
	if isSpendingLimitBlocked("", "personal-team-blocked:spending-limit") {
		t.Fatal("message-only must not match")
	}
	if isSpendingLimitBlocked("payment-required", "You have run out of credits") {
		t.Fatal("unknown 402 code must not match")
	}
	if isSpendingLimitBlocked("prefix-"+spendingLimitErrorCode, "") {
		t.Fatal("substring code must not match")
	}
}

func TestClassifyUnknown402IgnoresPermissionHeuristics(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusPaymentRequired,
		ChatCode:   "payment-required",
		ChatError:  "banned / suspended / permission-denied text must not reclassify bare 402",
	})
	if got.Classification == "permission_denied" || got.Classification == "spending_limit" {
		t.Fatalf("unknown 402 must stay non-actionable, got %+v", got)
	}
	if got.Action != "keep" {
		t.Fatalf("unknown 402 action=%q want keep", got.Action)
	}
}

func TestClassifyRealGrok402SpendingLimitPayload(t *testing.T) {
	body := `{"code":"personal-team-blocked:spending-limit","error":"You have run out of credits or need a Grok subscription. Add credits at https://grok.com/?_s=usage or upgrade at https://grok.com/supergrok."}`
	parsed := extractError(body)
	if parsed.Code != spendingLimitErrorCode {
		t.Fatalf("code = %q", parsed.Code)
	}
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusPaymentRequired,
		ChatCode:   parsed.Code,
		ChatError:  parsed.Message,
	})
	if got.Classification != "spending_limit" || got.Action != "disable" {
		t.Fatalf("got %+v", got)
	}
}
