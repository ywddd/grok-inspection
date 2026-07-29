package main

import (
	"fmt"
	"net/http"
	"strings"

	"grok-inspection/cpasdk/pluginapi"
)

// validBanDeleteCategories are explicit category selectors accepted by /ban-delete.
// Empty body / missing selector must not default to "delete everything".
var validBanDeleteCategories = map[string]struct{}{
	"all": {}, "quota": {}, "spending_limit": {}, "permission": {}, "unauthorized": {}, "manual": {},
}

// normalizeBanDeleteSelection requires an explicit non-empty auth_ids list and/or a
// known category (including "all"). Empty selection rejects before any CPA DELETE.
func normalizeBanDeleteSelection(authIDs []string, category string) (wanted map[string]struct{}, cat string, err error) {
	cat = strings.ToLower(strings.TrimSpace(category))
	wanted = make(map[string]struct{})
	for _, id := range authIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) > 0 {
		// Explicit ids win; category is optional and unused for filtering here
		// (same as historical unban-all auth_ids priority).
		return wanted, cat, nil
	}
	if cat == "" {
		return nil, "", fmt.Errorf("missing delete target: provide auth_ids or category")
	}
	if _, ok := validBanDeleteCategories[cat]; !ok {
		return nil, "", fmt.Errorf("invalid delete category")
	}
	return wanted, cat, nil
}

// startBanDeleteJob permanently deletes selected ban-pool accounts via CPA
// auth-files DELETE, reusing the shared unbanJob busy slot (mode=delete).
func startBanDeleteJob(authIDs []string, category, password string) error {
	return startBanDeleteJobWithOrigin(authIDs, category, password, nil)
}

// startBanDeleteJobWithOrigin snapshots a detached Host/Origin route context
// before the async worker starts so request header maps can be mutated after return.
//
// Important: after claimUnbanSlotWithMode(async=true) the worker goroutine must
// start immediately (wg.Add already done). Target resolution / host list stays
// inside the worker so Management /ban-delete returns without blocking.
func startBanDeleteJobWithOrigin(authIDs []string, category, password string, originHeaders http.Header) error {
	wanted, category, errSel := normalizeBanDeleteSelection(authIDs, category)
	if errSel != nil {
		return errSel
	}

	targets := make([]string, 0)
	for _, entry := range activeStore.All() {
		id := strings.TrimSpace(entry.AuthID)
		if id == "" {
			continue
		}
		if len(wanted) > 0 {
			if _, ok := wanted[id]; !ok {
				continue
			}
		} else if category != "all" {
			if banCategoryOf(entry.ErrorCode) != category {
				continue
			}
		}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return fmt.Errorf("no accounts to delete")
	}

	runID, errClaim := claimUnbanSlotWithMode(len(targets), "", true, "delete")
	if errClaim != nil {
		return errClaim
	}
	password = strings.TrimSpace(password)
	originHeaders = managementRouteHeaders(originHeaders)

	go func() {
		defer unbanJob.wg.Done()
		defer releaseUnbanSlot(runID)
		defer persistBanDeleteJobState(runID)

		// Resolve inside the worker (may block on host list once).
		resolved := buildBanDeleteAccountResults(targets)

		// Lookup failures: keep local ban/results, do not call CPA DELETE.
		items := make([]accountResult, 0, len(resolved))
		for _, r := range resolved {
			if r.skipDelete {
				unbanJob.mu.Lock()
				if unbanJob.ownsRunLocked(runID) {
					unbanJob.failed++
					unbanJob.done++
					if len(unbanJob.failures) < 20 {
						msg := r.item.AuthIndex + ": " + r.skipReason
						unbanJob.failures = append(unbanJob.failures, msg)
					}
				}
				unbanJob.mu.Unlock()
				continue
			}
			items = append(items, r.item)
		}

		for i := 0; i < len(items); i += deleteBatchSize {
			if !unbanJob.isActive(runID) {
				return
			}
			end := i + deleteBatchSize
			if end > len(items) {
				end = len(items)
			}
			chunk := items[i:end]
			current := ""
			if len(chunk) > 0 {
				current = firstNonEmpty(chunk[0].FileName, chunk[0].Name, chunk[0].AuthIndex)
				if len(chunk) > 1 {
					current = fmt.Sprintf("%s .. %s (%d-%d/%d)",
						current,
						firstNonEmpty(chunk[len(chunk)-1].FileName, chunk[len(chunk)-1].Name, chunk[len(chunk)-1].AuthIndex),
						i+1, end, len(items))
				}
			}
			unbanJob.mu.Lock()
			if unbanJob.ownsRunLocked(runID) {
				unbanJob.current = current
			}
			unbanJob.mu.Unlock()

			// Do not auto-clear locals here: fail-closed unmapped errors must keep
			// rows that are not positively confirmed. Clear only after per-item confirm.
			batchFails := deleteAuthFilesBatchWithClear(chunk, password, originHeaders, false, false)
			itemFailMsgs := banDeleteItemFailureMessages(chunk, batchFails)

			for i, item := range chunk {
				msg := ""
				if i < len(itemFailMsgs) {
					msg = itemFailMsgs[i]
				}
				if msg == "" {
					fn := resolveDeleteFileName(item)
					_ = clearOneDeletedAuthLocal(item, fn, nil, false)
				}
				unbanJob.mu.Lock()
				if unbanJob.ownsRunLocked(runID) {
					if msg != "" {
						unbanJob.failed++
						if len(unbanJob.failures) < 20 {
							unbanJob.failures = append(unbanJob.failures, msg)
						}
					} else {
						unbanJob.deleted++
					}
					unbanJob.done++
				}
				unbanJob.mu.Unlock()
			}
		}
	}()
	return nil
}

// persistBanDeleteJobState flushes inspection results + ban state after the job
// (or stop drain) and surfaces either failure on the shared job status.
// Results persistence uses persistSync()'s returned error so a concurrent newer
// persist cannot hide or invent this job's outcome via engine.persistError.
func persistBanDeleteJobState(runID uint64) {
	var parts []string
	if err := engine.persistSync(); err != nil {
		parts = append(parts, "persist results: "+err.Error())
	}
	if err := saveActiveStoreErr(); err != nil {
		parts = append(parts, "persist ban state: "+err.Error())
	}
	if len(parts) == 0 {
		return
	}
	msg := strings.Join(parts, "; ")
	unbanJob.mu.Lock()
	// defer order is persist then releaseUnbanSlot, so running is still true here.
	if unbanJob.ownsRunLocked(runID) {
		unbanJob.persistError = msg
		unbanJob.failed++
		if len(unbanJob.failures) < 20 {
			unbanJob.failures = append(unbanJob.failures, msg)
		}
	}
	unbanJob.mu.Unlock()
}

// banDeleteItemFailureMessages returns a per-chunk-item failure message.
// Empty string means the row is treated as deleted success.
//
// Fail-closed rules:
//   - failures that map to a physical file name mark every chunk row with that file;
//   - any non-empty failure that cannot be reliably mapped to a chunk row causes
//     every row that was not explicitly mapped as a named failure to also fail
//     (so unmapped CPA errors can never be counted as deleted success);
//   - rows with no resolvable file name always fail when any batch failure exists
//     or when their own local missing-name error is present.
func banDeleteItemFailureMessages(chunk []accountResult, batchFails []string) []string {
	out := make([]string, len(chunk))
	if len(chunk) == 0 {
		return out
	}
	failedByFile := map[string]string{}
	var unmapped []string
	for _, msg := range batchFails {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			continue
		}
		matchedFile := ""
		for _, item := range chunk {
			keys := []string{item.Name, item.FileName, item.AuthIndex, item.Email, resolveDeleteFileName(item)}
			for _, key := range keys {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				if strings.HasPrefix(msg, key+":") {
					matchedFile = resolveDeleteFileName(item)
					if matchedFile == "" {
						matchedFile = key
					}
					break
				}
			}
			if matchedFile != "" {
				break
			}
		}
		if matchedFile == "" {
			unmapped = append(unmapped, msg)
			continue
		}
		failedByFile[matchedFile] = msg
	}

	failClosed := len(unmapped) > 0
	batchFailMsg := ""
	if failClosed {
		batchFailMsg = unmapped[0]
		if len(unmapped) > 1 {
			batchFailMsg = fmt.Sprintf("%s (and %d more unmapped delete failures)", unmapped[0], len(unmapped)-1)
		}
	}

	for i, item := range chunk {
		id := firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email)
		fn := resolveDeleteFileName(item)
		if msg, ok := failedByFile[fn]; ok && fn != "" {
			out[i] = msg
			continue
		}
		// Also match when failedByFile was keyed by alias rather than resolveDeleteFileName.
		if fn != "" {
			if msg, ok := failedByFile[fn]; ok {
				out[i] = msg
				continue
			}
		}
		for _, key := range []string{item.Name, item.FileName, item.AuthIndex, item.Email, fn} {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if msg, ok := failedByFile[key]; ok {
				out[i] = msg
				break
			}
		}
		if out[i] != "" {
			continue
		}
		if failClosed {
			if id == "" {
				id = "unknown"
			}
			out[i] = id + ": " + batchFailMsg
			continue
		}
		if fn == "" {
			// No physical name: never treat as successful delete.
			if id == "" {
				id = "unknown"
			}
			out[i] = id + ": auth file name missing"
		}
	}
	return out
}

type banDeleteResolved struct {
	item       accountResult
	skipDelete bool
	skipReason string
}

// buildBanDeleteAccountResults maps ban AuthIDs to accountResult rows for
// deleteAuthFilesBatch. Resolution is fail-safe against cross-field identity
// collisions (e.g. A.AuthIndex == B.FileName): if more than one distinct row
// matches the token on any identity field, DELETE is skipped and the ban kept.
//
// Prefer inspection results; fall back to one host auth-list snapshot. Host list
// uses the same multi-match rejection. Only when no row matches anywhere may the
// AuthID be treated as a physical file name (manual-sync / legacy alias).
func buildBanDeleteAccountResults(authIDs []string) []banDeleteResolved {
	engine.mu.Lock()
	resultsSnap := append([]accountResult(nil), engine.results...)
	engine.mu.Unlock()

	var hostFiles []pluginapi.HostAuthFileEntry
	hostLoaded := false
	var hostErr error
	ensureHost := func() {
		if hostLoaded {
			return
		}
		hostLoaded = true
		list, err := callHostAuthListFn()
		if err != nil {
			hostErr = err
			return
		}
		hostFiles = list.Files
	}

	out := make([]banDeleteResolved, 0, len(authIDs))
	for _, id := range authIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		item, ambig, ok := resolveBanDeleteFromResults(resultsSnap, id)
		if ambig {
			out = append(out, banDeleteResolved{
				item:       accountResult{AuthIndex: id, Name: id, FileName: id},
				skipDelete: true,
				skipReason: "ambiguous auth identity (cross-field collision); refuse delete",
			})
			continue
		}
		if ok {
			out = append(out, banDeleteResolved{item: normalizeBanDeleteItem(item, id)})
			continue
		}
		ensureHost()
		if hostErr != nil {
			out = append(out, banDeleteResolved{
				item:       accountResult{AuthIndex: id, Name: id, FileName: id},
				skipDelete: true,
				skipReason: "auth lookup failed: " + hostErr.Error(),
			})
			continue
		}
		hostItem, hostAmbig, hostOK := resolveBanDeleteFromHostList(hostFiles, id)
		if hostAmbig {
			out = append(out, banDeleteResolved{
				item:       accountResult{AuthIndex: id, Name: id, FileName: id},
				skipDelete: true,
				skipReason: "ambiguous host auth identity (cross-field collision); refuse delete",
			})
			continue
		}
		if hostOK {
			out = append(out, banDeleteResolved{item: hostItem})
			continue
		}
		// Not found in results or host list: AuthID may already be the physical
		// file name from manual sync / legacy ban rows.
		out = append(out, banDeleteResolved{item: accountResult{
			AuthIndex: id,
			Name:      id,
			FileName:  id,
		}})
	}
	return out
}

func normalizeBanDeleteItem(item accountResult, id string) accountResult {
	if strings.TrimSpace(item.FileName) == "" {
		item.FileName = firstNonEmpty(item.Name, item.AuthIndex, id)
	}
	if strings.TrimSpace(item.Name) == "" {
		item.Name = firstNonEmpty(item.FileName, item.AuthIndex, id)
	}
	if strings.TrimSpace(item.AuthIndex) == "" {
		item.AuthIndex = id
	}
	return item
}

func accountResultMatchKey(item accountResult) string {
	return accountResultStableID(item)
}

// resolveBanDeleteFromResults returns a unique inspection row for ban AuthID.
// ambiguous=true when multiple distinct rows match the token on any identity field.
func resolveBanDeleteFromResults(results []accountResult, id string) (accountResult, bool, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return accountResult{}, false, false
	}
	matched := make([]accountResult, 0, 2)
	seen := map[string]struct{}{}
	for _, item := range results {
		if !accountResultMatchesBanToken(item, id) {
			continue
		}
		key := accountResultMatchKey(item)
		if key == "" {
			key = firstNonEmpty(item.AuthIndex, item.FileName, item.FileID, item.Name, item.Email, id)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		matched = append(matched, item)
		if len(matched) > 1 {
			return accountResult{}, true, false
		}
	}
	if len(matched) == 1 {
		return matched[0], false, true
	}
	return accountResult{}, false, false
}

func accountResultMatchesBanToken(item accountResult, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	return strings.TrimSpace(item.AuthIndex) == id ||
		strings.TrimSpace(item.FileName) == id ||
		strings.TrimSpace(item.FileID) == id ||
		strings.TrimSpace(item.Name) == id ||
		strings.TrimSpace(item.Email) == id
}

// resolveBanDeleteFromHostList mirrors resolveBanDeleteFromResults for CPA host
// auth-list entries (Name/ID = physical file, plus AuthIndex/Email/Label).
func resolveBanDeleteFromHostList(files []pluginapi.HostAuthFileEntry, id string) (accountResult, bool, bool) {
	id = strings.TrimSpace(id)
	if id == "" || len(files) == 0 {
		return accountResult{}, false, false
	}
	matched := make([]pluginapi.HostAuthFileEntry, 0, 2)
	seen := map[string]struct{}{}
	for _, file := range files {
		if !hostAuthMatchesBanToken(file, id) {
			continue
		}
		key := strings.Join([]string{
			strings.TrimSpace(file.AuthIndex),
			strings.TrimSpace(file.Name),
			strings.TrimSpace(file.ID),
			strings.TrimSpace(file.Email),
			strings.TrimSpace(file.Label),
		}, "\x1e")
		emptyHostKey := strings.Repeat("\x1e", 4)
		if key == emptyHostKey {
			key = id
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		matched = append(matched, file)
		if len(matched) > 1 {
			return accountResult{}, true, false
		}
	}
	if len(matched) != 1 {
		return accountResult{}, false, false
	}
	file := matched[0]
	fileName := firstNonEmpty(file.Name, file.ID, id)
	return accountResult{
		AuthIndex: firstNonEmpty(file.AuthIndex, id),
		Name:      fileName,
		FileName:  fileName,
		FileID:    file.ID,
		Email:     file.Email,
		Disabled:  file.Disabled,
	}, false, true
}

func hostAuthMatchesBanToken(file pluginapi.HostAuthFileEntry, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	return strings.TrimSpace(file.AuthIndex) == id ||
		strings.TrimSpace(file.Name) == id ||
		strings.TrimSpace(file.ID) == id ||
		strings.TrimSpace(file.Email) == id ||
		strings.TrimSpace(file.Label) == id
}
