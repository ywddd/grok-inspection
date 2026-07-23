package main

import (
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginabi"
	"grok-inspection/cpasdk/pluginapi"
)

var (
	activeStore = newBanStore()

	restoreOnce   sync.Once
	restoreStopMu sync.Mutex
	restoreStop   chan struct{}
	restoreDone   chan struct{}
	// restoreInterval controls background cooldown recovery. Overridable in tests.
	restoreInterval = 30 * time.Second
)

func handleUsage(raw []byte) ([]byte, error) {
	var record pluginapi.UsageRecord
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &record); err != nil {
			return okEnvelope(map[string]any{})
		}
	}
	if _, err := handleUsageRecord(record, loadedConfig(), time.Now()); err != nil {
		return nil, err
	}
	return okEnvelope(map[string]any{})
}

func handleUsageRecord(record pluginapi.UsageRecord, cfg pluginConfig, now time.Time) (banEntry, error) {
	if !cfg.Enabled {
		return banEntry{}, nil
	}

	entry, ok := detectBan(record, cfg, now)
	if !ok {
		return banEntry{}, nil
	}
	// Keep local ban first so restore/UI exclude the account even if CPA PATCH fails.
	// Must return quickly: CPA Usage Manager is single-threaded; no inline PATCH/disk.
	entry.CpaSynced = false
	entry.CpaSyncError = ""
	activeStore.Set(entry)
	entry, _ = activeStore.Get(entry.AuthID)
	if cfg.LogMatches {
		slog.Info("grok-inspection: autoban match",
			"auth_id", entry.AuthID,
			"error_code", entry.ErrorCode,
			"reset_source", entry.ResetSource,
			"reset_at", entry.ResetAt.Format(time.RFC3339),
		)
	}
	// Bounded background queue; on full, leave unsynced for restore/retry (do not pretend success).
	if !enqueueBanDispose(entry.AuthID, entry.Revision) {
		slog.Warn("grok-inspection: ban dispose queue full; left unsynced",
			"auth_id", entry.AuthID,
		)
		activeStore.UpdateCpaSyncState(entry.AuthID, false, "dispose queue full")
	}
	// Coalesced dirty persist: never spawn unbounded Save goroutines from usage path.
	markBanStoreDirty()
	entry, _ = activeStore.Get(entry.AuthID)
	return entry, nil
}

// restoreExpiredBans re-enables free-usage bans whose reset_at has passed.
// Manual bans (reset_source=manual_unban) are never auto-restored.
// Failed enables keep the ban entry so the next tick can retry.
// Missing auth files are dropped so pending_restore cannot stuck forever.
func restoreExpiredBans(store *banStore, now time.Time) (restored, failed int) {
	dirty := false
	// Retry CPA disable for still-active bans that only exist locally.
	// Skip expired entries so the enable path can restore them without a disable race.
	for _, entry := range store.UnsyncedCPA() {
		if !entry.ResetAt.After(now) {
			continue
		}
		expectedRev := entry.Revision
		var errDisable error
		_ = withAuthOp(entry.AuthID, func() error {
			errDisable = disableAuthInCPA(entry.AuthID)
			return nil
		})
		if errDisable != nil {
			// Auth already gone from CPA: drop local ban (incl. manual 401/403) so we do not retry forever. Other errors keep the entry for the next tick.
			if isAuthFileNotFoundError(errDisable) {
				if store.DeleteIf(entry.AuthID, expectedRev) {
					dirty = true
					slog.Info("grok-inspection: dropped ban for missing auth during disable retry",
						"auth_id", entry.AuthID,
						"error_code", entry.ErrorCode,
					)
				}
				continue
			}
			slog.Warn("grok-inspection: retry disable auth in CPA failed", "auth_id", entry.AuthID, "error", errDisable)
			store.UpdateCpaSyncState(entry.AuthID, false, sanitizeCPASyncError(errDisable))
			dirty = true
			continue
		}
		store.UpdateCpaSyncState(entry.AuthID, true, "")
		dirty = true
	}

	for _, authID := range store.Expired(now) {
		entry, ok := store.Get(authID)
		if !ok {
			continue
		}
		// Permanent / manual bans: leave until operator unbans.
		if entry.ResetSource == "manual_unban" {
			continue
		}
		expectedRev := entry.Revision
		var enabled bool
		var errEnable error
		var keptNewer bool
		_ = withAuthOp(authID, func() error {
			enabled, errEnable = enableAuthInCPAAllowMissing(authID, "")
			if errEnable != nil {
				return errEnable
			}
			// Atomic delete-or-observe after enable: if a newer ban landed during
			// the enable call, DeleteIfOrCurrent returns it and we re-disable.
			deleted, current, present := store.DeleteIfOrCurrent(authID, expectedRev)
			if deleted {
				restored++
				if enabled {
					slog.Info("grok-inspection: re-enabled expired ban",
						"auth_id", authID,
						"error_code", entry.ErrorCode,
						"reset_source", entry.ResetSource,
					)
				} else {
					slog.Info("grok-inspection: dropped ban for missing auth",
						"auth_id", authID,
						"error_code", entry.ErrorCode,
					)
				}
				return nil
			}
			if !present {
				return nil
			}
			_ = current
			keptNewer = true
			if errDisable := disableAuthInCPA(authID); errDisable != nil {
				store.UpdateCpaSyncState(authID, false, sanitizeCPASyncError(errDisable))
				slog.Warn("grok-inspection: re-disable after concurrent ban failed", "auth_id", authID, "error", errDisable)
			} else {
				store.UpdateCpaSyncState(authID, true, "")
			}
			dirty = true
			return nil
		})
		if errEnable != nil {
			failed++
			slog.Warn("grok-inspection: failed to re-enable expired auth in CPA",
				"auth_id", authID,
				"error", errEnable,
				"reset_at", entry.ResetAt.Format(time.RFC3339),
			)
			continue
		}
		if keptNewer {
			continue
		}
	}
	if restored > 0 {
		dirty = true
	}
	if dirty {
		markBanStoreDirty()
		if err := flushBanPersistWorker(); err != nil {
			slog.Warn("grok-inspection: failed to save ban state after restore", "error", err)
		}
	}
	return restored, failed
}

func handlePluginMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodUsageHandle:
		return handleUsage(request)
	default:
		return nil, nil
	}
}

func saveActiveStore() {
	_ = saveActiveStoreErr()
}

func saveActiveStoreErr() error {
	cfg := loadedConfig()
	if !(cfg.PersistState && cfg.StateFile != "") {
		return nil
	}
	return activeStore.Save(cfg.StateFile)
}

// startBanRestoreLoop runs a background timer so free-usage bans recover
// without any scheduler capability (CPA built-in routing only).
func startBanRestoreLoop() {
	restoreOnce.Do(func() {
		interval := restoreInterval
		if interval <= 0 {
			interval = 30 * time.Second
		}
		restoreStop = make(chan struct{})
		restoreDone = make(chan struct{})
		go func() {
			defer close(restoreDone)
			// First pass shortly after load (covers expired entries kept on disk).
			select {
			case <-time.After(2 * time.Second):
			case <-restoreStop:
				return
			}
			_, _ = restoreExpiredBans(activeStore, time.Now())
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					// Cooldown restore is independent of autoban_enabled so timed
					// free-usage bans still recover when new auto-bans are paused.
					_, _ = restoreExpiredBans(activeStore, time.Now())
				case <-restoreStop:
					return
				}
			}
		}()
	})
}

func stopBanRestoreLoop() {
	restoreStopMu.Lock()
	defer restoreStopMu.Unlock()
	if restoreStop == nil {
		return
	}
	select {
	case <-restoreStop:
		// already closed
	default:
		close(restoreStop)
	}
	waitRestoreLoopDone(restoreDone)
}

// waitRestoreLoopDone blocks until the restore worker signals completion.
// Extracted for tests so we do not touch the process-global Once/WaitGroup.
func waitRestoreLoopDone(done <-chan struct{}) {
	if done == nil {
		return
	}
	<-done
}

func banCategoryOf(errorCode string) string {
	c := strings.ToLower(strings.TrimSpace(errorCode))
	switch {
	case c == exhaustedErrorCode || strings.Contains(c, "free-usage-exhausted"):
		return "quota"
	case c == permissionDeniedErrorCode || strings.Contains(c, "permission-denied"):
		return "permission"
	case c == spendingLimitErrorCode || strings.Contains(c, "spending-limit"):
		return "spending_limit"
	case c == unauthorizedErrorCode || c == "401" || strings.Contains(c, "unauthorized") ||
		strings.Contains(c, "authentication_error") || strings.Contains(c, "invalid_token") ||
		strings.Contains(c, "token_expired") || strings.Contains(c, "token is expired") ||
		strings.Contains(c, "invalid_grant") || c == "unauthenticated":
		// Any HTTP 401 body codes still map to the 401 auth-failed category.
		return "unauthorized"
	case c == manualInspectionBanErrorCode || strings.Contains(c, "manual-disabled"):
		return "manual"
	default:
		return "other"
	}
}

func banStatus() map[string]any {
	now := time.Now()
	// Include expired pending-restore entries so operators can see/act on them.
	items := activeStore.All()
	out := make([]map[string]any, 0, len(items))
	quota := 0
	permission := 0
	spendingLimit := 0
	unauthorized := 0
	manualDisabled := 0
	other := 0
	pending := 0
	for _, entry := range items {
		cat := banCategoryOf(entry.ErrorCode)
		isPending := !entry.ResetAt.After(now) && entry.ResetSource != "manual_unban"
		if isPending {
			pending++
			// Keep category for filter chips; UI can label pending separately.
		}
		switch cat {
		case "quota":
			quota++
		case "permission":
			permission++
		case "spending_limit":
			spendingLimit++
		case "unauthorized":
			unauthorized++
		case "manual":
			manualDisabled++
		default:
			other++
		}
		remain := int64(entry.ResetAt.Sub(now).Seconds())
		if remain < 0 {
			remain = 0
		}
		out = append(out, map[string]any{
			"auth_id":           entry.AuthID,
			"provider":          entry.Provider,
			"error_code":        entry.ErrorCode,
			"category":          cat,
			"banned_at":         entry.BannedAt.Format(time.RFC3339),
			"reset_at":          entry.ResetAt.Format(time.RFC3339),
			"reset_source":      entry.ResetSource,
			"trace_id":          entry.TraceID,
			"cpa_synced":        entry.CpaSynced,
			"cpa_sync_error":    entry.CpaSyncError,
			"pending_restore":   isPending,
			"remaining_seconds": remain,
		})
	}
	// count matches the visible ban pool (including pending_restore rows).
	// pending_restore remains a separate field for the restore chip.
	cfg := loadedConfig()
	return map[string]any{
		"plugin":                pluginName,
		"enabled":               cfg.Enabled,
		"fallback_hours":        cfg.FallbackHours,
		"persist_state":         cfg.PersistState,
		"state_file":            cfg.StateFile,
		"log_matches":           cfg.LogMatches,
		"count":                 len(out),
		"quota_count":           quota,
		"permission_count":      permission,
		"spending_limit_count":  spendingLimit,
		"unauthorized_count":    unauthorized,
		"manual_disabled_count": manualDisabled,
		"other_count":           other,
		// manual_count kept for older UI clients: permission + unauthorized
		"manual_count":    permission + unauthorized,
		"pending_restore": pending,
		"unsynced_count":  len(activeStore.UnsyncedCPA()),
		"bans":            out,
	}
}
