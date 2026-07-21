package main

import (
	"net/http"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

const realGrok429Body = `{"code":"subscription:free-usage-exhausted","error":"You've used all the included free usage for model grok-4.5-build-free for now. Usage resets over a rolling 24-hour window — tokens (actual/limit): 2050798/2000000. Upgrade to a Grok subscription for higher limits: https://grok.com/supergrok"}`
const realGrok403Body = `{"code":"permission-denied","error":"Access to the chat endpoint is denied. Please ensure you're using the correct credentials. If you believe this is a mistake, please log into console.x.ai and update the permissions, or contact support."}`

func TestDetectRealGrokFreeUsageExhausted(t *testing.T) {
	now := time.Date(2026, 7, 12, 11, 40, 0, 0, time.UTC)
	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "xai-account-1",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       realGrok429Body,
		},
		ResponseHeaders: http.Header{
			"Date":         []string{"Sun, 12 Jul 2026 11:33:34 GMT"},
			"X-Request-Id": []string{"0adcec99-a0fb-9519-9498-5d73a4c58035"},
		},
	}

	entry, ok := detectBan(record, defaultPluginConfig(), now)
	if !ok {
		t.Fatal("detectBan() did not match real Grok 429")
	}
	wantReset := time.Date(2026, 7, 13, 11, 33, 34, 0, time.UTC)
	if !entry.ResetAt.Equal(wantReset) {
		t.Fatalf("reset at = %s, want %s", entry.ResetAt, wantReset)
	}
	if entry.ResetSource != "date_plus_fallback" {
		t.Fatalf("reset source = %q", entry.ResetSource)
	}
	if entry.ErrorCode != exhaustedErrorCode || entry.Provider != "xai" {
		t.Fatalf("entry = %#v", entry)
	}
	if entry.TraceID != "0adcec99-a0fb-9519-9498-5d73a4c58035" {
		t.Fatalf("trace id = %q", entry.TraceID)
	}
}

func TestDetectRealGrokPermissionDenied(t *testing.T) {
	now := time.Date(2026, 7, 12, 16, 1, 53, 0, time.UTC)
	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "xai-account-403",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 403,
			Body:       realGrok403Body,
		},
		ResponseHeaders: http.Header{
			"Date":           []string{"Sun, 12 Jul 2026 16:01:53 GMT"},
			"X-Should-Retry": []string{"false"},
		},
	}

	entry, ok := detectBan(record, defaultPluginConfig(), now)
	if !ok {
		t.Fatal("detectBan() did not match real Grok 403 permission-denied")
	}
	if entry.ErrorCode != permissionDeniedErrorCode {
		t.Fatalf("error code = %q", entry.ErrorCode)
	}
	if entry.ResetSource != "manual_unban" {
		t.Fatalf("reset source = %q", entry.ResetSource)
	}
	if !entry.ResetAt.Equal(now.AddDate(100, 0, 0)) {
		t.Fatalf("reset at = %s, want far-future manual unban", entry.ResetAt)
	}
}

func TestDetectGrokUnauthorized401(t *testing.T) {
	now := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)
	record := pluginapi.UsageRecord{
		Provider: "grok",
		AuthID:   "xai-account-401",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 401,
			Body:       `{"error":"invalid credentials"}`,
		},
	}

	entry, ok := detectBan(record, defaultPluginConfig(), now)
	if !ok {
		t.Fatal("detectBan() did not match Grok 401")
	}
	if entry.ErrorCode != unauthorizedErrorCode {
		t.Fatalf("error code = %q", entry.ErrorCode)
	}
	if entry.ResetSource != "manual_unban" {
		t.Fatalf("reset source = %q", entry.ResetSource)
	}
	if !entry.ResetAt.Equal(now.AddDate(100, 0, 0)) {
		t.Fatalf("reset at = %s, want far-future manual unban", entry.ResetAt)
	}

	// Prefer body code when present.
	record.Failure.Body = `{"code":"authentication_error","error":"token expired"}`
	entry, ok = detectBan(record, defaultPluginConfig(), now)
	if !ok {
		t.Fatal("detectBan() did not match Grok 401 with body code")
	}
	if entry.ErrorCode != "authentication_error" {
		t.Fatalf("error code = %q", entry.ErrorCode)
	}
}

func TestDetectBanRejectsNonExactMatches(t *testing.T) {
	base := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "auth-1",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 403,
			Body:       realGrok403Body,
		},
	}
	tests := []struct {
		name   string
		mutate func(*pluginapi.UsageRecord)
	}{
		{"wrong provider", func(r *pluginapi.UsageRecord) { r.Provider = "codex" }},
		{"not failed", func(r *pluginapi.UsageRecord) { r.Failed = false }},
		{"wrong status", func(r *pluginapi.UsageRecord) { r.Failure.StatusCode = 503 }},
		{"empty auth", func(r *pluginapi.UsageRecord) { r.AuthID = "" }},
		{"invalid json", func(r *pluginapi.UsageRecord) { r.Failure.Body = "too many requests" }},
		{"wrong code", func(r *pluginapi.UsageRecord) { r.Failure.Body = `{"code":"rate_limit"}` }},
		{"missing code", func(r *pluginapi.UsageRecord) { r.Failure.Body = `{"error":"rolling 24-hour window"}` }},
		{"403 wrong code", func(r *pluginapi.UsageRecord) {
			r.Failure.StatusCode = 403
			r.Failure.Body = `{"code":"forbidden"}`
		}},
		{"403 without code", func(r *pluginapi.UsageRecord) {
			r.Failure.StatusCode = 403
			r.Failure.Body = `{"error":"Access denied"}`
		}},
		{"401 wrong provider", func(r *pluginapi.UsageRecord) {
			r.Provider = "openai"
			r.Failure.StatusCode = 401
			r.Failure.Body = `{"error":"invalid credentials"}`
		}},
		{"429 wrong code", func(r *pluginapi.UsageRecord) {
			r.Failure.StatusCode = 429
			r.Failure.Body = `{"code":"rate_limit"}`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := base
			tt.mutate(&record)
			if _, ok := detectBan(record, defaultPluginConfig(), time.Now()); ok {
				t.Fatal("detectBan() matched an ineligible record")
			}
		})
	}
}

func TestDetectBanAcceptsGrokProviderAndUsesLocalFallback(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	record := pluginapi.UsageRecord{
		Provider: "GROK",
		AuthID:   "auth-1",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       realGrok429Body,
		},
		ResponseHeaders: http.Header{"Date": []string{"not-a-date"}},
	}
	entry, ok := detectBan(record, defaultPluginConfig(), now)
	if !ok {
		t.Fatal("detectBan() did not match provider GROK")
	}
	if entry.Provider != "xai" {
		t.Fatalf("provider = %q, want xai", entry.Provider)
	}
	if !entry.ResetAt.Equal(now.Add(24*time.Hour)) || entry.ResetSource != "local_plus_fallback" {
		t.Fatalf("entry = %#v", entry)
	}
}