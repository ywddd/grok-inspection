package main

import (
	"strings"
	"time"
)

// scheduledQuotaExhaustedTargets selects exact free-usage 429 rows for auto-disable.
// Bare 429 / text-only / substring codes are intentionally excluded.
func scheduledQuotaExhaustedTargets(action string, scope map[string]struct{}) []string {
	return scheduledTargetsExact(429, "quota_exhausted", exhaustedErrorCode, action, scope)
}

// scheduledUnauthorizedTargets selects exact unauthorized 401 reauth rows.
// Scheduled disposal always disables (manual restore only); never deletes here.
func scheduledUnauthorizedTargets(action string, scope map[string]struct{}) []string {
	return scheduledTargetsExact(401, "reauth", unauthorizedErrorCode, action, scope)
}

// scheduledHealthyRecoverTargets returns Disabled+healthy accounts from this run
// whose exact free-usage cooldowns have all expired. A transient healthy probe
// must never bypass an active quota cooldown.
func scheduledHealthyRecoverTargets(scope map[string]struct{}) []string {
	engine.mu.Lock()
	results := append([]accountResult(nil), engine.results...)
	engine.mu.Unlock()

	now := time.Now()
	targets := make([]string, 0)
	for _, item := range results {
		if scope != nil && !resultInProbedScope(scope, item) {
			continue
		}
		if !item.Disabled {
			continue
		}
		if strings.TrimSpace(item.Classification) != "healthy" {
			continue
		}
		if !accountHasExpiredExactQuotaExhaustionBans(item, now) {
			continue
		}
		if id := firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email); id != "" {
			targets = append(targets, id)
		}
	}
	return targets
}

func accountHasExpiredExactQuotaExhaustionBans(item accountResult, now time.Time) bool {
	bans := matchingBansForAccountResult(item)
	if len(bans) == 0 {
		return false
	}
	for _, entry := range bans {
		if strings.TrimSpace(entry.ErrorCode) != exhaustedErrorCode ||
			entry.ResetAt.IsZero() || entry.ResetAt.After(now) {
			return false
		}
	}
	return true
}

// accountHasExactQuotaExhaustionBan is the recover gate: at least one matching ban
// exists, and every matching ban is exact free-usage-exhausted (no 401/402/403/manual mix).
func accountHasExactQuotaExhaustionBan(item accountResult) bool {
	return accountHasOnlyExactQuotaExhaustionBans(item)
}

// accountHasOnlyExactQuotaExhaustionBans reports true only when every ban row that
// matches this account identity is exact free-usage-exhausted. Mixed alias reasons
// (exhausted + unauthorized/402/403/manual) are never auto-recoverable.
func accountHasOnlyExactQuotaExhaustionBans(item accountResult) bool {
	bans := matchingBansForAccountResult(item)
	if len(bans) == 0 {
		return false
	}
	for _, entry := range bans {
		if strings.TrimSpace(entry.ErrorCode) != exhaustedErrorCode {
			return false
		}
	}
	return true
}

// accountHasAnyMatchingBan reports whether any autoban row still matches the account.
// Used by scheduled enable success counting; must not run under engine.mu callers that
// also take store locks in the opposite order — callers snapshot results first.
func accountHasAnyMatchingBan(item accountResult) bool {
	return len(matchingBansForAccountResult(item)) > 0
}

func matchingBansForAccountResult(item accountResult) []banEntry {
	target := hostAuthFromAccountResult(&item)
	fallback := firstNonEmpty(item.FileName, item.AuthIndex, item.Name, item.Email)
	return listBansMatchingTarget(activeStore, target, fallback)
}
