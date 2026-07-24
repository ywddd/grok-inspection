package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func (e *inspectionEngine) start(req startRequest) error {
	_, err := e.startWithRunID(req)
	return err
}

func (e *inspectionEngine) startWithRunID(req startRequest) (uint64, error) {
	lang := normalizeLang(req.Lang)
	workers, errWorkers := normalizeWorkersLocalized(req.Workers, lang)
	if errWorkers != nil {
		return 0, errWorkers
	}
	classifications := normalizeClassifications(req.Classifications)
	classifyScoped := len(classifications) > 0
	if classifyScoped && req.Incremental {
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "category_with_incremental")))
	}
	if req.Sample && req.Incremental {
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "sample_with_incremental")))
	}
	sampleCount, samplePercent, errSample := normalizeSampleRequest(req.Sample, req.SampleCount, req.SamplePercent, lang)
	if errSample != nil {
		return 0, errSample
	}
	sampleMode := req.Sample

	e.mu.Lock()
	if e.shuttingDown {
		e.mu.Unlock()
		return 0, httpErr(http.StatusServiceUnavailable, fmt.Errorf("%s", T(lang, "busy_generic")))
	}
	if e.running || e.applying || e.applyDraining {
		e.mu.Unlock()
		return 0, httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "already_running")))
	}
	if e.actionInFlight > 0 {
		e.mu.Unlock()
		return 0, httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_row_action")))
	}
	unbanJob.mu.Lock()
	unbanBusy := unbanJob.running
	unbanJob.mu.Unlock()
	if unbanBusy {
		e.mu.Unlock()
		return 0, httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_unban")))
	}
	if req.Incremental && len(e.results) == 0 {
		e.mu.Unlock()
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "incremental_needs_results")))
	}
	if classifyScoped && len(e.results) == 0 {
		e.mu.Unlock()
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "category_needs_results")))
	}
	if classifyScoped {
		matched := 0
		classSet := stringSet(classifications)
		for _, item := range e.results {
			if classificationMatches(item.Classification, classSet) {
				matched++
			}
		}
		if matched == 0 {
			e.mu.Unlock()
			return 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "no_accounts_in_category")))
		}
	}
	includeDisabled := req.IncludeDisabled
	onlyDisabled := req.OnlyDisabled
	if onlyDisabled {
		includeDisabled = false
	}
	e.lang = lang
	e.running = true
	e.stopped = false
	e.applying = false
	e.runTargets = nil
	e.runModel = ""
	e.runClassifyScoped = false
	e.incremental = req.Incremental && !classifyScoped && !sampleMode
	e.sampleMode = sampleMode
	e.sampleCount = sampleCount
	e.samplePercent = samplePercent
	e.classifications = classifications
	e.workers = workers
	e.includeDisabled = includeDisabled
	e.onlyDisabled = onlyDisabled
	// Full inspect clears only after auth list succeeds (see run).
	// Sample and category runs keep prior results and upsert.
	e.fullClearPending = !req.Incremental && !classifyScoped && !sampleMode
	e.runIsFullInspect = !req.Incremental && !classifyScoped && !sampleMode
	e.runListOK = false
	e.runListError = ""
	e.total = 0
	e.probeDone = 0
	e.probePhase = "listing"
	e.retryDone = 0
	e.retryTotal = 0
	e.retryWorkers = slowRetryWorkers(workers)
	e.applyDone = 0
	e.applyTotal = 0
	e.applyCurrent = ""
	e.applyFailures = nil
	e.startedAt = time.Now()
	e.finishedAt = time.Time{}
	e.runID++
	runID := e.runID
	incremental := e.incremental
	e.persistLocked()
	// Add while still locked relative to shutdown Wait to avoid race.
	e.runWG.Add(1)
	e.mu.Unlock()

	go func() {
		defer e.runWG.Done()
		e.run(runID, workers, includeDisabled, onlyDisabled, incremental, classifications)
	}()
	return runID, nil
}

// stop aborts the job immediately for the UI (legacy/internal entrypoint).
// Prefer stopWithLang when a request language is available.
func (e *inspectionEngine) stop() {
	e.stopWithLang("")
}

// stopWithLang aborts the job immediately for the UI:
// running flips false now, unfinished targets become T(stopLang, "stopped_before_probe"),
// and in-flight probe results are discarded (run continues draining in background).
// Also cancels bulk apply and async unban jobs.
//
// lang is the stop request language. Empty keeps internal/shutdown callers working
// and falls back to applyLang (when apply is active) or the inspection language.
func (e *inspectionEngine) stopWithLang(lang string) {
	stopUnbanJob()
	e.mu.Lock()
	e.stopped = true
	stopLang := e.resolveStopLangLocked(lang)
	e.stopLang = stopLang
	// Cancel bulk apply without waiting for in-flight Management calls.
	// applyDraining keeps start/apply/unban blocked until those calls finish.
	if e.applying {
		e.applyRunID++
		e.applying = false
		e.applyDraining = true
		e.applyCurrent = T(stopLang, "stopped")
	}
	if !e.running {
		snap := e.copyPersistedLocked()
		e.mu.Unlock()
		e.saveSnapshotAndRecord(snap)
		return
	}
	e.abortRunLocked()
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	e.saveSnapshotAndRecord(snap)
}

// resolveStopLangLocked picks the language used for stop-written user copy.
func (e *inspectionEngine) resolveStopLangLocked(lang string) Lang {
	if strings.TrimSpace(lang) != "" {
		return normalizeLang(lang)
	}
	if e.applying {
		if e.applyLang != "" {
			return e.applyLang
		}
		if e.lang != "" {
			return e.lang
		}
		return LangZH
	}
	if e.lang != "" {
		return e.lang
	}
	return LangZH
}

// abortRunLocked finalizes a running job under e.mu.
func (e *inspectionEngine) abortRunLocked() {
	if !e.running {
		return
	}
	model := e.runModel
	// Mark every not-yet-recorded target as cancelled so progress hits total now.
	for _, file := range e.runTargets {
		if resultContainsAuthFile(e.results, file) {
			continue
		}
		stopLang := e.stopLang
		if stopLang == "" {
			stopLang = e.lang
		}
		item := cancelledAccountResult(file, model, stopLang)
		if e.runClassifyScoped {
			if idx := findResultIndex(e.results, item); idx >= 0 {
				e.results[idx] = item
			} else {
				e.results = append(e.results, item)
			}
		} else {
			e.results = append(e.results, item)
		}
	}
	if e.total > 0 {
		e.probeDone = e.total
	} else {
		e.probeDone = len(e.results)
		e.total = e.probeDone
	}
	e.running = false
	e.probePhase = "stopped"
	e.finishedAt = time.Now()
	e.runTargets = nil
	e.bumpResultsLocked()
	// Invalidate in-flight writers from this run.
	e.runID++
}

func (e *inspectionEngine) isStopped(runID uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopped || e.runID != runID
}

func (e *inspectionEngine) appendResult(runID uint64, result accountResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// Discard after stop/abort (runID bumped) or if this is a stale worker.
	if e.runID != runID || e.stopped {
		return
	}
	e.results = append(e.results, result)
	e.probeDone++
	e.bumpResultsLocked()
	// Periodic flush so a crash mid-run still keeps partial progress.
	if e.probeDone%25 == 0 {
		e.persistLocked()
	}
}

// replaceResult updates a primary-phase row after a timeout retry without
// incrementing the main account progress a second time.
func (e *inspectionEngine) replaceResult(runID uint64, result accountResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runID != runID || e.stopped {
		return
	}
	if idx := findResultIndex(e.results, result); idx >= 0 {
		e.results[idx] = result
	} else {
		e.results = append(e.results, result)
	}
	e.retryDone++
	e.bumpResultsLocked()
	if e.retryDone%25 == 0 {
		e.persistLocked()
	}
}

func (e *inspectionEngine) setRetryPhase(runID uint64, total, workers int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runID != runID || e.stopped {
		return false
	}
	e.probePhase = "retry"
	e.retryTotal = total
	e.retryDone = 0
	e.retryWorkers = workers
	return true
}

// upsertResult replaces an existing row by stable identity, or appends if new.
// Used by classify-scoped re-inspect so other categories stay intact.
func (e *inspectionEngine) upsertResult(runID uint64, result accountResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// Discard after stop/abort (runID bumped) or if this is a stale worker.
	if e.runID != runID || e.stopped {
		return
	}
	if idx := findResultIndex(e.results, result); idx >= 0 {
		e.results[idx] = result
	} else {
		e.results = append(e.results, result)
	}
	e.probeDone++
	e.bumpResultsLocked()
	if e.probeDone%25 == 0 {
		e.persistLocked()
	}
}

func (e *inspectionEngine) finish(runID uint64) {
	e.mu.Lock()
	if e.runID != runID {
		e.mu.Unlock()
		return
	}
	e.running = false
	e.probePhase = "finished"
	e.finishedAt = time.Now()
	// Publish tokenized outcome for this completed run (schedule gates on runID).
	e.lastFinishedRunID = runID
	e.lastFinishedListOK = e.runListOK
	e.lastFinishedListError = e.runListError
	e.lastFinishedFullInspect = e.runIsFullInspect
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	// Final flush is synchronous so the last results survive process restart.
	e.saveSnapshotAndRecord(snap)
}

// noteRunListOutcome records whether this run obtained the account list.
// Only updates when runID is still current (superseded runs are ignored).
func (e *inspectionEngine) noteRunListOutcome(runID uint64, ok bool, errMsg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runID != runID {
		return
	}
	e.runListOK = ok
	if ok {
		e.runListError = ""
	} else {
		e.runListError = strings.TrimSpace(errMsg)
	}
}

// latestRunID returns the current engine run token (tests / diagnostics).
func (e *inspectionEngine) latestRunID() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.runID
}

// inspectionRunWaitState reports progress for a specific run token:
//   - "running": this run is the active inspection
//   - "finished": this run completed via finish (outcome published under runID)
//   - "superseded": this run was stopped/aborted or replaced; do not follow a newer run
func (e *inspectionEngine) inspectionRunWaitState(runID uint64) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lastFinishedRunID == runID {
		return "finished"
	}
	if e.runID == runID && e.running {
		return "running"
	}
	return "superseded"
}

// finishedRunOutcome returns the published outcome for a completed runID.
// found=false if a different run finished last (or none yet).
func (e *inspectionEngine) finishedRunOutcome(runID uint64) (listOK bool, listError string, fullInspect bool, found bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.lastFinishedRunID != runID {
		return false, "", false, false
	}
	return e.lastFinishedListOK, e.lastFinishedListError, e.lastFinishedFullInspect, true
}

// knownResultKeys builds skip-keys for incremental inspect.
// Prefer stable auth_index only. Never use email/display name alone (re-import
// with a new token would incorrectly skip). Without auth_index, fall back to
// file_name + size + mtime (or file id) so a rewritten file is re-probed.

// commitRunPlanLocked registers probe targets and, for full inspect, clears prior
// results in the same critical section. Caller must hold e.mu.
// cont=false means the run should exit (stale runID or stop already requested).
// cleared=true when full results were wiped and should be persisted.
func (e *inspectionEngine) commitRunPlanLocked(runID uint64, targets []pluginapi.HostAuthFileEntry, missing []accountResult, model string, classifyScoped bool) (cont bool, cleared bool) {
	if e.runID != runID {
		return false, false
	}
	if e.fullClearPending {
		e.results = nil
		e.bumpResultsLocked()
		e.fullClearPending = false
		cleared = true
	}
	if classifyScoped {
		e.total = len(targets) + len(missing)
	} else {
		e.total = len(targets)
	}
	// Only probe targets are cancellable via immediate stop; missing already written.
	e.runTargets = append([]pluginapi.HostAuthFileEntry(nil), targets...)
	e.runModel = model
	e.runClassifyScoped = classifyScoped
	e.probePhase = "primary"
	if e.stopped {
		e.abortRunLocked()
		return false, cleared
	}
	return true, cleared
}
