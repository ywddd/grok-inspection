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
	AuthID      string    `json:"auth_id"`
	Provider    string    `json:"provider"`
	ErrorCode   string    `json:"error_code"`
	BannedAt    time.Time `json:"banned_at"`
	ResetAt     time.Time `json:"reset_at"`
	ResetSource string    `json:"reset_source"`
	TraceID     string    `json:"trace_id,omitempty"`
	// CpaSynced is true after a successful CPA disable/enable PATCH.
	// False means local ban is active for scheduling but CPA may still be enabled.
	CpaSynced bool `json:"cpa_synced"`
	// Revision is a store-local CAS token; restore deletes only matching revision.
	Revision uint64 `json:"revision,omitempty"`
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
	if status == http.StatusUnauthorized {
		if !hasCode {
			errorCode = unauthorizedErrorCode
		}
	} else if !hasCode {
		return banEntry{}, false
	}

	resetAt, resetSource, ok := resolveBanWindow(status, errorCode, record.ResponseHeaders, now, cfg)
	if !ok {
		return banEntry{}, false
	}

	return banEntry{
		AuthID:      authID,
		Provider:    provider,
		ErrorCode:   errorCode,
		BannedAt:    now,
		ResetAt:     resetAt,
		ResetSource: resetSource,
		TraceID:     firstHeader(record.ResponseHeaders, "X-Request-Id"),
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
	if resetAt, ok := absoluteResetTime(headers); ok && resetAt.After(now) {
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

func absoluteResetTime(headers http.Header) (time.Time, bool) {
	for _, name := range []string{"X-RateLimit-Reset-At", "X-Reset-At", "X-Grok-Reset-At"} {
		value := firstHeader(headers, name)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed, true
		}
		if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
			return time.Unix(unix, 0), true
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
