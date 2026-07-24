package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// setAuthDisabled toggles CPA auth via Management API PATCH /auth-files/status.
// host.auth.save alone is NOT enough: CLIProxyAPI buildAuthFromFileData does not
// promote JSON "disabled" onto Auth.Disabled, so the main UI stays enabled.
// Must run outside management.handle (background goroutine) to avoid re-entry deadlock.
//
// After a successful PATCH we trust CPA and update local results only — we do not
// re-list all auth files (that was the main reason single-row ops felt slow).
// persist=false is used by bulk apply so 1000 disk flushes do not dominate runtime.
func setAuthDisabled(name string, disabled bool, password string, headers http.Header, persist bool) error {
	return setAuthDisabledWithBanReason(name, disabled, password, headers, persist, manualInspectionBanErrorCode)
}

func setAuthDisabledWithBanReason(name string, disabled bool, password string, headers http.Header, persist bool, banErrorCode string) error {
	resolvedPassword := strings.TrimSpace(password)
	if resolvedPassword == "" {
		resolvedPassword = resolveManagementPassword(headers)
	}
	originHeaders := managementOriginOnlyHeaders(headers)
	target, errTarget := findAuthFile(name)
	if errTarget != nil {
		return errTarget
	}
	fileName := firstNonEmpty(target.Name, target.ID)
	if strings.TrimSpace(fileName) == "" {
		return fmt.Errorf("auth file name missing for %s", name)
	}
	body, errMarshal := json.Marshal(map[string]any{
		"name":     fileName,
		"disabled": disabled,
	})
	if errMarshal != nil {
		return errMarshal
	}
	// Serialize per-account network mutation + local ban commit against Usage/restore.
	authKey := firstNonEmpty(fileName, name)
	return withAuthOp(authKey, func() error {
		// Snapshot matching ban revisions before enable network call (CAS later).
		var pre []banEntry
		if !disabled {
			pre = listBansMatchingTarget(activeStore, target, name)
		}
		if _, _, errPatch := callCPAManagementWithAuth(http.MethodPatch, "/v0/management/auth-files/status", body, resolvedPassword, headers); errPatch != nil {
			return errPatch
		}

		banStateChanged := false
		resultDisabled := disabled
		var errCAS error
		if disabled {
			// Local ban + result update under engine.mu only (no Management HTTP).
			engine.mu.Lock()
			for i := range engine.results {
				item := &engine.results[i]
				if resultMatchesTarget(*item, target, name) {
					item.Disabled = true
					item.Action = recommendAction(item.Classification, item.Disabled)
				}
			}
			engine.bumpResultsLocked()
			banStateChanged = syncInspectionBan(activeStore, target, name, time.Now(), banErrorCode)
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
		} else {
			// CAS + optional re-disable MUST NOT hold engine.mu (Management HTTP).
			var removed int
			var localBanRemains bool
			var cpaDisabledOK bool
			var storeChanged bool
			removed, localBanRemains, cpaDisabledOK, storeChanged, errCAS = clearBansMatchingTargetCASWithOrigin(activeStore, target, name, pre, resolvedPassword, originHeaders)
			_ = removed
			banStateChanged = storeChanged
			// Only mark the row disabled when a local ban remains AND CPA re-disable
			// succeeded. That is NOT enable success: return a conflict sentinel so
			// single-row/bulk UI count a failure. If re-disable failed, keep the
			// real error, Disabled=false, and recommend disable.
			if localBanRemains {
				if cpaDisabledOK && errCAS == nil {
					resultDisabled = true
					errCAS = errBanSupersededByNewerRevision
				} else {
					resultDisabled = false
					// keep errCAS (redisable HTTP / sync failure)
				}
			} else {
				resultDisabled = false
			}
			engine.mu.Lock()
			for i := range engine.results {
				item := &engine.results[i]
				if resultMatchesTarget(*item, target, name) {
					item.Disabled = resultDisabled
					item.Action = recommendAction(item.Classification, item.Disabled)
				}
			}
			engine.bumpResultsLocked()
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
		}
		if banStateChanged && persist {
			// Single-account path saves synchronously for immediate status. Only
			// enqueue a background retry when that save fails; otherwise the
			// duplicate worker write can outlive the operation.
			if err := saveActiveStoreErr(); err != nil {
				markBanStoreDirty()
				return fmt.Errorf("updated in CPA but failed to persist ban state: %w", err)
			}
		}
		return errCAS
	})
}
