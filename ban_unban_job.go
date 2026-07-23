package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Async bulk unban job (POST /unban-all). Avoids blocking Management on large ban pools.
type unbanJobState struct {
	mu           sync.Mutex
	runID        uint64
	running      bool
	stopped      bool
	done         int
	total        int
	enabled      int
	missing      int
	failed       int
	current      string
	failures     []string
	persistError string
	startedAt    time.Time
	finished     time.Time
	wg           sync.WaitGroup
}

var unbanJob = &unbanJobState{}

func unbanJobStatus() map[string]any {
	unbanJob.mu.Lock()
	defer unbanJob.mu.Unlock()
	out := map[string]any{
		"running":        unbanJob.running,
		"stopped":        unbanJob.stopped && !unbanJob.running,
		"done":           unbanJob.done,
		"total":          unbanJob.total,
		"enabled":        unbanJob.enabled,
		"missing":        unbanJob.missing,
		"failed":         unbanJob.failed,
		"current":        unbanJob.current,
		"failures":       append([]string(nil), unbanJob.failures...),
		"persist_error":  unbanJob.persistError,
	}
	if !unbanJob.startedAt.IsZero() {
		out["started_at"] = unbanJob.startedAt.Format(time.RFC3339)
	}
	if !unbanJob.finished.IsZero() {
		out["finished_at"] = unbanJob.finished.Format(time.RFC3339)
	}
	return out
}

// stopUnbanJob requests cancel. The worker stays busy (running=true) until
// in-flight CPA calls finish, so a new job cannot overlap the old one.
func stopUnbanJob() {
	unbanJob.mu.Lock()
	if !unbanJob.running {
		unbanJob.mu.Unlock()
		return
	}
	unbanJob.stopped = true
	unbanJob.runID++
	unbanJob.current = "stopping"
	unbanJob.mu.Unlock()
}

func (j *unbanJobState) isActive(runID uint64) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.running && j.runID == runID && !j.stopped
}

// claimUnbanSlot atomically claims the unban busy slot under
// engine.mu -> unbanJob.mu. Async jobs join wg before the locks are released,
// so shutdown cannot start waiting in the gap before Add.
// Caller must releaseUnbanSlot when finished.
func claimUnbanSlot(total int, current string, async bool) (runID uint64, err error) {
	engine.mu.Lock()
	if engine.running || engine.applying || engine.applyDraining || engine.actionInFlight > 0 {
		engine.mu.Unlock()
		return 0, fmt.Errorf("busy")
	}
	unbanJob.mu.Lock()
	if unbanJob.running {
		unbanJob.mu.Unlock()
		engine.mu.Unlock()
		return 0, fmt.Errorf("busy: unban already running")
	}
	unbanJob.runID++
	runID = unbanJob.runID
	unbanJob.running = true
	unbanJob.stopped = false
	unbanJob.done = 0
	unbanJob.total = total
	unbanJob.enabled = 0
	unbanJob.missing = 0
	unbanJob.failed = 0
	unbanJob.current = current
	unbanJob.failures = nil
	unbanJob.persistError = ""
	unbanJob.startedAt = time.Now()
	unbanJob.finished = time.Time{}
	if async {
		unbanJob.wg.Add(1)
	}
	unbanJob.mu.Unlock()
	engine.mu.Unlock()
	return runID, nil
}

// releaseUnbanSlot ends the busy/draining state for a claimed unban worker.
// Safe after stop() which may have bumped runID while this worker still ran.
func releaseUnbanSlot(runID uint64) {
	_ = runID
	unbanJob.mu.Lock()
	unbanJob.running = false
	unbanJob.finished = time.Now()
	if unbanJob.current == "stopping" || unbanJob.stopped {
		unbanJob.current = "stopped"
	}
	unbanJob.mu.Unlock()
}

// commitUnbanAfterEnable deletes the ban only when the store still holds the
// same revision that this unban targeted. A newer concurrent ban is kept and
// CPA is re-disabled so local/CPA state stay aligned.
func commitUnbanAfterEnable(authID string, expectedRev uint64, hadEntry bool) (removed bool) {
	if !hadEntry {
		return false
	}
	if current, still := activeStore.Get(authID); still {
		if current.Revision == expectedRev {
			return activeStore.DeleteIf(authID, expectedRev)
		}
		if current.Revision > expectedRev {
			if errDisable := disableAuthInCPA(authID); errDisable != nil {
				activeStore.UpdateCpaSynced(authID, false)
			} else {
				activeStore.UpdateCpaSynced(authID, true)
			}
			return false
		}
	}
	return false
}

// unbanOneAccount enables one auth in CPA and drops the matching ban revision.
// Claims the unban slot for the entire host call so inspection/bulk ops cannot
// start mid-flight.
func unbanOneAccount(authID, password string) (enabled bool, removed bool, err error) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return false, false, fmt.Errorf("missing_auth_id")
	}
	runID, errClaim := claimUnbanSlot(1, authID, false)
	if errClaim != nil {
		return false, false, errClaim
	}
	defer releaseUnbanSlot(runID)

	entry, hadEntry := activeStore.Get(authID)
	expectedRev := entry.Revision
	var errEnable error
	_ = withAuthOp(authID, func() error {
		var en bool
		en, errEnable = enableAuthInCPAAllowMissing(authID, password)
		if errEnable != nil {
			return errEnable
		}
		enabled = en
		removed = commitUnbanAfterEnable(authID, expectedRev, hadEntry)
		return nil
	})
	if errEnable != nil {
		unbanJob.mu.Lock()
		if unbanJob.runID == runID {
			unbanJob.failed++
			unbanJob.done++
			if len(unbanJob.failures) < 20 {
				unbanJob.failures = append(unbanJob.failures, authID+": "+errEnable.Error())
			}
		}
		unbanJob.mu.Unlock()
		return false, false, errEnable
	}
	unbanJob.mu.Lock()
	if unbanJob.runID == runID {
		if enabled {
			unbanJob.enabled++
		} else {
			unbanJob.missing++
		}
		unbanJob.done++
	}
	unbanJob.mu.Unlock()
	if err := saveActiveStoreErr(); err != nil {
		return enabled, removed, fmt.Errorf("unbanned in CPA but failed to persist ban state: %w", err)
	}
	return enabled, removed, nil
}

// startUnbanJob unbans selected accounts asynchronously.
// authIDs take priority; otherwise category filters the ban pool; empty = all.
func startUnbanJob(authIDs []string, category, password string) error {
	category = strings.ToLower(strings.TrimSpace(category))
	wanted := make(map[string]struct{})
	for _, id := range authIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = struct{}{}
		}
	}

	type unbanTarget struct {
		authID string
		rev    uint64
	}
	targets := make([]unbanTarget, 0)
	for _, entry := range activeStore.All() {
		id := strings.TrimSpace(entry.AuthID)
		if id == "" {
			continue
		}
		if len(wanted) > 0 {
			if _, ok := wanted[id]; !ok {
				continue
			}
		} else if category != "" && category != "all" {
			if banCategoryOf(entry.ErrorCode) != category {
				continue
			}
		}
		targets = append(targets, unbanTarget{authID: id, rev: entry.Revision})
	}
	if len(targets) == 0 {
		return fmt.Errorf("no accounts to unban")
	}

	runID, errClaim := claimUnbanSlot(len(targets), "", true)
	if errClaim != nil {
		return errClaim
	}
	password = strings.TrimSpace(password)

	go func() {
		defer unbanJob.wg.Done()
		defer releaseUnbanSlot(runID)
		defer func() {
			if err := saveActiveStoreErr(); err != nil {
				unbanJob.mu.Lock()
				unbanJob.persistError = err.Error()
				// Surface save failure as a failed tally so UI does not look fully successful.
				unbanJob.failed++
				if len(unbanJob.failures) < 20 {
					unbanJob.failures = append(unbanJob.failures, "persist ban state: "+err.Error())
				}
				unbanJob.mu.Unlock()
			}
		}()

		for _, target := range targets {
			if !unbanJob.isActive(runID) {
				return
			}
			unbanJob.mu.Lock()
			unbanJob.current = target.authID
			unbanJob.mu.Unlock()

			var enabled bool
			var errEnable error
			_ = withAuthOp(target.authID, func() error {
				enabled, errEnable = enableAuthInCPAAllowMissing(target.authID, password)
				if errEnable != nil {
					return errEnable
				}
				_ = commitUnbanAfterEnable(target.authID, target.rev, true)
				return nil
			})
			unbanJob.mu.Lock()
			sameJob := unbanJob.runID == runID
			if errEnable != nil {
				if sameJob {
					unbanJob.failed++
					if len(unbanJob.failures) < 20 {
						unbanJob.failures = append(unbanJob.failures, target.authID+": "+errEnable.Error())
					}
					unbanJob.done++
				}
			} else if sameJob {
				if enabled {
					unbanJob.enabled++
				} else {
					unbanJob.missing++
				}
				unbanJob.done++
			}
			unbanJob.mu.Unlock()
			if !sameJob {
				return
			}
		}
	}()
	return nil
}
