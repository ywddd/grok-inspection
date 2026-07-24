package main

import (
	"grok-inspection/cpasdk/pluginapi"
	"net/http"
	"strings"
	"time"
)

// syncManualInspectionBan records an inspection-triggered manual disable in
// the existing auto-ban pool. It never creates a second source of ban state.
func syncManualInspectionBan(store *banStore, target *pluginapi.HostAuthFileEntry, fallback string, now time.Time) bool {
	return syncInspectionBan(store, target, fallback, now, manualInspectionBanErrorCode)
}

func syncInspectionBan(store *banStore, target *pluginapi.HostAuthFileEntry, fallback string, now time.Time, errorCode string) bool {
	if store == nil {
		return false
	}
	errorCode = strings.TrimSpace(errorCode)
	if errorCode == "" {
		errorCode = manualInspectionBanErrorCode
	}
	authID := strings.TrimSpace(fallback)
	provider := "xai"
	if target != nil {
		authID = firstNonEmpty(target.Name, target.ID, fallback, target.AuthIndex, target.Email)
		if normalized := normalizeProvider(target.Provider); normalized != "" {
			provider = normalized
		}
	}
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return false
	}

	// Collapse an older entry keyed by another alias before writing the canonical
	// file-name key, so the same account cannot appear twice in the shared pool.
	clearBansMatchingTargetInStore(store, target, fallback)
	store.Set(banEntry{
		AuthID:      authID,
		Provider:    provider,
		ErrorCode:   errorCode,
		BannedAt:    now,
		ResetAt:     now.AddDate(100, 0, 0),
		ResetSource: manualInspectionBanResetSource,
		CpaSynced:   true,
	})
	return true
}

// clearBansMatchingTarget drops autoban pool rows for the same account identity.
// AuthIDs in the ban store are typically file names; match AuthIndex/Name/ID/request aliases.
// Returns how many ban entries were removed. Caller decides when to persist ban state.
func clearBansMatchingTarget(target *pluginapi.HostAuthFileEntry, name string) int {
	return clearBansMatchingTargetInStore(activeStore, target, name)
}

func clearBansMatchingTargetInStore(store *banStore, target *pluginapi.HostAuthFileEntry, name string) int {
	if store == nil {
		return 0
	}
	aliases := map[string]struct{}{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" {
			aliases[v] = struct{}{}
		}
	}
	add(name)
	if target != nil {
		add(target.AuthIndex)
		add(target.Name)
		add(target.ID)
		add(target.Email)
		add(target.Label)
	}
	if len(aliases) == 0 {
		return 0
	}
	removed := 0
	for _, entry := range store.All() {
		id := strings.TrimSpace(entry.AuthID)
		if id == "" {
			continue
		}
		if _, ok := aliases[id]; ok {
			if store.Delete(id) {
				removed++
			}
			continue
		}
		// File base name without path segments sometimes used as ban key.
		base := id
		if i := strings.LastIndexAny(id, `/\`); i >= 0 {
			base = id[i+1:]
		}
		if _, ok := aliases[base]; ok {
			if store.Delete(id) {
				removed++
			}
		}
	}
	return removed
}

// listBansMatchingTarget returns current ban entries matching the account aliases.
func listBansMatchingTarget(store *banStore, target *pluginapi.HostAuthFileEntry, name string) []banEntry {
	if store == nil {
		return nil
	}
	aliases := banAliases(target, name)
	if len(aliases) == 0 {
		return nil
	}
	var out []banEntry
	for _, entry := range store.All() {
		if banIDMatchesAliases(entry.AuthID, aliases) {
			out = append(out, entry)
		}
	}
	return out
}

// clearBansMatchingTargetCAS deletes only bans whose revision still matches the
// pre-mutation snapshot. Newer revisions are kept and re-disabled in CPA.
// Must not be called while holding engine.mu (performs Management HTTP).
//
//	localBanRemains: at least one matching ban still in the store after CAS
//	cpaDisabledOK: every remaining ban was successfully re-disabled in CPA
//	               (true when no remaining bans either)
func clearBansMatchingTargetCAS(store *banStore, target *pluginapi.HostAuthFileEntry, name string, pre []banEntry) (removed int, localBanRemains bool, cpaDisabledOK bool, storeChanged bool, err error) {
	return clearBansMatchingTargetCASWithOrigin(store, target, name, pre, "", nil)
}

func clearBansMatchingTargetCASWithOrigin(store *banStore, target *pluginapi.HostAuthFileEntry, name string, pre []banEntry, password string, originHeaders http.Header) (removed int, localBanRemains bool, cpaDisabledOK bool, storeChanged bool, err error) {
	if store == nil {
		return 0, false, true, false, nil
	}
	preByID := map[string]uint64{}
	for _, e := range pre {
		preByID[e.AuthID] = e.Revision
	}
	var firstErr error
	reDisableOK := true
	for _, entry := range store.All() {
		if !banIDMatchesAliases(entry.AuthID, banAliases(target, name)) {
			continue
		}
		expected, inPre := preByID[entry.AuthID]
		if !inPre {
			// Not in the pre-enable snapshot: concurrent ban — re-disable.
			if errRD := redisableKeptBanWithOrigin(store, entry.AuthID, password, originHeaders); errRD != nil {
				reDisableOK = false
				if firstErr == nil {
					firstErr = errRD
				}
			}
			localBanRemains = true
			storeChanged = true
			continue
		}
		// Atomic: delete only if still expectedRev; otherwise get live newer entry.
		deleted, current, present := store.DeleteIfOrCurrent(entry.AuthID, expected)
		if deleted {
			removed++
			storeChanged = true
			continue
		}
		if !present {
			// Concurrently removed (e.g. another unban/restore) — nothing to re-disable.
			continue
		}
		// DeleteIf failed because revision moved: re-disable the live ban.
		_ = current
		if errRD := redisableKeptBanWithOrigin(store, entry.AuthID, password, originHeaders); errRD != nil {
			reDisableOK = false
			if firstErr == nil {
				firstErr = errRD
			}
		}
		localBanRemains = true
		storeChanged = true
	}
	if !localBanRemains {
		return removed, false, true, storeChanged, firstErr
	}
	return removed, true, reDisableOK && firstErr == nil, storeChanged, firstErr
}

// redisableKeptBanWithOrigin re-disables a retained ban in CPA and records sync state.
func redisableKeptBanWithOrigin(store *banStore, authID, password string, originHeaders http.Header) error {
	if store == nil {
		return nil
	}
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return nil
	}
	if errDisable := disableAuthInCPAWithOrigin(authID, password, originHeaders); errDisable != nil {
		store.UpdateCpaSyncState(authID, false, sanitizeCPASyncError(errDisable))
		return errDisable
	}
	store.UpdateCpaSyncState(authID, true, "")
	return nil
}
