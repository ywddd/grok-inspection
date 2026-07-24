package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

func normalizeForceAction(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "disable", "enable", "delete":
		return value, nil
	default:
		return "", fmt.Errorf("force_action_invalid")
	}
}

func itemSelected(item accountResult, indexSet, classSet map[string]struct{}) bool {
	if len(classSet) > 0 {
		if _, ok := classSet[item.Classification]; !ok {
			return false
		}
	}
	if len(indexSet) == 0 {
		return true
	}
	if _, ok := indexSet[item.AuthIndex]; ok {
		return true
	}
	if _, ok := indexSet[item.Name]; ok {
		return true
	}
	if _, ok := indexSet[item.FileName]; ok {
		return true
	}
	if _, ok := indexSet[item.Email]; ok {
		return true
	}
	return false
}

func (e *inspectionEngine) collectCandidates(req applyRequest) ([]accountResult, error) {
	lang := normalizeLang(req.Lang)
	force, errForce := normalizeForceAction(req.ForceAction)
	if errForce != nil {
		return nil, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "force_action_invalid")))
	}
	indexSet := stringSet(req.AuthIndexes)
	actionSet := stringSet(req.Actions)
	classSet := stringSet(req.Classifications)
	// Filter-based bulk ops must name targets (or classification) explicitly.
	if force != "" && len(indexSet) == 0 && len(classSet) == 0 {
		return nil, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "force_action_requires_targets")))
	}

	candidates := make([]accountResult, 0)
	for _, item := range e.results {
		if !itemSelected(item, indexSet, classSet) {
			continue
		}
		if force != "" {
			copied := item
			copied.Action = force
			copied.BanErrorCode = req.BanErrorCode
			candidates = append(candidates, copied)
			continue
		}
		// Recommended-only mode (执行建议操作)
		if item.Action != "disable" && item.Action != "enable" && item.Action != "delete" {
			continue
		}
		if len(actionSet) > 0 {
			if _, ok := actionSet[item.Action]; !ok {
				continue
			}
		}
		candidates = append(candidates, item)
	}
	return candidates, nil
}

// startApply runs recommended or forced bulk actions asynchronously.
// password/headers are captured for background delete calls (page Management Key).
func cloneHTTPHeader(src http.Header) http.Header {
	if src == nil {
		return nil
	}
	dst := make(http.Header, len(src))
	for k, vals := range src {
		dst[k] = append([]string(nil), vals...)
	}
	return dst
}

func (e *inspectionEngine) startApply(req applyRequest, password string, headers http.Header) error {
	lang := normalizeLang(req.Lang)
	e.mu.Lock()
	if e.shuttingDown {
		e.mu.Unlock()
		return httpErr(http.StatusServiceUnavailable, fmt.Errorf("%s", T(lang, "busy_generic")))
	}
	if e.running || e.applying || e.applyDraining || e.actionInFlight > 0 {
		e.mu.Unlock()
		return httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_generic")))
	}
	unbanJob.mu.Lock()
	unbanBusy := unbanJob.running
	unbanJob.mu.Unlock()
	if unbanBusy {
		e.mu.Unlock()
		return httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_unban")))
	}
	candidates, errCollect := e.collectCandidates(req)
	if errCollect != nil {
		e.mu.Unlock()
		return errCollect
	}
	if len(candidates) == 0 {
		e.mu.Unlock()
		if strings.TrimSpace(req.ForceAction) != "" {
			return httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "no_accounts_matched")))
		}
		return httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "no_recommended_actions")))
	}
	e.applying = true
	e.applyRunID++
	applyID := e.applyRunID
	e.applyLang = lang
	e.applyDone = 0
	e.applyTotal = len(candidates)
	e.applyCurrent = ""
	e.applyFailures = nil
	// Capture auth material for the background goroutine (request may free headers after return).
	password = strings.TrimSpace(password)
	headers = cloneHTTPHeader(headers)
	e.runWG.Add(1)
	e.mu.Unlock()

	go func() {
		defer e.runWG.Done()
		// Capture lang for this job so concurrent inspection/lang changes cannot rewrite progress.
		e.runApply(applyID, candidates, password, headers, lang)
	}()
	return nil
}

// startAction runs a single enable/disable/delete asynchronously.
// Returns action_seq so clients can poll light /status.recent_row_actions
// until that seq is reported — do not treat 202 alone as success.
func (e *inspectionEngine) startAction(req actionRequest, password string, headers http.Header) (uint64, string, error) {
	lang := normalizeLang(req.Lang)
	name := firstNonEmpty(req.Name, req.AuthIndex)
	if name == "" {
		return 0, "", httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "name_or_auth_required")))
	}
	action := "enable"
	if req.Delete {
		action = "delete"
	} else if req.Disabled {
		action = "disable"
	}
	key := firstNonEmpty(req.AuthIndex, req.Name, name)

	e.mu.Lock()
	if e.shuttingDown {
		e.mu.Unlock()
		return 0, "", httpErr(http.StatusServiceUnavailable, fmt.Errorf("%s", T(lang, "busy_generic")))
	}
	if e.running {
		e.mu.Unlock()
		return 0, "", httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_inspection")))
	}
	if e.applying || e.applyDraining {
		e.mu.Unlock()
		return 0, "", httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_apply")))
	}
	unbanJob.mu.Lock()
	unbanBusy := unbanJob.running
	unbanJob.mu.Unlock()
	if unbanBusy {
		e.mu.Unlock()
		return 0, "", httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_unban")))
	}
	e.actionSeq++
	seq := e.actionSeq
	e.actionInFlight++
	password = strings.TrimSpace(password)
	headers = cloneHTTPHeader(headers)
	e.runWG.Add(1)
	e.mu.Unlock()

	go func() {
		defer e.runWG.Done()
		// Yield so management.handle can finish before we re-enter Management HTTP
		// (avoids re-entry deadlock while keeping the UI path snappy).
		time.Sleep(5 * time.Millisecond)
		var errAction error
		if req.Delete {
			errAction = deleteAuthFile(name, password, headers, true)
		} else {
			errAction = setAuthDisabled(name, req.Disabled, password, headers, true)
		}
		e.mu.Lock()
		e.actionInFlight--
		if e.actionInFlight < 0 {
			e.actionInFlight = 0
		}
		e.recordRowActionLocked(seq, key, action, errAction)
		if errAction != nil {
			// Keep a short recent failure list for bulk-style surfaces.
			msg := name + ": " + errAction.Error()
			e.applyFailures = append([]string{msg}, e.applyFailures...)
			if len(e.applyFailures) > 20 {
				e.applyFailures = e.applyFailures[:20]
			}
			e.persistLocked()
		}
		// Success path already persisted inside setAuthDisabled/deleteAuthFile.
		e.mu.Unlock()
	}()
	return seq, action, nil
}

func (e *inspectionEngine) isApplyActive(applyID uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.applying && e.applyRunID == applyID
}

func (e *inspectionEngine) runApply(applyID uint64, candidates []accountResult, password string, headers http.Header, lang Lang) {
	lang = normalizeLang(string(lang))
	defer func() {
		// Bulk enable/delete skip per-account ban saves; flush once before the
		// job becomes idle so a failed flush is part of the completed status.
		banSaveErr := saveActiveStoreErr()
		e.mu.Lock()
		if banSaveErr != nil {
			e.applyFailures = append(e.applyFailures, T(lang, "save_autoban_state_failed", banSaveErr.Error()))
			if len(e.applyFailures) > 20 {
				e.applyFailures = e.applyFailures[:20]
			}
		}
		if e.applyRunID == applyID {
			e.applying = false
			if e.applyCurrent != T(lang, "stopped") {
				e.applyCurrent = ""
			}
		}
		// Always clear draining when this apply goroutine exits so new jobs can start.
		e.applyDraining = false
		e.applying = false
		e.persistLocked()
		e.mu.Unlock()
	}()

	// Split deletes (host batch API) from enable/disable (host single-item only).
	deletes := make([]accountResult, 0)
	others := make([]accountResult, 0)
	for _, item := range candidates {
		if item.Action == "delete" {
			deletes = append(deletes, item)
		} else {
			others = append(others, item)
		}
	}

	// --- Bulk delete via CPA multi-name DELETE (chunks of deleteBatchSize) ---
	for i := 0; i < len(deletes); i += deleteBatchSize {
		if !e.isApplyActive(applyID) {
			return
		}
		end := i + deleteBatchSize
		if end > len(deletes) {
			end = len(deletes)
		}
		chunk := deletes[i:end]
		e.mu.Lock()
		if e.applyRunID != applyID {
			e.mu.Unlock()
			return
		}
		e.applyCurrent = T(lang, "apply_delete_batch", i+1, end, len(deletes))
		e.mu.Unlock()

		batchFails := deleteAuthFilesBatch(chunk, password, headers, false)
		e.mu.Lock()
		if e.applyRunID != applyID {
			e.mu.Unlock()
			return
		}
		if len(batchFails) > 0 {
			e.applyFailures = append(e.applyFailures, batchFails...)
			if len(e.applyFailures) > 20 {
				e.applyFailures = e.applyFailures[:20]
			}
		}
		e.applyDone += len(chunk)
		if e.applyDone%applyPersistEvery == 0 || end == len(deletes) {
			e.persistLocked()
		}
		e.mu.Unlock()
	}

	// --- Enable/disable: no host batch API → concurrent single PATCH ---
	if len(others) == 0 || !e.isApplyActive(applyID) {
		return
	}
	workers := defaultApplyWorkers
	if workers > maxApplyWorkers {
		workers = maxApplyWorkers
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(others) {
		workers = len(others)
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, item := range others {
		if !e.isApplyActive(applyID) {
			break
		}
		item := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if !e.isApplyActive(applyID) {
				return
			}

			e.mu.Lock()
			if e.applyRunID != applyID {
				e.mu.Unlock()
				return
			}
			e.applyCurrent = localizedActionVerb(lang, item.Action) + " " + item.Name
			e.mu.Unlock()

			// Prefer physical auth file name so CPA Auth dir entry is deleted correctly.
			targetName := firstNonEmpty(item.FileName, item.AuthIndex, item.Name, item.Email)
			var errAction error
			switch item.Action {
			case "disable":
				errAction = setAuthDisabledWithBanReason(targetName, true, password, headers, false, item.BanErrorCode)
			case "enable":
				errAction = setAuthDisabled(targetName, false, password, headers, false)
			default:
				errAction = fmt.Errorf("unsupported action %q", item.Action)
			}

			e.mu.Lock()
			if e.applyRunID != applyID {
				e.mu.Unlock()
				return
			}
			if errAction != nil {
				e.applyFailures = append(e.applyFailures, item.Name+": "+errAction.Error())
				if len(e.applyFailures) > 20 {
					e.applyFailures = e.applyFailures[:20]
				}
			}
			e.applyDone++
			done := e.applyDone
			// Occasional flush during bulk — full persist only every N items + final defer.
			if done%applyPersistEvery == 0 {
				e.persistLocked()
			}
			e.mu.Unlock()
		}()
	}
	wg.Wait()
}
