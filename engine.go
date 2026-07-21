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
	// Schedule is the periodic inspect + auto-apply configuration (no secrets).
	Schedule scheduleStatus `json:"schedule"`
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
	mu               sync.Mutex
	runWG            sync.WaitGroup
	running          bool
	stopped          bool
	applying         bool
	actionInFlight   int // concurrent single-row enable/disable/delete goroutines
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
	startedAt        time.Time
	finishedAt       time.Time
	// Current-run bookkeeping for immediate stop (filled when targets are known).
	runTargets        []pluginapi.HostAuthFileEntry
	runModel          string
	runClassifyScoped bool

	// Periodic schedule (password kept in memory only).
	schedule          persistedSchedule
	schedulePassword  string
	scheduleGen       uint64
	schedulerStarted  bool
	schedulerStop     chan struct{}
	pendingAutoApply  bool // set by startScheduled; consumed when a run actually starts
	autoApplyAfterRun bool
}

const maxRecentRowActions = 32

var engine = &inspectionEngine{
	workers:       defaultWorkers,
	schedulerStop: make(chan struct{}),
	schedule: persistedSchedule{
		IntervalMinutes: defaultScheduleIntervalMin,
		Workers:         defaultWorkers,
		// Default filters when schedule is first enabled from API without body fields.
		IncludeDisabled: true,
		AutoApply:       true,
	},
}

var (
	callHostAuthListFn = callHostAuthList
	inspectAccountFn   = inspectAccount
)

func init() {
	engine.loadFromDisk()
	engine.ensureSchedulerLoop()
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
	if snap.Schedule != nil {
		s := *snap.Schedule
		if s.IntervalMinutes < minScheduleIntervalMin || s.IntervalMinutes > maxScheduleIntervalMin {
			s.IntervalMinutes = defaultScheduleIntervalMin
		}
		if s.Workers < minWorkers || s.Workers > maxWorkers {
			s.Workers = defaultWorkers
		}
		// Keep last_* fields; if enabled and next is empty/past, fire soon (loop handles it).
		e.schedule = s
	}
}

// copyPersistedLocked builds a disk snapshot while the caller holds e.mu.
func (e *inspectionEngine) copyPersistedLocked() persistedSnapshot {
	sched := e.schedule
	snap := persistedSnapshot{
		Workers:         e.workers,
		IncludeDisabled: e.includeDisabled,
		OnlyDisabled:    e.onlyDisabled,
		Results:         append([]accountResult(nil), e.results...),
		Schedule:        &sched,
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
	snap := jobSnapshot{
		Running:          e.running,
		Stopped:          e.stopped && !e.running,
		Applying:         e.applying,
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
	snap.Schedule = e.scheduleSnapshotLocked()
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
	// Consume one-shot auto-apply flag (set by startScheduled; manual start stays false).
	e.autoApplyAfterRun = e.pendingAutoApply
	e.pendingAutoApply = false
	e.runTargets = nil
	e.runModel = ""
	e.runClassifyScoped = false
	e.incremental = req.Incremental && !classifyScoped
	e.classifications = classifications
	e.workers = workers
	e.includeDisabled = includeDisabled
	e.onlyDisabled = onlyDisabled
	// Full inspect clears; incremental and classify-scoped keep existing rows.
	if !req.Incremental && !classifyScoped {
		e.results = nil
		e.bumpResultsLocked()
	}
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
	e.probePhase = "stopped"
	e.finishedAt = time.Now()
	e.runTargets = nil
	e.bumpResultsLocked()
	// Invalidate in-flight writers from this run.
	e.runID++
}

func (e *inspectionEngine) shutdown() {
	e.mu.Lock()
	e.stopped = true
	e.runID++
	e.schedule.Enabled = false
	e.autoApplyAfterRun = false
	if e.schedulerStop != nil {
		select {
		case <-e.schedulerStop:
		default:
			close(e.schedulerStop)
		}
	}
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
	// Capture auto-apply intent before unlocking; trigger after disk write.
	wantAuto := e.autoApplyAfterRun
	e.mu.Unlock()
	// Final flush is synchronous so the last results survive process restart.
	_ = savePersistedSnapshot(snap)
	if wantAuto {
		e.triggerAutoApplyIfNeeded(runID)
	}
}

// knownResultKeys builds skip-keys for incremental inspect.
// Prefer stable auth_index only. Never use email/display name alone (re-import
// with a new token would incorrectly skip). Without auth_index, fall back to
// file_name + size + mtime (or file id) so a rewritten file is re-probed.
func (e *inspectionEngine) run(runID uint64, workers int, includeDisabled, onlyDisabled, incremental bool, classifications []string) {
	defer e.finish(runID)

	list, errList := callHostAuthListFn()
	if errList != nil {
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
		e.probePhase = "primary"
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
			result := inspectAccountFn(file, model)
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
			e.replaceResult(runID, inspectAccountFn(file, model))
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
