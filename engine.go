package main

import (
	"fmt"
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
	deleteBatchSize = 50
)

type accountResult struct {
	AuthIndex      string `json:"auth_index"`
	Name           string `json:"name"`
	FileName       string `json:"file_name,omitempty"`
	Email          string `json:"email,omitempty"`
	// FileID / FileModUnix / FileSize help incremental skip without relying on email/name.
	FileID      string `json:"file_id,omitempty"`
	FileModUnix int64  `json:"file_mod_unix,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
	Disabled       bool   `json:"disabled"`
	Classification string `json:"classification"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Model          string `json:"model,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
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
	Running         bool            `json:"running"`
	Stopped         bool            `json:"stopped"`
	Applying        bool            `json:"applying"`
	Incremental     bool            `json:"incremental"`
	// Classifications is set when this run re-probes only matching last classifications.
	Classifications []string        `json:"classifications,omitempty"`
	Done            int             `json:"done"`
	Total           int             `json:"total"`
	Workers         int             `json:"workers"`
	IncludeDisabled bool            `json:"include_disabled"`
	OnlyDisabled    bool            `json:"only_disabled"`
	ApplyDone       int             `json:"apply_done"`
	ApplyTotal      int             `json:"apply_total"`
	ApplyCurrent    string          `json:"apply_current,omitempty"`
	ApplyFailures   []string        `json:"apply_failures,omitempty"`
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
	// Schedule is the background timer status (cheap; no host calls).
	Schedule *scheduleConfig `json:"schedule,omitempty"`
}

type startRequest struct {
	Workers         int  `json:"workers"`
	IncludeDisabled bool `json:"include_disabled"`
	OnlyDisabled    bool `json:"only_disabled"`
	// Incremental only probes Auth accounts not already present in the last results.
	Incremental bool `json:"incremental"`
	// Classifications re-probes only accounts whose last classification matches.
	// Keeps other results. Special token "other" matches non-primary classes
	// (not healthy / permission_denied / quota_exhausted / reauth).
	// Mutually exclusive with Incremental.
	Classifications []string `json:"classifications"`
}

type applyRequest struct {
	// empty AuthIndexes means apply all matching recommended actions (when ForceAction empty)
	AuthIndexes     []string `json:"auth_indexes"`
	Actions         []string `json:"actions"`         // optional: disable/enable/delete (recommended only)
	Classifications []string `json:"classifications"` // optional: reauth/healthy/...
	// ForceAction overrides recommended action for selected accounts.
	// Used by filter-based bulk disable/delete. Values: disable | enable | delete
	ForceAction string `json:"force_action"`
}

type actionRequest struct {
	AuthIndex string `json:"auth_index"`
	Name      string `json:"name"`
	Disabled  bool   `json:"disabled"`
	Delete    bool   `json:"delete"`
}

type authListResponse struct {
	Files []pluginapi.HostAuthFileEntry `json:"files"`
}

type inspectionEngine struct {
	mu              sync.Mutex
	runWG           sync.WaitGroup
	running         bool
	stopped         bool
	applying        bool
	actionInFlight  int // concurrent single-row enable/disable/delete goroutines
	actionSeq       uint64
	recentRowActions []rowActionReport // ring of latest completed single-row ops
	incremental     bool
	classifications []string // current/last scoped re-inspect classes
	runID           uint64
	workers         int
	includeDisabled bool
	onlyDisabled    bool
	total           int
	probeDone       int // probes completed in the current run (full or incremental)
	results         []accountResult
	applyDone       int
	applyTotal      int
	applyCurrent    string
	applyFailures   []string
	resultsGen      uint64 // monotonic; used by light /status clients
	startedAt       time.Time
	finishedAt      time.Time
	// Current-run bookkeeping for immediate stop (filled when targets are known).
	runTargets      []pluginapi.HostAuthFileEntry
	runModel        string
	runClassifyScoped bool
}

const maxRecentRowActions = 32

var engine = &inspectionEngine{workers: defaultWorkers}

func init() {
	engine.loadFromDisk()
	scheduler.init()
}

func normalizeWorkers(workers int) (int, error) {
	if workers == 0 {
		return defaultWorkers, nil
	}
	if workers < minWorkers || workers > maxWorkers {
		return 0, fmt.Errorf("workers must be an integer between %d and %d", minWorkers, maxWorkers)
	}
	return workers, nil
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
func (e *inspectionEngine) copyPersistedLocked() persistedSnapshot {
	snap := persistedSnapshot{
		Workers:         e.workers,
		IncludeDisabled: e.includeDisabled,
		OnlyDisabled:    e.onlyDisabled,
		Results:         append([]accountResult(nil), e.results...),
	}
	if !e.startedAt.IsZero() {
		snap.StartedAt = e.startedAt.Format(time.RFC3339)
	}
	if !e.finishedAt.IsZero() {
		snap.FinishedAt = e.finishedAt.Format(time.RFC3339)
	}
	return snap
}

// persistLocked copies under the caller lock, then writes asynchronously so
// status/snapshot callers are not blocked on disk I/O for large result lists.
func (e *inspectionEngine) persistLocked() {
	snap := e.copyPersistedLocked()
	go func(s persistedSnapshot) {
		_ = savePersistedSnapshot(s)
	}(snap)
}

// persist copies under lock and writes outside the critical section.
func (e *inspectionEngine) persist() {
	e.mu.Lock()
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	_ = savePersistedSnapshot(snap)
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
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.snapshotLocked(includeResults)
}

func (e *inspectionEngine) snapshotLocked(includeResults bool) jobSnapshot {
	summary := summarizeResults(e.results)
	sched := scheduler.snapshot()
	snap := jobSnapshot{
		Running:          e.running,
		Stopped:          e.stopped && !e.running,
		Applying:         e.applying,
		Incremental:      e.incremental,
		Classifications:  append([]string(nil), e.classifications...),
		Done:             e.probeDone,
		Total:            e.total,
		Workers:          e.workers,
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
		Schedule:         &sched,
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
	workers, errWorkers := normalizeWorkers(req.Workers)
	if errWorkers != nil {
		return errWorkers
	}
	classifications := normalizeClassifications(req.Classifications)
	classifyScoped := len(classifications) > 0
	if classifyScoped && req.Incremental {
		return fmt.Errorf("分类巡检不能与增量巡检同时使用")
	}

	e.mu.Lock()
	if e.running || e.applying {
		e.mu.Unlock()
		return fmt.Errorf("inspection already running")
	}
	if e.actionInFlight > 0 {
		e.mu.Unlock()
		return fmt.Errorf("busy: row action in progress")
	}
	if req.Incremental && len(e.results) == 0 {
		e.mu.Unlock()
		return fmt.Errorf("增量巡检需要已有结果，请先完整巡检")
	}
	if classifyScoped && len(e.results) == 0 {
		e.mu.Unlock()
		return fmt.Errorf("分类巡检需要已有结果，请先完整巡检")
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
			return fmt.Errorf("当前分类下没有可巡检账号")
		}
	}
	includeDisabled := req.IncludeDisabled
	onlyDisabled := req.OnlyDisabled
	if onlyDisabled {
		includeDisabled = false
	}
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
	// Do NOT clear results here for full inspect.
	// Clearing at start() used to wipe the UI (and results.json) before auth
	// list was fetched; if listing failed or returned 0 targets, the page
	// showed "0 accounts" even though CPA still had hundreds of auth files.
	// Full-inspect clear happens in run() only after the host auth list succeeds.
	e.total = 0
	e.probeDone = 0
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
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		e.run(runID, workers, includeDisabled, onlyDisabled, incremental, classifications)
	}()
	return nil
}

// stop aborts the job immediately for the UI:
// running flips false now, unfinished targets become "已停止，未探测",
// and in-flight probe results are discarded (run continues draining in background).
func (e *inspectionEngine) stop() {
	e.mu.Lock()
	e.stopped = true
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.abortRunLocked()
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	_ = savePersistedSnapshot(snap)
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
		item := cancelledAccountResult(file, model)
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
	e.finishedAt = time.Now()
	e.runTargets = nil
	e.bumpResultsLocked()
	// Invalidate in-flight writers from this run.
	e.runID++
}

func (e *inspectionEngine) shutdown() {
	scheduler.shutdown()
	e.mu.Lock()
	e.stopped = true
	e.runID++
	e.mu.Unlock()
	e.runWG.Wait()
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
	e.finishedAt = time.Now()
	snap := e.copyPersistedLocked()
	e.mu.Unlock()
	// Final flush is synchronous so the last results survive process restart.
	_ = savePersistedSnapshot(snap)
}

// knownResultKeys builds skip-keys for incremental inspect.
// Prefer stable auth_index only. Never use email/display name alone (re-import
// with a new token would incorrectly skip). Without auth_index, fall back to
// file_name + size + mtime (or file id) so a rewritten file is re-probed.
func (e *inspectionEngine) run(runID uint64, workers int, includeDisabled, onlyDisabled, incremental bool, classifications []string) {
	defer e.finish(runID)

	list, errList := callHostAuthList()
	if errList != nil {
		// Keep previous results on list failure so the UI does not drop to 0.
		write := e.appendResult
		if len(classifications) > 0 {
			write = e.upsertResult
		}
		write(runID, accountResult{
			Name:           "system",
			Classification: "probe_error",
			Action:         "keep",
			Reason:         "列出账号失败: " + errList.Error(),
		})
		return
	}

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

	// Full inspect: clear previous rows only after we successfully listed auths
	// and built the target set. This avoids wiping results.json / UI on failures
	// or brief empty host states during CPA restart.
	if !incremental && !classifyScoped {
		e.mu.Lock()
		if e.runID == runID {
			e.results = nil
			e.probeDone = 0
			e.bumpResultsLocked()
			// Persist the intentional clear only once list succeeded.
			e.persistLocked()
		}
		e.mu.Unlock()
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
			missed.Reason = "已停止，未探测"
			missed.HTTPStatus = 0
			missed.ErrorCode = ""
			missed.ErrorMessage = ""
			writeResult(runID, missed)
			continue
		}
		missed := item
		missed.Classification = "probe_error"
		missed.Action = "keep"
		missed.Reason = "Auth 列表中已不存在该账号"
		missed.HTTPStatus = 0
		missed.ErrorCode = ""
		missed.ErrorMessage = ""
		writeResult(runID, missed)
	}

	model := resolveSharedProbeModel(targets)
	e.mu.Lock()
	if e.runID == runID {
		if classifyScoped {
			e.total = len(targets) + len(missing)
		} else {
			e.total = len(targets)
		}
		// Only probe targets are cancellable via immediate stop; missing already written.
		e.runTargets = append([]pluginapi.HostAuthFileEntry(nil), targets...)
		e.runModel = model
		e.runClassifyScoped = classifyScoped
		if e.stopped {
			e.abortRunLocked()
			snap := e.copyPersistedLocked()
			e.mu.Unlock()
			_ = savePersistedSnapshot(snap)
			return
		}
	}
	e.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
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
			result := inspectAccount(file, model)
			writeResult(runID, result)
		}(file)
	}
	wg.Wait()
}

