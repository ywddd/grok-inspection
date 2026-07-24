package main

import (
	"errors"
	"fmt"
	"net/http"
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
	// Sample is true when this run randomly probes a subset and keeps other results.
	Sample bool `json:"sample"`
	// SampleCount / SamplePercent are the request parameters for a sample run (0 = unset).
	SampleCount   int `json:"sample_count,omitempty"`
	SamplePercent int `json:"sample_percent,omitempty"`
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
	// Sample randomly probes a subset of the current target set and keeps other results.
	// Mutually exclusive with Incremental. May combine with Classifications.
	Sample bool `json:"sample"`
	// SampleCount is an absolute sample size (0 = unset). When both count and percent
	// are set, the engine uses the smaller resolved size.
	SampleCount int `json:"sample_count"`
	// SamplePercent is 1-100 (0 = unset).
	SamplePercent int `json:"sample_percent"`
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
	lang      Lang // operator-facing language for the current/last inspection run (default zh)
	applyLang Lang // language for the active bulk-apply job (request-scoped)
	stopLang  Lang // language of the latest user stop request (cancel/stop copy)
	mu        sync.Mutex
	runWG     sync.WaitGroup
	persistWG sync.WaitGroup // async persistLocked writers
	running   bool
	stopped   bool
	// shuttingDown is permanent plugin unload gate. Unlike stopped (user cancel),
	// it is never cleared by start/claim and blocks all runWG/unban WaitGroup Adds.
	shuttingDown     bool
	applying         bool
	applyDraining    bool   // bulk apply stopped but in-flight PATCHs still finishing
	applyRunID       uint64 // invalidates in-flight bulk apply on stop
	persistSeq       uint64 // monotonic snapshot id assigned at copy time
	actionInFlight   int    // concurrent single-row enable/disable/delete goroutines
	actionSeq        uint64
	recentRowActions []rowActionReport // ring of latest completed single-row ops
	incremental      bool
	sampleMode       bool
	sampleCount      int
	samplePercent    int
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
	runClassifyScoped bool // upsert/merge results category-or-sample
	// fullClearPending: full inspect waits for list success before wiping results.
	fullClearPending bool
	// Per-run list outcome (current run). Used so finish can publish a tokenized result.
	runIsFullInspect bool
	runListOK        bool
	runListError     string
	// Last finished run outcome (runID-tokenized). Schedule auto-actions must match this.
	lastFinishedRunID       uint64
	lastFinishedListOK      bool
	lastFinishedListError   string
	lastFinishedFullInspect bool
	schedule                persistedInspectionSchedule
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
