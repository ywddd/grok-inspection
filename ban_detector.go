package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

const (
	exhaustedErrorCode             = "subscription:free-usage-exhausted"
	permissionDeniedErrorCode      = "permission-denied"
	spendingLimitErrorCode         = "personal-team-blocked:spending-limit"
	unauthorizedErrorCode          = "unauthorized"
	manualInspectionBanErrorCode   = "manual-disabled"
	manualInspectionBanResetSource = "manual_unban"
)

type banEntry struct {
	AuthID    string `json:"auth_id"`
	Provider  string `json:"provider"`
	ErrorCode string `json:"error_code"`
	// ErrorCodeDiag is optional body-level diagnostic code; category uses ErrorCode.
	ErrorCodeDiag string    `json:"error_code_diag,omitempty"`
	BannedAt      time.Time `json:"banned_at"`
	ResetAt       time.Time `json:"reset_at"`
	ResetSource   string    `json:"reset_source"`
	TraceID       string    `json:"trace_id,omitempty"`
	// CpaSynced is true after a successful CPA disable/enable PATCH.
	// False means local ban is active for scheduling but CPA may still be enabled.
	CpaSynced bool `json:"cpa_synced"`
	// Revision is a store-local CAS token; restore deletes only matching revision.
	Revision uint64 `json:"revision,omitempty"`
	// CpaSyncError is the last CPA disable/enable sync failure (never contains credentials).
	CpaSyncError string `json:"cpa_sync_error,omitempty"`
}

// isContentSafetyViolation reports request-level moderation/safety blocks.
// These must never trigger account auto-disable/delete recommendations.
func isContentSafetyViolation(code, message string) bool {
	blob := lower(code) + " " + lower(message)
	return containsAny(blob,
		"content violates usage guidelines",
		"violates usage guidelines",
		"safety_check",
		"safety check",
		"content safety",
		"content policy",
		"moderation",
		"policy violation",
		"policy_violation",
		"blocked by safety",
		"usage guidelines",
	)
}

// isAccountLevelPermissionDenied requires explicit account-level permission evidence.
// HTTP 403 + code=permission-denied alone is insufficient (WAF/safety/other 403s).
func isAccountLevelPermissionDenied(status int, code, message string) bool {
	// Account-level permission denial is an HTTP 403 phenomenon for classify/detect.
	// Never promote 500/200/etc. body text (credentials/suspended) into permission_denied.
	if status != http.StatusForbidden {
		return false
	}
	if isContentSafetyViolation(code, message) {
		return false
	}
	blob := lower(code) + " " + lower(message)
	// Explicit account-level signals observed on Grok/xAI and common suspensions.
	if containsAny(blob,
		"access to the chat endpoint is denied",
		"chat endpoint is denied",
		"using the correct credentials",
		"console.x.ai",
		"update the permissions",
		"contact support",
		"console permissions",
		"credentials",
		"deactivated",
		"suspended",
		"account has been banned",
		"account banned",
		"account is banned",
		"account suspended",
		"account deactivated",
	) {
		return true
	}
	// Bare permission-denied / generic forbidden is not enough.
	return false
}

func detectBan(record pluginapi.UsageRecord, cfg pluginConfig, now time.Time) (banEntry, bool) {
	provider := normalizeProvider(record.Provider)
	if provider != "xai" || !record.Failed {
		return banEntry{}, false
	}

	authID := strings.TrimSpace(record.AuthID)
	if authID == "" {
		return banEntry{}, false
	}

	status := record.Failure.StatusCode
	errorCode, hasCode := parseErrorCode(record.Failure.Body)
	msg := parseErrorMessage(record.Failure.Body)
	var diagCode string
	if status == http.StatusUnauthorized {
		// Visible error_code/category must always be unauthorized for any HTTP 401,
		// even when body code is permission-denied / spending-limit / etc.
		if hasCode {
			diagCode = errorCode
		}
		errorCode = unauthorizedErrorCode
	} else if !hasCode {
		return banEntry{}, false
	}

	// Account auto-ban on 403 requires account-level evidence, not bare permission-denied.
	if status == http.StatusForbidden {
		if errorCode != permissionDeniedErrorCode || !isAccountLevelPermissionDenied(status, errorCode, msg) {
			return banEntry{}, false
		}
	}

	resetAt, resetSource, ok := resolveBanWindow(status, errorCode, record.ResponseHeaders, now, cfg)
	if !ok {
		return banEntry{}, false
	}

	return banEntry{
		AuthID:        authID,
		Provider:      provider,
		ErrorCode:     errorCode,
		ErrorCodeDiag: diagCode,
		BannedAt:      now,
		ResetAt:       resetAt,
		ResetSource:   resetSource,
		TraceID:       firstHeader(record.ResponseHeaders, "X-Request-Id"),
	}, true
}

func resolveBanWindow(status int, errorCode string, headers http.Header, now time.Time, cfg pluginConfig) (time.Time, string, bool) {
	switch {
	case status == http.StatusTooManyRequests && errorCode == exhaustedErrorCode:
		fallback := time.Duration(cfg.FallbackHours) * time.Hour
		if fallback <= 0 {
			fallback = 24 * time.Hour
		}
		resetAt, resetSource := resolveResetAt(headers, now, fallback)
		return resetAt, resetSource, true
	case status == http.StatusForbidden && errorCode == permissionDeniedErrorCode:
		// Permission issues are not temporary quota windows. Keep the account out of
		// the pool until an operator unbans it manually.
		return now.AddDate(100, 0, 0), "manual_unban", true
	case status == http.StatusPaymentRequired && errorCode == spendingLimitErrorCode:
		// Spending-limit / subscription blocks do not expose a reliable cooldown.
		// Keep them disabled until an operator manually unbans the account.
		return now.AddDate(100, 0, 0), "manual_unban", true
	case status == http.StatusUnauthorized:
		// Auth failures usually mean expired or invalid credentials. Keep the account
		// out of the pool until an operator unbans it manually.
		return now.AddDate(100, 0, 0), "manual_unban", true
	default:
		return time.Time{}, "", false
	}
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "xai", "x-ai", "grok":
		return "xai"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func parseErrorCode(body string) (string, bool) {
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return "", false
	}
	payload.Code = strings.TrimSpace(payload.Code)
	return payload.Code, payload.Code != ""
}

func resolveResetAt(headers http.Header, now time.Time, fallback time.Duration) (time.Time, string) {
	if resetAt, ok := absoluteResetTime(headers, now); ok && resetAt.After(now) {
		return resetAt, "header_absolute"
	}
	if retryAfter, ok := retryAfterDuration(headers, now); ok {
		return now.Add(retryAfter), "header_relative"
	}
	if dateValue := firstHeader(headers, "Date"); dateValue != "" {
		if responseAt, err := http.ParseTime(dateValue); err == nil {
			return responseAt.Add(fallback), "date_plus_fallback"
		}
	}
	return now.Add(fallback), "local_plus_fallback"
}

func absoluteResetTime(headers http.Header, now time.Time) (time.Time, bool) {
	// Callers must supply the inspection/usage clock; do not sample wall time here.
	if now.IsZero() {
		return time.Time{}, false
	}
	maxFuture := now.AddDate(5, 0, 0)
	for _, name := range []string{"X-RateLimit-Reset-At", "X-Reset-At", "X-Grok-Reset-At"} {
		value := firstHeader(headers, name)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			if !parsed.After(now) || parsed.After(maxFuture) {
				continue
			}
			return parsed, true
		}
		if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
			parsed, ok := unixResetTime(unix, now)
			if ok {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func retryAfterDuration(headers http.Header, now time.Time) (time.Duration, bool) {
	value := firstHeader(headers, "Retry-After")
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second, true
	}
	if retryAt, err := http.ParseTime(value); err == nil && retryAt.After(now) {
		return retryAt.Sub(now), true
	}
	return 0, false
}

func firstHeader(headers http.Header, name string) string {
	return strings.TrimSpace(headers.Get(name))
}

// unixResetTime accepts Unix seconds or 13-digit milliseconds.
// Rejects absurd far-future and clearly invalid values so callers fall back.
func unixResetTime(unix int64, now time.Time) (time.Time, bool) {
	if unix <= 0 {
		return time.Time{}, false
	}
	var t time.Time
	// 13+ digit values are milliseconds (e.g. 1700000000000).
	if unix >= 1_000_000_000_000 {
		t = time.UnixMilli(unix)
	} else {
		t = time.Unix(unix, 0)
	}
	// Reject expired / equal timestamps; resolveResetAt also checks After(now).
	if !t.After(now) {
		return time.Time{}, false
	}
	// Reject absurdly far future (beyond 5 years) — likely unit confusion garbage.
	if t.After(now.AddDate(5, 0, 0)) {
		return time.Time{}, false
	}
	return t, true
}

func parseErrorMessage(body string) string {
	var payload struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	switch v := payload.Error.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if m, ok := v["message"].(string); ok {
			return strings.TrimSpace(m)
		}
		if m, ok := v["error"].(string); ok {
			return strings.TrimSpace(m)
		}
	}
	return strings.TrimSpace(payload.Message)
}
