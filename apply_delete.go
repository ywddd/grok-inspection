package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"grok-inspection/cpasdk/pluginapi"
	"net/http"
	"net/url"
	"strings"
)

func (e *inspectionEngine) removeResultLocked(target *pluginapi.HostAuthFileEntry, name string) {
	kept := make([]accountResult, 0, len(e.results))
	for _, item := range e.results {
		if resultMatchesTarget(item, target, name) {
			continue
		}
		kept = append(kept, item)
	}
	e.results = kept
	e.bumpResultsLocked()
	if !e.running {
		e.total = len(e.results)
	}
}

// deleteAuthFile must only be called from a background goroutine after management.handle
// has returned, so it does not deadlock on the management lock.
// It deletes the CPA Auth credential file AND removes the row from local JSON results.
// password/headers come from the page Management Key (or env fallbacks) so third-party
// installs work without MANAGEMENT_PASSWORD on the process.
func deleteAuthFile(name string, password string, headers http.Header, persist bool) error {
	target, errTarget := findAuthFile(name)
	if errTarget != nil {
		// Idempotent: already gone counts as success for delete recommendations.
		if strings.Contains(errTarget.Error(), "auth not found") {
			return withAuthOp(name, func() error {
				engine.mu.Lock()
				engine.removeResultLocked(nil, name)
				bansRemoved := clearBansMatchingTarget(nil, name)
				if persist {
					engine.persistLocked()
				}
				engine.mu.Unlock()
				if bansRemoved > 0 && persist {
					if err := saveActiveStoreErr(); err != nil {
						return fmt.Errorf("deleted locally but failed to persist ban state: %w", err)
					}
				}
				return nil
			})
		}
		return errTarget
	}
	fileName := firstNonEmpty(target.Name)
	if fileName == "" {
		return fmt.Errorf("auth file name missing for %s", name)
	}
	return withAuthOp(fileName, func() error {
		path := "/v0/management/auth-files?name=" + url.QueryEscape(fileName)
		if _, _, errDelete := callCPAManagementWithAuth(http.MethodDelete, path, nil, password, headers); errDelete != nil {
			// Concurrent deletes / already-removed files often surface as 404 — treat as success.
			msg := errDelete.Error()
			if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
				engine.mu.Lock()
				engine.removeResultLocked(target, name)
				bansRemoved := clearBansMatchingTarget(target, name)
				if persist {
					engine.persistLocked()
				}
				engine.mu.Unlock()
				if bansRemoved > 0 && persist {
					if err := saveActiveStoreErr(); err != nil {
						return fmt.Errorf("deleted in CPA but failed to persist ban state: %w", err)
					}
				}
				return nil
			}
			// Remote failure: keep local results/bans.
			return errDelete
		}
		// Trust a successful DELETE; do not re-list all CPA auth files to verify.
		engine.mu.Lock()
		engine.removeResultLocked(target, name)
		bansRemoved := clearBansMatchingTarget(target, name)
		if persist {
			engine.persistLocked()
		}
		engine.mu.Unlock()
		if bansRemoved > 0 && persist {
			if err := saveActiveStoreErr(); err != nil {
				return fmt.Errorf("deleted in CPA but failed to persist ban state: %w", err)
			}
		}
		return nil
	})
}

// resolveDeleteFileName picks the CPA auth file name for management DELETE.
func resolveDeleteFileName(item accountResult) string {
	return firstNonEmpty(item.FileName, item.Name, item.AuthIndex, item.Email)
}

// deleteAuthFilesBatch deletes many auth files in one CPA Management API call.
// Host supports: DELETE /v0/management/auth-files with body {"names":[...]} or multi ?name=.
// Returns per-item failure messages (empty means all ok).
// Successful names clear local results/bans when the remote outcome is known.
func deleteAuthFilesBatch(items []accountResult, password string, headers http.Header, persist bool) []string {
	return deleteAuthFilesBatchWithClear(items, password, headers, persist, true)
}

// deleteAuthFilesBatchWithClear is deleteAuthFilesBatch with optional local clear.
// clearLocal=false is for callers that apply fail-closed accounting and only clear
// rows they can positively confirm succeeded.
func deleteAuthFilesBatchWithClear(items []accountResult, password string, headers http.Header, persist bool, clearLocal bool) []string {
	if len(items) == 0 {
		return nil
	}
	type mapped struct {
		item     accountResult
		fileName string
	}
	mappedItems := make([]mapped, 0, len(items))
	// Duplicate result rows sharing a physical file name. Cleared only after a
	// successful remote DELETE for that name — never when remote fails.
	dupAliases := map[string][]string{}
	names := make([]string, 0, len(items))
	failures := make([]string, 0)
	seenName := map[string]struct{}{}
	for _, item := range items {
		fileName := resolveDeleteFileName(item)
		if fileName == "" {
			failures = append(failures, item.Name+": auth file name missing")
			continue
		}
		alias := firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email)
		if _, ok := seenName[fileName]; ok {
			dupAliases[fileName] = append(dupAliases[fileName], alias)
			continue
		}
		seenName[fileName] = struct{}{}
		mappedItems = append(mappedItems, mapped{item: item, fileName: fileName})
		names = append(names, fileName)
	}
	if len(names) == 0 {
		if persist {
			engine.persist()
		}
		return failures
	}

	body, errMarshal := json.Marshal(map[string]any{"names": names})
	if errMarshal != nil {
		for _, m := range mappedItems {
			failures = append(failures, m.item.Name+": "+errMarshal.Error())
		}
		return failures
	}

	status, raw, errDelete := callCPAManagementWithAuth(http.MethodDelete, "/v0/management/auth-files", body, password, headers)
	if errDelete != nil {
		msg := errDelete.Error()
		// Already gone: treat as success and clear local. Other errors keep local state.
		if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
			if clearLocal {
				for _, m := range mappedItems {
					clearOneDeletedAuthLocal(m.item, m.fileName, dupAliases[m.fileName], false)
				}
				if persist {
					engine.persist()
					_ = saveActiveStoreErr()
				}
			}
			return failures
		}
		for _, m := range mappedItems {
			failures = append(failures, m.item.Name+": "+msg)
		}
		return failures
	}

	// 207 Multi-Status is fail-closed at the batch layer: only clear per-item when
	// every failure row can be parsed and attributed to a requested name. Ordinary
	// 200 responses stay lenient (including {status:ok} with empty/malformed extras).
	failedNames := map[string]string{}
	if status == http.StatusMultiStatus {
		parsed, batchErr := parseAuthDeleteMultiStatusFailures(raw, seenName)
		if batchErr != nil {
			for _, m := range mappedItems {
				label := firstNonEmpty(m.item.Name, m.fileName, m.item.AuthIndex)
				failures = append(failures, label+": "+batchErr.Error())
			}
			// Keep local state for the whole batch; do not clear on unusable 207.
			return failures
		}
		failedNames = parsed
	} else if len(raw) > 0 {
		// Non-207 bodies may still carry a partial failed[] list (legacy hosts).
		if parsed, err := parseAuthDeleteFailedMapLenient(raw); err == nil {
			failedNames = parsed
		}
	}

	anyCleared := false
	for _, m := range mappedItems {
		if errText, ok := failedNames[m.fileName]; ok {
			label := firstNonEmpty(m.item.Name, m.fileName, m.item.AuthIndex)
			failures = append(failures, label+": "+errText)
			continue
		}
		if clearLocal {
			if clearOneDeletedAuthLocal(m.item, m.fileName, dupAliases[m.fileName], false) {
				anyCleared = true
			}
		}
	}
	if persist {
		engine.persist()
		if anyCleared {
			_ = saveActiveStoreErr()
		}
	}
	return failures
}

type authDeleteMultiFailed struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type authDeleteMultiPayload struct {
	Status string                  `json:"status"`
	Failed []authDeleteMultiFailed `json:"failed"`
	Files  []string                `json:"files"`
}

// parseAuthDeleteMultiStatusFailures parses a 207 body. requested must contain
// every physical name sent in this DELETE. The body must be a known multi-status
// schema: JSON object with an explicit "failed" array (empty array = no failures).
// Missing/null/non-array failed, empty/unknown failure names, or malformed JSON
// make the whole multi-status unusable (caller fail-closes, keeps local state).
func parseAuthDeleteMultiStatusFailures(raw []byte, requested map[string]struct{}) (map[string]string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("multi-status delete response missing body")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("multi-status delete response unusable: %v", err)
	}
	failedRaw, ok := top["failed"]
	if !ok {
		return nil, fmt.Errorf("multi-status delete response missing failed array")
	}
	trimFailed := bytes.TrimSpace(failedRaw)
	if len(trimFailed) == 0 || bytes.Equal(trimFailed, []byte("null")) {
		return nil, fmt.Errorf("multi-status delete response failed is null")
	}
	var failedList []authDeleteMultiFailed
	if err := json.Unmarshal(failedRaw, &failedList); err != nil {
		return nil, fmt.Errorf("multi-status delete response failed is not an array")
	}
	// empty failed array is valid: no per-name failures.
	failedNames := map[string]string{}
	for _, f := range failedList {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			return nil, fmt.Errorf("multi-status delete failure missing name")
		}
		if _, ok := requested[name]; !ok {
			return nil, fmt.Errorf("multi-status delete failure for unknown name %q", name)
		}
		errText := strings.TrimSpace(f.Error)
		if errText == "" {
			errText = "delete failed"
		}
		failedNames[name] = errText
	}
	return failedNames, nil
}

// parseAuthDeleteFailedMapLenient extracts failed[] from non-207 bodies without
// failing the whole batch on parse issues (preserves historical 200 behavior).
func parseAuthDeleteFailedMapLenient(raw []byte) (map[string]string, error) {
	var payload authDeleteMultiPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	failedNames := map[string]string{}
	for _, f := range payload.Failed {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			continue
		}
		errText := strings.TrimSpace(f.Error)
		if errText == "" {
			errText = "delete failed"
		}
		failedNames[name] = errText
	}
	return failedNames, nil
}

// clearOneDeletedAuthLocal removes local results/bans for one successfully deleted
// auth file (and any duplicate aliases that pointed at the same physical name).
// Serialized per fileName so Usage/restore cannot interleave inconsistently.
func clearOneDeletedAuthLocal(item accountResult, fileName string, dupAliases []string, persist bool) bool {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return false
	}
	changed := false
	_ = withAuthOp(fileName, func() error {
		engine.mu.Lock()
		alias := firstNonEmpty(item.AuthIndex, fileName, item.Name, item.Email)
		engine.removeResultLocked(nil, alias)
		if clearBansMatchingTarget(nil, alias) > 0 {
			changed = true
		}
		if clearBansMatchingTarget(nil, fileName) > 0 {
			changed = true
		}
		for _, a := range dupAliases {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			engine.removeResultLocked(nil, a)
			if clearBansMatchingTarget(nil, a) > 0 {
				changed = true
			}
		}
		if persist {
			engine.persistLocked()
		}
		engine.mu.Unlock()
		if changed && persist {
			_ = saveActiveStoreErr()
		}
		return nil
	})
	return changed
}
