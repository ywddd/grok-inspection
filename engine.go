package main

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

const (
	defaultWorkers      = 6
	minWorkers          = 1
	maxWorkers          = 16
	defaultApplyWorkers = 6 // concurrent Management API calls for bulk enable/disable
	maxApplyWorkers     = 8
	applyPersistEvery   = 25 // persist every N bulk ops (not each) for speed
	// CPA DELETE /auth-files supports multi-name in one request. 50 balances
	// payload size, partial-failure reporting, and progress granularity.
	deleteBatchSize     = 50
	maxSlowRetryWorkers = 8
)

type accountResult struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	FileName  string `json:"file_name,omitempty"`
	Email     string `json:"email,omitempty"`
	// FileID / FileModUnix / FileSize help incremental skip without relying on email/name.
	FileID         string `json:"file_id,omitempty"`
	FileModUnix    int64  `json:"file_mod_unix,omitempty"`
	FileSize       int64  `json:"file_size,omitempty"`
	Disabled       bool   `json:"disabled"`
	Classification string `json:"classification"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Model          string `json:"model,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	BanErrorCode   string `json:"-"`
}

// rowActionReport is a lightweight completion record for single-row /action.
// Clients poll light /status until RecentRowActions contains their action_seq.
type rowActionReport struct {
	Seq    uint64 `json:"seq"`
	Key    string `json:"key,omitempty"`
	Action string `json:"action,omitempty"` // enable | disable | delete
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

type jobSnapshot struct {
	Running     bool `json:"running"`
	Stopped     bool `json:"stopped"`
	Applying    bool `json:"applying"`
	Incremental bool `json:"incremental"`
	// Classifications is set when this run re-probes only matching last classifications.
	Classifications []string `json:"classifications,omitempty"`
	Done            int      `json:"done"`
	Total           int      `json:"total"`
	Workers         int      `json:"workers"`
	ProbePhase      string   `json:"probe_phase,omitempty"`
	RetryDone       int      `json:"retry_done"`
	RetryTotal      int      `json:"retry_total"`
	RetryWorkers    int      `json:"retry_workers"`
	IncludeDisabled bool     `json:"include_disabled"`
	OnlyDisabled    bool     `json:"only_disabled"`
	ApplyDone       int      `json:"apply_done"`
	ApplyTotal      int      `json:"apply_total"`
	ApplyCurrent    string   `json:"apply_current,omitempty"`
	ApplyFailures   []string `json:"apply_failures,omitempty"`
	// ActionInFlight is single-row ops still running (not bulk apply).
	ActionInFlight int `json:"action_in_flight"`
	// RecentRowActions holds latest completed single-row ops for light confirmation.
	RecentRowActions []rowActionReport `json:"recent_row_actions,omitempty"`
	StartedAt        string            `json:"started_at,omitempty"`
	FinishedAt       string            `json:"finished_at,omitempty"`
	Results          []accountResult   `json:"results,omitempty"`
	Summary          map[string]int    `json:"summary"`
	StorePath        string            `json:"store_path,omitempty"`
	// ResultsGen bumps whenever results content changes; light status omits Results.
	ResultsGen     uint64 `json:"results_gen"`
	IncludeResults bool   `json:"include_results"`
	// PersistError is the last results.json save failure, if any.
	PersistError string                      `json:"persist_error,omitempty"`
	Unban        map[string]any              `json:"unban,omitempty"`
	Schedule     persistedInspectionSchedule `json:"schedule"`
}

// httpStatusError pairs a stable HTTP status with a localized operator message.
// Status mapping must not depend on message language.
type httpStatusError struct {
	status int
	err    error
}

func (e *httpStatusError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *httpStatusError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func statusFromError(err error, fallback int) int {
	var he *httpStatusError
	if errors.As(err, &he) && he != nil && he.status > 0 {
		return he.status
	}
	return fallback
}

func httpErr(status int, err error) error {
	return &httpStatusError{status: status, err: err}
}

type stopRequest struct {
	// Lang selects operator-facing stop status language: "zh" (default) or "en".
	Lang string `json:"lang"`
}

type startRequest struct {
	// Lang selects operator-facing runtime message language: "zh" (default) or "en".
	Lang            string `json:"lang"`
	Workers         int    `json:"workers"`
	IncludeDisabled bool   `json:"include_disabled"`
	OnlyDisabled    bool   `json:"only_disabled"`
	// Incremental only probes Auth accounts not already present in the last results.
	Incremental bool `json:"incremental"`
	// Classifications re-probes only accounts whose last classification matches.
	// Keeps other results. Special token "other" matches non-primary classes
	// (not healthy / permission_denied / quota_exhausted / reauth).
	// Mutually exclusive with Incremental.
	Classifications []string `json:"classifications"`
}

type applyRequest struct {
	// Lang is request-scoped operator language for busy/progress copy (default zh).
	// Independent of the last inspection run language.
	Lang string `json:"lang"`
	// empty AuthIndexes means apply all matching recommended actions (when ForceAction empty)
	AuthIndexes     []string `json:"auth_indexes"`
	Actions         []string `json:"actions"`         // optional: disable/enable/delete (recommended only)
	Classifications []string `json:"classifications"` // optional: reauth/healthy/...
	// ForceAction overrides recommended action for selected accounts.
	// Used by filter-based bulk disable/delete. Values: disable | enable | delete
	ForceAction string `json:"force_action"`
	// BanErrorCode is internal-only. Scheduled 403 handling preserves the
	// permission-denied reason instead of labeling the action as manual.
	BanErrorCode string `json:"-"`
}

type actionRequest struct {
	// Lang is request-scoped operator language for busy errors (default zh).
	Lang      string `json:"lang"`
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	Disabled  bool   `json:"disabled"`
	Delete    bool   `json:"delete"`
}

type authListResponse struct {
	Files []pluginapi.HostAuthFileEntry `json:"files"`
}

type inspectionEngine struct {
	lang             Lang // operator-facing language for the current/last inspection run (default zh)
	applyLang        Lang // language for the active bulk-apply job (request-scoped)
	stopLang         Lang // language of the latest user stop request (cancel/stop copy)
	mu               sync.Mutex
	runWG            sync.WaitGroup
	persistWG        sync.WaitGroup // async persistLocked writers
	running          bool
	stopped          bool
	applying         bool
	applyDraining    bool   // bulk apply stopped but in-flight PATCHs still finishing
	applyRunID       uint64 // invalidates in-flight bulk apply on stop
	persistSeq       uint64 // monotonic snapshot id assigned at copy time
	actionInFlight   int    // concurrent single-row enable/disable/delete goroutines
	actionSeq        uint64
	recentRowActions []rowActionReport // ring of latest completed single-row ops
	incremental      bool
	classifications  []string // current/last scoped re-inspect classes
	runID            uint64
	workers          int
	includeDisabled  bool
	onlyDisabled     bool
	total            int
	probeDone        int // probes completed in the current run (full or incremental)
	probePhase       string
	retryDone        int
	retryTotal       int
	retryWorkers     int
	results          []accountResult
	applyDone        int
	applyTotal       int
	applyCurrent     string
	applyFailures    []string
	resultsGen       uint64 // monotonic; used by light /status clients
	persistError     string
	persistStatusSeq uint64 // only seq>=this may update/clear persistError
	startedAt        time.Time
	finishedAt       time.Time
	// Current-run bookkeeping for immediate stop (filled when targets are known).
	runTargets        []pluginapi.HostAuthFileEntry
	runModel          string
	runClassifyScoped bool
	// fullClearPending: full inspect waits for list success before wiping results.
	fullClearPending bool
	schedule         persistedInspectionSchedule
}

const maxRecentRowActions = 32

var engine = &inspectionEngine{
	workers:  defaultWorkers,
	schedule: defaultInspectionSchedule(),
}

var (
	callHostAuthListFn = callHostAuthList
	inspectAccountFn   = inspectAccount
)

func init() {
	// Only restore inspection results here. Ban state and the restore loop must
	// wait for CPA PluginRegister/Reconfigure so state_file/settings are real.
	engine.loadFromDisk()
}

func normalizeWorkers(workers int) (int, error) {
	if workers == 0 {
		return defaultWorkers, nil
	}
	if workers < minWorkers || workers > maxWorkers {
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("workers_invalid"))
	}
	return workers, nil
}

func normalizeWorkersLocalized(workers int, lang Lang) (int, error) {
	n, err := normalizeWorkers(workers)
	if err == nil {
		return n, nil
	}
	return 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "workers_range", minWorkers, maxWorkers)))
}

func slowRetryWorkers(workers int) int {
	retryWorkers := (workers + 1) / 2
	if retryWorkers < 1 {
		return 1
	}
	if retryWorkers > maxSlowRetryWorkers {
		return maxSlowRetryWorkers
	}
	return retryWorkers
}

func (e *inspectionEngine) loadFromDisk() {
	snap, err := loadPersistedSnapshot()
	if err != nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running || e.applying {
		return
	}
	e.results = append([]accountResult(nil), snap.Results...)
	// Prefer small schedule.json; fall back to results.json schedule (migration).
	if sched, errSched := loadInspectionScheduleFromDisk(); errSched == nil {
		e.schedule = sched
	} else {
		e.schedule = normalizePersistedInspectionSchedule(snap.Schedule)
	}
	e.bumpResultsLocked()
	if snap.Workers >= minWorkers && snap.Workers <= maxWorkers {
		e.workers = snap.Workers
	}
	e.includeDisabled = snap.IncludeDisabled
	e.onlyDisabled = snap.OnlyDisabled
	e.total = len(snap.Results)
	if snap.StartedAt != "" {
		if t, errParse := time.Parse(time.RFC3339, snap.StartedAt); errParse == nil {
			e.startedAt = t
		}
	}
	if snap.FinishedAt != "" {
		if t, errParse := time.Parse(time.RFC3339, snap.FinishedAt); errParse == nil {
			e.finishedAt = t
		}
	}
}

// copyPersistedLocked builds a disk snapshot while the caller holds e.mu.
// Each snapshot gets a monotonic seq so delayed async saves cannot overwrite
// a newer finish/stop flush.
func (e *inspectionEngine) copyPersistedLocked() persistedSnapshot {
	e.persistSeq++
	snap := persistedSnapshot{
		Workers:         e.workers,
		IncludeDisabled: e.includeDisabled,
		OnlyDisabled:    e.onlyDisabled,
		Results:         append([]accountResult(nil), e.results...),
		Schedule:        e.schedule,
		seq:             e.persistSeq,
	}
	if !e.startedAt.IsZero() {
		snap.StartedAt = e.startedAt.Format(time.RFC3339)
	}
	if !e.finishedAt.IsZero() {
		snap.FinishedAt = e.finishedAt.Format(time.RFC3339)
	}
	return snap
}

// persistAsyncBeforeSave is an optional test hook invoked inside async
// persistLocked goroutines before disk I/O (nil in production).
var persistAsyncBeforeSave func()

// persistLocked copies under the caller lock, then writes asynchronously so
// status/snapshot callers are not blocked on disk I/O for large result lists.
// Caller must hold e.mu. The writer is tracked on persistWG so shutdown can wait.
func (e *inspectionEngine) persistLocked() {
	e.persistWG.Add(1)
	snap := e.copyPersistedLocked()
	go func(s persistedSnapshot) {
		defer e.persistWG.Done()
		if hook := persistAsyncBeforeSave; hook != nil {
			hook()
		}
		err := savePersistedSnapshot(s)
		e.mu.Lock()
		e.applyPersistResultLocked(s.seq, err)
		e.mu.Unlock()
	}(snap)
}

// waitAsyncPersist blocks until all persistLocked writers finish.
func (e *inspectionEngine) waitAsyncPersist() {
	e.persistWG.Wait()
}

// persist copies under lock and writes outside the critical section.
func (e *inspectionEngine) persist() {
	e.mu.Lock()
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	e.saveSnapshotAndRecord(snap)
}

func (e *inspectionEngine) saveSnapshotAndRecord(snap persistedSnapshot) {
	err := savePersistedSnapshot(snap)
	e.mu.Lock()
	e.applyPersistResultLocked(snap.seq, err)
	e.mu.Unlock()
}

// applyPersistResultLocked updates persistError only for snapshots that are not
// older than the last reported generation. A delayed stale save that returns
// nil must not clear a newer failure.
func (e *inspectionEngine) applyPersistResultLocked(seq uint64, err error) {
	if seq != 0 && seq < e.persistStatusSeq {
		return
	}
	if seq > e.persistStatusSeq {
		e.persistStatusSeq = seq
	}
	if err != nil {
		e.persistError = err.Error()
	} else {
		e.persistError = ""
	}
}

func (e *inspectionEngine) bumpResultsLocked() {
	e.resultsGen++
}

func summarizeResults(results []accountResult) map[string]int {
	summary := map[string]int{
		"total":             len(results),
		"healthy":           0,
		"permission_denied": 0,
		"quota_exhausted":   0,
		"spending_limit":    0,
		"reauth":            0,
		"other":             0,
	}
	for _, item := range results {
		switch item.Classification {
		case "healthy":
			summary["healthy"]++
		case "permission_denied":
			summary["permission_denied"]++
		case "quota_exhausted":
			summary["quota_exhausted"]++
		case "spending_limit":
			summary["spending_limit"]++
		case "reauth":
			summary["reauth"]++
		default:
			summary["other"]++
		}
	}
	return summary
}

// snapshot builds a status payload. When includeResults is false (light poll),
// Results is omitted so progress polling stays cheap with 1000+ accounts.
func (e *inspectionEngine) snapshot(includeResults bool) jobSnapshot {
	return e.snapshotWithLang(includeResults, "")
}

func (e *inspectionEngine) snapshotWithLang(includeResults bool, lang string) jobSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	snap := e.snapshotLocked(includeResults)
	if strings.TrimSpace(lang) == "" {
		return snap
	}
	l := normalizeLang(lang)
	// Results are heavy; only rewrite when the client asked for the full list.
	if includeResults {
		for i := range snap.Results {
			snap.Results[i].Reason = localizeKnownReason(l, snap.Results[i].Reason)
		}
	}
	// Light status still needs current-language apply/action diagnostics.
	snap.ApplyCurrent = localizeKnownActionError(l, snap.ApplyCurrent)
	for i := range snap.ApplyFailures {
		snap.ApplyFailures[i] = localizeKnownActionError(l, snap.ApplyFailures[i])
	}
	for i := range snap.RecentRowActions {
		if snap.RecentRowActions[i].Error != "" {
			snap.RecentRowActions[i].Error = localizeKnownActionError(l, snap.RecentRowActions[i].Error)
		}
	}
	return snap
}

func (e *inspectionEngine) snapshotLocked(includeResults bool) jobSnapshot {
	summary := summarizeResults(e.results)
	snap := jobSnapshot{
		Running:          e.running,
		Stopped:          e.stopped && !e.running,
		Applying:         e.applying || e.applyDraining,
		Incremental:      e.incremental,
		Classifications:  append([]string(nil), e.classifications...),
		Done:             e.probeDone,
		Total:            e.total,
		Workers:          e.workers,
		ProbePhase:       e.probePhase,
		RetryDone:        e.retryDone,
		RetryTotal:       e.retryTotal,
		RetryWorkers:     e.retryWorkers,
		IncludeDisabled:  e.includeDisabled,
		OnlyDisabled:     e.onlyDisabled,
		ApplyDone:        e.applyDone,
		ApplyTotal:       e.applyTotal,
		ApplyCurrent:     e.applyCurrent,
		ApplyFailures:    append([]string(nil), e.applyFailures...),
		ActionInFlight:   e.actionInFlight,
		RecentRowActions: append([]rowActionReport(nil), e.recentRowActions...),
		Summary:          summary,
		StorePath:        storeFilePath(),
		ResultsGen:       e.resultsGen,
		IncludeResults:   includeResults,
		PersistError:     e.persistError,
		Unban:            unbanJobStatus(),
		Schedule:         e.schedule,
	}
	if includeResults {
		snap.Results = append([]accountResult(nil), e.results...)
	}
	if !e.startedAt.IsZero() {
		snap.StartedAt = e.startedAt.Format(time.RFC3339)
	}
	if !e.finishedAt.IsZero() {
		snap.FinishedAt = e.finishedAt.Format(time.RFC3339)
	}
	return snap
}

func (e *inspectionEngine) recordRowActionLocked(seq uint64, key, action string, errAction error) {
	rep := rowActionReport{
		Seq:    seq,
		Key:    key,
		Action: action,
		OK:     errAction == nil,
	}
	if errAction != nil {
		rep.Error = errAction.Error()
	}
	e.recentRowActions = append(e.recentRowActions, rep)
	if len(e.recentRowActions) > maxRecentRowActions {
		e.recentRowActions = append([]rowActionReport(nil), e.recentRowActions[len(e.recentRowActions)-maxRecentRowActions:]...)
	}
}

func (e *inspectionEngine) start(req startRequest) error {
	lang := normalizeLang(req.Lang)
	workers, errWorkers := normalizeWorkersLocalized(req.Workers, lang)
	if errWorkers != nil {
		return errWorkers
	}
	classifications := normalizeClassifications(req.Classifications)
	classifyScoped := len(classifications) > 0
	if classifyScoped && req.Incremental {
		return httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "category_with_incremental")))
	}

	e.mu.Lock()
	if e.running || e.applying || e.applyDraining {
		e.mu.Unlock()
		return httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "already_running")))
	}
	if e.actionInFlight > 0 {
		e.mu.Unlock()
		return httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_row_action")))
	}
	unbanJob.mu.Lock()
	unbanBusy := unbanJob.running
	unbanJob.mu.Unlock()
	if unbanBusy {
		e.mu.Unlock()
		return httpErr(http.StatusConflict, fmt.Errorf("%s", T(lang, "busy_unban")))
	}
	if req.Incremental && len(e.results) == 0 {
		e.mu.Unlock()
		return httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "incremental_needs_results")))
	}
	if classifyScoped && len(e.results) == 0 {
		e.mu.Unlock()
		return httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "category_needs_results")))
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
			return httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "no_accounts_in_category")))
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
	e.incremental = req.Incremental && !classifyScoped
	e.classifications = classifications
	e.workers = workers
	e.includeDisabled = includeDisabled
	e.onlyDisabled = onlyDisabled
	// Full inspect clears only after auth list succeeds (see run).
	e.fullClearPending = !req.Incremental && !classifyScoped
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
	return nil
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

func (e *inspectionEngine) shutdown() {
	// Stop background CPA dispose workers before waiting on other lifecycle gates.
	stopBanDisposeWorkers()
	stopUnbanJob()
	e.mu.Lock()
	e.stopped = true
	e.runID++
	e.applyRunID++
	e.applying = false
	e.applyDraining = true
	e.mu.Unlock()
	e.runWG.Wait()
	unbanJob.wg.Wait()
	// Async results.json writers must finish before TempDir/teardown.
	e.waitAsyncPersist()
	// Soft-timeout probes may still hold abandoned host.http.do callbacks.
	// Wait until they finish before clearing host/dlclose.
	waitHostCallsForShutdown(5 * time.Second)
	e.mu.Lock()
	e.applyDraining = false
	e.mu.Unlock()
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
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	// Final flush is synchronous so the last results survive process restart.
	e.saveSnapshotAndRecord(snap)
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

func (e *inspectionEngine) run(runID uint64, workers int, includeDisabled, onlyDisabled, incremental bool, classifications []string) {
	defer e.finish(runID)

	list, errList := callHostAuthListFn()
	if errList != nil {
		// Keep previous results on list failure (full inspect no longer wipes early).
		// Use a stable auth_index so repeated failures do not accumulate fake rows.
		write := e.upsertResult
		listReason := T(e.lang, "list_accounts_failed", errList.Error())
		if errors.Is(errList, errListAccountsTimeout) {
			listReason = T(e.lang, "list_accounts_timeout")
		}
		write(runID, accountResult{
			AuthIndex:      "__system_list_error__",
			Name:           "system",
			Classification: "probe_error",
			Action:         "keep",
			Reason:         listReason,
		})
		return
	}

	// Full inspect clear is deferred until runTargets are registered (same critical
	// section), so stop between list success and target commit cannot wipe history
	// without cancelled rows.
	classSet := stringSet(classifications)
	classifyScoped := len(classSet) > 0

	var known map[string]struct{}
	if incremental {
		e.mu.Lock()
		known = knownResultKeys(e.results)
		e.mu.Unlock()
	}

	var targets []pluginapi.HostAuthFileEntry
	var missing []accountResult
	if classifyScoped {
		e.mu.Lock()
		selected := make([]accountResult, 0)
		for _, item := range e.results {
			if classificationMatches(item.Classification, classSet) {
				selected = append(selected, item)
			}
		}
		e.mu.Unlock()
		targets, missing = resolveClassifyTargets(list.Files, selected)
		// Category re-probe matches the UI card: all accounts currently in that
		// classification. Do not apply includeDisabled/onlyDisabled here — those
		// options only scope full/incremental runs. Silently dropping disabled
		// rows made card counts disagree with actual probe totals.
		filtered := make([]pluginapi.HostAuthFileEntry, 0, len(targets))
		for _, file := range targets {
			if isXAIEntry(file.Provider, file.Name, file.Type) {
				filtered = append(filtered, file)
			}
		}
		targets = filtered
	} else if incremental {
		targets = filterNewAuthEntries(list.Files, known, includeDisabled, onlyDisabled)
	} else {
		targets = make([]pluginapi.HostAuthFileEntry, 0)
		for _, file := range list.Files {
			if shouldInspectEntry(file.Provider, file.Name, file.Type, file.Disabled, file.Status, includeDisabled, onlyDisabled) {
				targets = append(targets, file)
			}
		}
		sort.Slice(targets, func(i, j int) bool {
			return strings.ToLower(targets[i].Name) < strings.ToLower(targets[j].Name)
		})
	}

	writeResult := e.appendResult
	if classifyScoped {
		writeResult = e.upsertResult
	}

	// Resolve missing rows first (cheap) so stop/abort only needs to cancel live probe targets.
	for _, item := range missing {
		if e.isStopped(runID) {
			missed := item
			missed.Classification = "probe_error"
			missed.Action = "keep"
			missed.Reason = T(e.lang, "stopped_before_probe")
			missed.HTTPStatus = 0
			missed.ErrorCode = ""
			missed.ErrorMessage = ""
			writeResult(runID, missed)
			continue
		}
		missed := item
		missed.Classification = "probe_error"
		missed.Action = "keep"
		missed.Reason = T(e.lang, "account_missing")
		missed.HTTPStatus = 0
		missed.ErrorCode = ""
		missed.ErrorMessage = ""
		writeResult(runID, missed)
	}

	model := resolveSharedProbeModel(targets)
	e.mu.Lock()
	cont, cleared := e.commitRunPlanLocked(runID, targets, missing, model, classifyScoped)
	if cleared {
		e.persistLocked()
	}
	if !cont {
		snap := e.copyPersistedLocked()
		e.mu.Unlock()
		e.saveSnapshotAndRecord(snap)
		return
	}
	e.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var retryMu sync.Mutex
	retryTargets := make([]pluginapi.HostAuthFileEntry, 0)
	for i := 0; i < len(targets); i++ {
		if e.isStopped(runID) {
			// stop() already finalized progress; do not schedule more work.
			break
		}
		file := targets[i]
		wg.Add(1)
		sem <- struct{}{}
		go func(file pluginapi.HostAuthFileEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			if e.isStopped(runID) {
				// Aborted: stop() already wrote cancelled rows; discard this worker.
				return
			}
			result := inspectAccountFn(file, model, e.lang)
			writeResult(runID, result)
			if isAccountProbeTimeout(result) {
				retryMu.Lock()
				retryTargets = append(retryTargets, file)
				retryMu.Unlock()
			}
		}(file)
	}
	wg.Wait()

	if len(retryTargets) == 0 || e.isStopped(runID) {
		return
	}

	retryWorkers := slowRetryWorkers(workers)
	if !e.setRetryPhase(runID, len(retryTargets), retryWorkers) {
		return
	}
	retrySem := make(chan struct{}, retryWorkers)
	var retryWG sync.WaitGroup
	for _, file := range retryTargets {
		if e.isStopped(runID) {
			break
		}
		retryWG.Add(1)
		retrySem <- struct{}{}
		go func(file pluginapi.HostAuthFileEntry) {
			defer retryWG.Done()
			defer func() { <-retrySem }()
			if e.isStopped(runID) {
				return
			}
			e.replaceResult(runID, inspectAccountFn(file, model, e.lang))
		}(file)
	}
	retryWG.Wait()
}

func isAccountProbeTimeout(result accountResult) bool {
	if result.Classification != "probe_error" {
		return false
	}
	return isProbeTimeoutErr(fmt.Errorf("%s %s", result.Reason, result.ErrorMessage))
}
