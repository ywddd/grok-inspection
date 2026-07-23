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
	return setAuthDisabledWithBanReason(name, disabled, password, headers, persist, manualInspectionBanErrorCode)
}

func setAuthDisabledWithBanReason(name string, disabled bool, password string, headers http.Header, persist bool, banErrorCode string) error {
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
		if _, _, errPatch := callCPAManagementWithAuth(http.MethodPatch, "/v0/management/auth-files/status", body, password, headers); errPatch != nil {
			return errPatch
		}
		engine.mu.Lock()
		for i := range engine.results {
			item := &engine.results[i]
			if resultMatchesTarget(*item, target, name) {
				item.Disabled = disabled
				item.Action = recommendAction(item.Classification, item.Disabled)
			}
		}
		engine.bumpResultsLocked()
		banStateChanged := false
		if disabled {
			banStateChanged = syncInspectionBan(activeStore, target, name, time.Now(), banErrorCode)
		} else {
			// CAS clear only pre-enable revisions; newer bans stay and re-disable CPA.
			changed, errCAS := clearBansMatchingTargetCAS(activeStore, target, name, pre)
			banStateChanged = changed > 0
			if errCAS != nil {
				engine.mu.Unlock()
				if persist {
					_ = saveActiveStoreErr()
				}
				return errCAS
			}
		}
		if persist {
			engine.persistLocked()
		}
		engine.mu.Unlock()
		if banStateChanged && persist {
			if err := saveActiveStoreErr(); err != nil {
				return fmt.Errorf("updated in CPA but failed to persist ban state: %w", err)
			}
		}
		return nil
	})
}

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

func banAliases(target *pluginapi.HostAuthFileEntry, name string) map[string]struct{} {
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
	return aliases
}

func banIDMatchesAliases(authID string, aliases map[string]struct{}) bool {
	id := strings.TrimSpace(authID)
	if id == "" {
		return false
	}
	if _, ok := aliases[id]; ok {
		return true
	}
	base := id
	if i := strings.LastIndexAny(id, `/\`); i >= 0 {
		base = id[i+1:]
	}
	_, ok := aliases[base]
	return ok
}

// clearBansMatchingTargetCAS deletes only bans whose revision still matches the
// pre-mutation snapshot. Newer revisions are kept and re-disabled in CPA.
func clearBansMatchingTargetCAS(store *banStore, target *pluginapi.HostAuthFileEntry, name string, pre []banEntry) (int, error) {
	if store == nil {
		return 0, nil
	}
	preByID := map[string]uint64{}
	for _, e := range pre {
		preByID[e.AuthID] = e.Revision
	}
	removed := 0
	var firstErr error
	for _, entry := range store.All() {
		if !banIDMatchesAliases(entry.AuthID, banAliases(target, name)) {
			continue
		}
		if oldRev, ok := preByID[entry.AuthID]; ok && entry.Revision == oldRev {
			if store.DeleteIf(entry.AuthID, oldRev) {
				removed++
			}
			continue
		}
		// Newer ban landed during enable: re-disable and keep local state.
		if err := disableAuthInCPA(entry.AuthID); err != nil {
			store.UpdateCpaSyncState(entry.AuthID, false, sanitizeCPASyncError(err))
			if firstErr == nil {
				firstErr = err
			}
		} else {
			store.UpdateCpaSyncState(entry.AuthID, true, "")
		}
	}
	return removed, firstErr
}

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
func deleteAuthFilesBatch(items []accountResult, password string, headers http.Header, persist bool) []string {
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
			for _, m := range mappedItems {
				clearOneDeletedAuthLocal(m.item, m.fileName, dupAliases[m.fileName], false)
			}
			if persist {
				engine.persist()
				_ = saveActiveStoreErr()
			}
			return failures
		}
		for _, m := range mappedItems {
			failures = append(failures, m.item.Name+": "+msg)
		}
		return failures
	}

	failedNames := map[string]string{}
	if status == http.StatusMultiStatus || len(raw) > 0 {
		var payload struct {
			Status string `json:"status"`
			Failed []struct {
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

	anyCleared := false
	for _, m := range mappedItems {
		if errText, ok := failedNames[m.fileName]; ok {
			failures = append(failures, m.item.Name+": "+errText)
			continue
		}
		if clearOneDeletedAuthLocal(m.item, m.fileName, dupAliases[m.fileName], false) {
			anyCleared = true
		}
	}
	if persist {
		engine.persist()
		if anyCleared {
			_ = saveActiveStoreErr()
		}
	}
	_ = status
	return failures
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
