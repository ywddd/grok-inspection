package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func findAuthFromResults(name string) *pluginapi.HostAuthFileEntry {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	for i := range engine.results {
		item := &engine.results[i]
		if item.AuthIndex == name || item.FileName == name || item.Name == name || item.Email == name {
			fileName := firstNonEmpty(item.FileName, item.Name)
			if fileName == "" {
				return nil
			}
			return &pluginapi.HostAuthFileEntry{
				AuthIndex: item.AuthIndex,
				Name:      fileName,
				ID:        firstNonEmpty(item.FileName, item.AuthIndex),
				Email:     item.Email,
				Disabled:  item.Disabled,
			}
		}
	}
	return nil
}

func findAuthFile(name string) (*pluginapi.HostAuthFileEntry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	// Fast path: after a full/incremental inspect we already know file names.
	// Avoid listing 1000+ CPA auth files on every enable/disable/delete click.
	if entry := findAuthFromResults(name); entry != nil {
		return entry, nil
	}
	list, errList := callHostAuthList()
	if errList != nil {
		return nil, errList
	}
	for i := range list.Files {
		file := &list.Files[i]
		if file.Name == name || file.ID == name || file.AuthIndex == name || file.Email == name {
			return file, nil
		}
	}
	return nil, fmt.Errorf("auth not found: %s", name)
}

// setAuthDisabled toggles CPA auth via Management API PATCH /auth-files/status.
// host.auth.save alone is NOT enough: CLIProxyAPI buildAuthFromFileData does not
// promote JSON "disabled" onto Auth.Disabled, so the main UI stays enabled.
// Must run outside management.handle (background goroutine) to avoid re-entry deadlock.
//
// After a successful PATCH we trust CPA and update local results only — we do not
// re-list all auth files (that was the main reason single-row ops felt slow).
// persist=false is used by bulk apply so 1000 disk flushes do not dominate runtime.
func setAuthDisabled(name string, disabled bool, password string, headers http.Header, persist bool) error {
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
	if _, _, errPatch := callCPAManagementWithAuth(http.MethodPatch, "/v0/management/auth-files/status", body, password, headers); errPatch != nil {
		return errPatch
	}
	engine.mu.Lock()
	for i := range engine.results {
		item := &engine.results[i]
		if resultMatchesTarget(*item, target, name) {
			item.Disabled = disabled
			if disabled && item.Action == "disable" {
				item.Action = "keep"
			}
			if !disabled && item.Action == "enable" {
				item.Action = "keep"
			}
			if disabled && item.Classification == "healthy" {
				item.Action = "enable"
			}
		}
	}
	engine.bumpResultsLocked()
	if persist {
		engine.persistLocked()
	}
	engine.mu.Unlock()
	return nil
}

func resultMatchesTarget(item accountResult, target *pluginapi.HostAuthFileEntry, name string) bool {
	name = strings.TrimSpace(name)
	if target != nil {
		if item.AuthIndex != "" && item.AuthIndex == target.AuthIndex {
			return true
		}
		if item.FileName != "" && (item.FileName == target.Name || item.FileName == target.ID) {
			return true
		}
		if item.Name != "" && (item.Name == target.Name || item.Name == target.Email || item.Name == target.ID) {
			return true
		}
	}
	if name == "" {
		return false
	}
	return item.AuthIndex == name || item.FileName == name || item.Name == name || item.Email == name
}

// removeResultLocked drops matching rows from the in-memory list. Caller must hold e.mu.
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
			engine.mu.Lock()
			engine.removeResultLocked(nil, name)
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
			return nil
		}
		return errTarget
	}
	fileName := firstNonEmpty(target.Name)
	if fileName == "" {
		return fmt.Errorf("auth file name missing for %s", name)
	}
	path := "/v0/management/auth-files?name=" + url.QueryEscape(fileName)
	if _, _, errDelete := callCPAManagementWithAuth(http.MethodDelete, path, nil, password, headers); errDelete != nil {
		// Concurrent deletes / already-removed files often surface as 404 — treat as success.
		msg := errDelete.Error()
		if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
			engine.mu.Lock()
			engine.removeResultLocked(target, name)
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
			return nil
		}
		return errDelete
	}
	// Trust a successful DELETE; do not re-list all CPA auth files to verify.
	engine.mu.Lock()
	engine.removeResultLocked(target, name)
	if persist {
		engine.persistLocked()
	}
	engine.mu.Unlock()
	return nil
}

// resolveDeleteFileName picks the CPA auth file name for management DELETE.
func resolveDeleteFileName(item accountResult) string {
	return firstNonEmpty(item.FileName, item.Name, item.AuthIndex, item.Email)
}

// deleteAuthFilesBatch deletes many auth files in one CPA Management API call.
// Host supports: DELETE /v0/management/auth-files with body {"names":[...]} or multi ?name=.
// Returns per-item failure messages (empty means all ok).
func deleteAuthFilesBatch(items []accountResult, password string, headers http.Header, persist bool) []string {
	if len(items) == 0 {
		return nil
	}
	type mapped struct {
		item     accountResult
		fileName string
	}
	mappedItems := make([]mapped, 0, len(items))
	names := make([]string, 0, len(items))
	failures := make([]string, 0)
	seenName := map[string]struct{}{}
	for _, item := range items {
		fileName := resolveDeleteFileName(item)
		if fileName == "" {
			failures = append(failures, item.Name+": auth file name missing")
			continue
		}
		// Skip duplicate physical names in the same batch (same file, multiple result rows).
		if _, ok := seenName[fileName]; ok {
			// Still drop matching local rows for this identity.
			engine.mu.Lock()
			engine.removeResultLocked(nil, firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email))
			engine.mu.Unlock()
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
		// Whole batch failed (network / hard error). Mark all remaining names failed.
		msg := errDelete.Error()
		// If everything was already gone, treat as success.
		if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
			engine.mu.Lock()
			for _, m := range mappedItems {
				engine.removeResultLocked(nil, firstNonEmpty(m.item.AuthIndex, m.fileName, m.item.Name, m.item.Email))
			}
			if persist {
				engine.persistLocked()
			}
			engine.mu.Unlock()
			return failures
		}
		for _, m := range mappedItems {
			failures = append(failures, m.item.Name+": "+msg)
		}
		return failures
	}

	// Parse optional partial failure payload (HTTP 207 Multi-Status).
	failedNames := map[string]string{}
	if status == http.StatusMultiStatus || len(raw) > 0 {
		var payload struct {
			Status  string `json:"status"`
			Failed  []struct {
				Name  string `json:"name"`
				Error string `json:"error"`
			} `json:"failed"`
			Files []string `json:"files"`
		}
		if err := json.Unmarshal(raw, &payload); err == nil {
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
		}
	}

	engine.mu.Lock()
	for _, m := range mappedItems {
		if errText, ok := failedNames[m.fileName]; ok {
			failures = append(failures, m.item.Name+": "+errText)
			continue
		}
		engine.removeResultLocked(nil, firstNonEmpty(m.item.AuthIndex, m.fileName, m.item.Name, m.item.Email))
	}
	if persist {
		engine.persistLocked()
	}
	engine.mu.Unlock()
	_ = status
	return failures
}

func normalizeForceAction(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "disable", "enable", "delete":
		return value, nil
	default:
		return "", fmt.Errorf("force_action must be disable, enable, or delete")
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
	force, errForce := normalizeForceAction(req.ForceAction)
	if errForce != nil {
		return nil, errForce
	}
	indexSet := stringSet(req.AuthIndexes)
	actionSet := stringSet(req.Actions)
	classSet := stringSet(req.Classifications)
	// Filter-based bulk ops must name targets (or classification) explicitly.
	if force != "" && len(indexSet) == 0 && len(classSet) == 0 {
		return nil, fmt.Errorf("force_action requires auth_indexes or classifications")
	}

	candidates := make([]accountResult, 0)
	for _, item := range e.results {
		if !itemSelected(item, indexSet, classSet) {
			continue
		}
		if force != "" {
			copied := item
			copied.Action = force
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
	e.mu.Lock()
		if e.running || e.applying || e.reauthing || e.baseURLApplying || e.actionInFlight > 0 {
			e.mu.Unlock()
			return fmt.Errorf("busy")
		}
	candidates, errCollect := e.collectCandidates(req)
	if errCollect != nil {
		e.mu.Unlock()
		return errCollect
	}
	if len(candidates) == 0 {
		e.mu.Unlock()
		if strings.TrimSpace(req.ForceAction) != "" {
			return fmt.Errorf("no accounts matched current selection")
		}
		return fmt.Errorf("no recommended actions")
	}
	e.applying = true
	e.applyDone = 0
	e.applyTotal = len(candidates)
	e.applyCurrent = ""
	e.applyFailures = nil
	// Capture auth material for the background goroutine (request may free headers after return).
	password = strings.TrimSpace(password)
	headers = cloneHTTPHeader(headers)
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		e.runApply(candidates, password, headers)
	}()
	return nil
}

// startAction runs a single enable/disable/delete asynchronously.
// Returns action_seq so clients can poll light /status.recent_row_actions
// until that seq is reported — do not treat 202 alone as success.
func (e *inspectionEngine) startAction(req actionRequest, password string, headers http.Header) (uint64, string, error) {
	name := firstNonEmpty(req.Name, req.AuthIndex)
	if name == "" {
		return 0, "", fmt.Errorf("name or auth_index required")
	}
	action := "enable"
	if req.Delete {
		action = "delete"
	} else if req.Disabled {
		action = "disable"
	}
	key := firstNonEmpty(req.AuthIndex, req.Name, name)

	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return 0, "", fmt.Errorf("busy: inspection running")
	}
	if e.applying {
		e.mu.Unlock()
		return 0, "", fmt.Errorf("busy: bulk apply in progress")
	}
		if e.reauthing {
			e.mu.Unlock()
			return 0, "", fmt.Errorf("busy: token refresh in progress")
		}
		if e.baseURLApplying {
			e.mu.Unlock()
			return 0, "", fmt.Errorf("busy: base_url switch in progress")
		}
		e.actionSeq++
	seq := e.actionSeq
	e.actionInFlight++
	password = strings.TrimSpace(password)
	headers = cloneHTTPHeader(headers)
	e.mu.Unlock()

	e.runWG.Add(1)
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

func (e *inspectionEngine) runApply(candidates []accountResult, password string, headers http.Header) {
	defer func() {
		e.mu.Lock()
		e.applying = false
		e.applyCurrent = ""
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
		end := i + deleteBatchSize
		if end > len(deletes) {
			end = len(deletes)
		}
		chunk := deletes[i:end]
		e.mu.Lock()
		e.applyCurrent = fmt.Sprintf("delete batch %d-%d/%d", i+1, end, len(deletes))
		e.mu.Unlock()

		batchFails := deleteAuthFilesBatch(chunk, password, headers, false)
		e.mu.Lock()
		if len(batchFails) > 0 {
			e.applyFailures = append(e.applyFailures, batchFails...)
		}
		e.applyDone += len(chunk)
		if e.applyDone%applyPersistEvery == 0 || end == len(deletes) {
			e.persistLocked()
		}
		e.mu.Unlock()
	}

	// --- Enable/disable: no host batch API → concurrent single PATCH ---
	if len(others) == 0 {
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
		item := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			e.mu.Lock()
			e.applyCurrent = item.Action + " " + item.Name
			e.mu.Unlock()

			// Prefer physical auth file name so CPA Auth dir entry is deleted correctly.
			targetName := firstNonEmpty(item.FileName, item.AuthIndex, item.Name, item.Email)
			var errAction error
			switch item.Action {
			case "disable":
				errAction = setAuthDisabled(targetName, true, password, headers, false)
			case "enable":
				errAction = setAuthDisabled(targetName, false, password, headers, false)
			default:
				errAction = fmt.Errorf("unsupported action %q", item.Action)
			}

			e.mu.Lock()
			if errAction != nil {
				e.applyFailures = append(e.applyFailures, item.Name+": "+errAction.Error())
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
