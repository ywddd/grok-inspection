package main

import (
	"net/http"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestDetectBanRejectsBareAuthenticationRequired401(t *testing.T) {
	// Real free accounts often emit transient Cloudflare/proxy 401
	// {"error":"Authentication required"} before a true free-usage 429.
	// That bare signal must not permanently autoban as unauthorized.
	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "xai-tmp-bare-401.json",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 401,
			Body:       `{"error":"Authentication required"}`,
		},
	}
	if _, ok := detectBan(record, defaultPluginConfig(), time.Now()); ok {
		t.Fatal("bare Authentication required 401 must not autoban")
	}
}

func TestDetectBanStillMatchesStrongUnauthorized401(t *testing.T) {
	now := time.Now()
	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "xai-strong-401.json",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: http.StatusUnauthorized,
			Body:       `{"error":"invalid_grant: Access denied"}`,
		},
	}
	entry, ok := detectBan(record, defaultPluginConfig(), now)
	if !ok {
		t.Fatal("strong 401 must still autoban")
	}
	if entry.ErrorCode != unauthorizedErrorCode {
		t.Fatalf("code=%q", entry.ErrorCode)
	}
}

func TestBanStoreExactQuotaSupersedesUnauthorized(t *testing.T) {
	store := newBanStore()
	now := time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC)
	store.Set(banEntry{
		AuthID:      "xai-mixed.json",
		Provider:    "xai",
		ErrorCode:   unauthorizedErrorCode,
		BannedAt:    now.Add(-time.Hour),
		ResetAt:     now.AddDate(100, 0, 0),
		ResetSource: "manual_unban",
		CpaSynced:   true,
	})
	store.Set(banEntry{
		AuthID:      "xai-mixed.json",
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    now,
		ResetAt:     now.Add(24 * time.Hour),
		ResetSource: "local_plus_fallback",
		CpaSynced:   false,
	})
	got, ok := store.Get("xai-mixed.json")
	if !ok {
		t.Fatal("missing ban")
	}
	if got.ErrorCode != exhaustedErrorCode {
		t.Fatalf("error_code=%q want exhausted after true 429 supersedes prior 401", got.ErrorCode)
	}
	if got.ResetSource != "local_plus_fallback" {
		t.Fatalf("reset_source=%q", got.ResetSource)
	}
	if !got.ResetAt.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("reset_at=%s want 24h quota window", got.ResetAt)
	}
}

func TestClassifyBareAuthenticationRequired401IsProbeError(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusUnauthorized,
		ChatError:  "Authentication required",
	})
	if got.Classification == "reauth" {
		t.Fatalf("bare Authentication required must not be reauth: %+v", got)
	}
	if got.Action == "delete" {
		t.Fatalf("bare Authentication required must not recommend delete: %+v", got)
	}
}


func TestClassifyNon401RefreshTokenTextIsNotReauth(t *testing.T) {
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusInternalServerError,
		ChatError:  "failed to refresh token: connection reset",
	})
	if got.Classification == "reauth" || got.Action == "delete" {
		t.Fatalf("non-401 refresh-token transport error must not be reauth/delete: %+v", got)
	}
}

func TestClassifyAccessDeniedOn403IsNotReauthViaStrongHelper(t *testing.T) {
	// Strong-helper must not fire on non-401 even if message contains broad auth-ish words.
	got := classifyProbe(classifyInput{
		Lang:       LangZH,
		ChatStatus: http.StatusForbidden,
		ChatCode:   "permission-denied",
		ChatError:  "Access to the chat endpoint is denied. Please ensure you're using the correct credentials.",
	})
	if got.Classification == "reauth" {
		t.Fatalf("403 permission text must not become reauth: %+v", got)
	}
}

func TestDetectBanRejectsBroadAccessDeniedWithoutCredentialCode(t *testing.T) {
	record := pluginapi.UsageRecord{
		Provider: "xai",
		AuthID:   "xai-broad-401.json",
		Failed:   true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 401,
			Body:       `{"error":"access denied by upstream proxy"}`,
		},
	}
	if _, ok := detectBan(record, defaultPluginConfig(), time.Now()); ok {
		t.Fatal("broad access denied without credential semantics must not autoban")
	}
}
