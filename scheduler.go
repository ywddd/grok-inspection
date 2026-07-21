package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultScheduleIntervalMin = 10
	minScheduleIntervalMin     = 1
	maxScheduleIntervalMin     = 24 * 60
)

// scheduleRequest is the body for PUT /schedule.
type scheduleRequest struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
	// AutoApply applies the fixed policy after each scheduled inspect finishes.
	// Policy: permission_denied→delete, quota_exhausted→disable, healthy+disabled→enable.
	AutoApply       bool `json:"auto_apply"`
	Workers         int  `json:"workers"`
	IncludeDisabled bool `json:"include_disabled"`
	OnlyDisabled    bool `json:"only_disabled"`
}

// scheduleStatus is exposed on GET /status and GET /schedule (no secrets).
type scheduleStatus struct {
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
	AutoApply       bool   `json:"auto_apply"`
	Workers         int    `json:"workers"`
	IncludeDisabled bool   `json:"include_disabled"`
	OnlyDisabled    bool   `json:"only_disabled"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	NextRunAt       string `json:"next_run_at,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	LastAutoSummary string `json:"last_auto_summary,omitempty"`
	// HasPassword is true when a Management Key is available (env or previously captured).
	HasPassword bool `json:"has_password"`
}

func normalizeScheduleInterval(minutes int) (int, error) {
	if minutes == 0 {
		return defaultScheduleIntervalMin, nil
	}
	if minutes < minScheduleIntervalMin || minutes > maxScheduleIntervalMin {
		return 0, fmt.Errorf("interval_minutes must be between %d and %d", minScheduleIntervalMin, maxScheduleIntervalMin)
	}
	return minutes, nil
}

// autoPolicyAction returns the forced action for the fixed auto-heal policy, or "" to skip.
// Independent of the UI "recommended action" so reauth/probe_error are never auto-touched.
func autoPolicyAction(item accountResult) string {
	switch item.Classification {
	case "permission_denied":
		return "delete"
	case "quota_exhausted":
		if item.Disabled {
			return ""
		}
		return "disable"
	case "healthy":
		if item.Disabled {
			return "enable"
		}
		return ""
	default:
		return ""
	}
}

func collectAutoPolicyCandidates(results []accountResult) []accountResult {
	out := make([]accountResult, 0)
	for _, item := range results {
		action := autoPolicyAction(item)
		if action == "" {
			continue
		}
		copied := item
		copied.Action = action
		out = append(out, copied)
	}
	return out
}

func summarizeAutoCandidates(candidates []accountResult) string {
	var del, dis, en int
	for _, item := range candidates {
		switch item.Action {
		case "delete":
			del++
		case "disable":
			dis++
		case "enable":
			en++
		}
	}
	return fmt.Sprintf("delete=%d disable=%d enable=%d total=%d", del, dis, en, len(candidates))
}

func (e *inspectionEngine) scheduleSnapshotLocked() scheduleStatus {
	s := e.schedule
	if s.IntervalMinutes <= 0 {
		s.IntervalMinutes = defaultScheduleIntervalMin
	}
	if s.Workers <= 0 {
		s.Workers = defaultWorkers
	}
	st := scheduleStatus{
		Enabled:         s.Enabled,
		IntervalMinutes: s.IntervalMinutes,
		AutoApply:       s.AutoApply,
		Workers:         s.Workers,
		IncludeDisabled: s.IncludeDisabled,
		OnlyDisabled:    s.OnlyDisabled,
		LastRunAt:       s.LastRunAt,
		NextRunAt:       s.NextRunAt,
		LastError:       s.LastError,
		LastAutoSummary: s.LastAutoSummary,
		HasPassword:     strings.TrimSpace(e.schedulePassword) != "" || strings.TrimSpace(cpaManagementPassword()) != "",
	}
	return st
}

func (e *inspectionEngine) getSchedule() scheduleStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.scheduleSnapshotLocked()
}

// setSchedule updates the schedule and (re)starts the ticker when enabled.
// password from the request is kept in memory only (not persisted).
func (e *inspectionEngine) setSchedule(req scheduleRequest, password string) (scheduleStatus, error) {
	interval, errInterval := normalizeScheduleInterval(req.IntervalMinutes)
	if errInterval != nil {
		return scheduleStatus{}, errInterval
	}
	workers, errWorkers := normalizeWorkers(req.Workers)
	if errWorkers != nil {
		return scheduleStatus{}, errWorkers
	}
	includeDisabled := req.IncludeDisabled
	onlyDisabled := req.OnlyDisabled
	if onlyDisabled {
		includeDisabled = false
	}
	// When enabling schedule for the first time with zero-value body, default to
	// include disabled accounts so healthy-but-disabled can be re-enabled.
	if req.Enabled && !req.OnlyDisabled && !req.IncludeDisabled {
		// If the client explicitly sent include_disabled:false it is still false;
		// we only auto-default when both filters are zero (typical first enable).
		// UI always sends include_disabled explicitly, so this is a safety net for API clients.
	}

	password = strings.TrimSpace(password)
	e.mu.Lock()
	if password != "" {
		e.schedulePassword = password
	}
	e.schedule.Enabled = req.Enabled
	e.schedule.IntervalMinutes = interval
	// Default auto_apply to true when enabling unless client set it false intentionally.
	// JSON false is valid — we trust the request field as-is (UI sends true by default).
	e.schedule.AutoApply = req.AutoApply
	e.schedule.Workers = workers
	e.schedule.IncludeDisabled = includeDisabled
	e.schedule.OnlyDisabled = onlyDisabled
	e.schedule.LastError = ""
	if req.Enabled {
		// Fire as soon as the 15s poll notices (then advance by interval).
		e.schedule.NextRunAt = time.Now().Format(time.RFC3339)
	} else {
		e.schedule.NextRunAt = ""
	}
	// Bump generation so the loop reloads timer promptly.
	e.scheduleGen++
	gen := e.scheduleGen
	snap := e.copyPersistedLocked()
	status := e.scheduleSnapshotLocked()
	e.mu.Unlock()

	_ = savePersistedSnapshot(snap)
	e.ensureSchedulerLoop()
	_ = gen
	return status, nil
}

func (e *inspectionEngine) ensureSchedulerLoop() {
	e.mu.Lock()
	if e.schedulerStarted {
		e.mu.Unlock()
		return
	}
	e.schedulerStarted = true
	e.mu.Unlock()
	go e.schedulerLoop()
}

func (e *inspectionEngine) schedulerLoop() {
	// Poll every 15s so interval changes apply without long wait; actual fire uses NextRunAt.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-e.schedulerStop:
			return
		case <-ticker.C:
			e.maybeFireSchedule()
		}
	}
}

func (e *inspectionEngine) maybeFireSchedule() {
	e.mu.Lock()
	if !e.schedule.Enabled {
		e.mu.Unlock()
		return
	}
	if e.running || e.applying || e.actionInFlight > 0 {
		e.mu.Unlock()
		return
	}
	interval := e.schedule.IntervalMinutes
	if interval <= 0 {
		interval = defaultScheduleIntervalMin
	}
	nextRaw := e.schedule.NextRunAt
	var next time.Time
	if nextRaw != "" {
		if t, err := time.Parse(time.RFC3339, nextRaw); err == nil {
			next = t
		}
	}
	now := time.Now()
	if !next.IsZero() && now.Before(next) {
		e.mu.Unlock()
		return
	}
	// Due: mark next slot first so a long inspect does not double-fire.
	e.schedule.LastRunAt = now.Format(time.RFC3339)
	e.schedule.NextRunAt = now.Add(time.Duration(interval) * time.Minute).Format(time.RFC3339)
	e.schedule.LastError = ""
	workers := e.schedule.Workers
	includeDisabled := e.schedule.IncludeDisabled
	onlyDisabled := e.schedule.OnlyDisabled
	autoApply := e.schedule.AutoApply
	// Scheduled inspect always wants disabled accounts when auto-enabling healthy ones.
	if autoApply && !onlyDisabled {
		includeDisabled = true
	}
	e.mu.Unlock()

	req := startRequest{
		Workers:         workers,
		IncludeDisabled: includeDisabled,
		OnlyDisabled:    onlyDisabled,
		Incremental:     false,
	}
	if err := e.startScheduled(req, autoApply); err != nil {
		e.mu.Lock()
		e.schedule.LastError = err.Error()
		e.persistLocked()
		e.mu.Unlock()
	}
}

// startScheduled is like start() but tags the run for optional auto-apply on finish.
func (e *inspectionEngine) startScheduled(req startRequest, autoApply bool) error {
	e.mu.Lock()
	e.pendingAutoApply = autoApply
	e.mu.Unlock()
	if err := e.start(req); err != nil {
		e.mu.Lock()
		e.pendingAutoApply = false
		e.autoApplyAfterRun = false
		e.mu.Unlock()
		return err
	}
	return nil
}

// triggerAutoApplyIfNeeded is called from finish() after a successful scheduled inspect.
func (e *inspectionEngine) triggerAutoApplyIfNeeded(runID uint64) {
	e.mu.Lock()
	if e.runID != runID {
		e.mu.Unlock()
		return
	}
	if !e.autoApplyAfterRun {
		e.mu.Unlock()
		return
	}
	e.autoApplyAfterRun = false
	if e.stopped {
		e.mu.Unlock()
		return
	}
	candidates := collectAutoPolicyCandidates(e.results)
	password := firstNonEmpty(e.schedulePassword, cpaManagementPassword())
	summary := summarizeAutoCandidates(candidates)
	e.schedule.LastAutoSummary = summary
	if len(candidates) == 0 {
		e.persistLocked()
		e.mu.Unlock()
		return
	}
	if password == "" {
		e.schedule.LastError = "auto_apply skipped: no management password (set MANAGEMENT_PASSWORD or enable schedule from UI with Key)"
		e.persistLocked()
		e.mu.Unlock()
		return
	}
	// Re-use startApply path under lock carefully: startApply takes the lock itself.
	e.mu.Unlock()

	// Build force-style candidates already have Action set; feed runApply via startApply-like path.
	if err := e.startAutoApply(candidates, password, nil); err != nil {
		e.mu.Lock()
		e.schedule.LastError = "auto_apply: " + err.Error()
		e.persistLocked()
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	e.schedule.LastError = ""
	e.persistLocked()
	e.mu.Unlock()
}

// startAutoApply runs policy actions without requiring force_action auth_indexes.
func (e *inspectionEngine) startAutoApply(candidates []accountResult, password string, headers http.Header) error {
	e.mu.Lock()
	if e.running || e.applying || e.actionInFlight > 0 {
		e.mu.Unlock()
		return fmt.Errorf("busy")
	}
	if len(candidates) == 0 {
		e.mu.Unlock()
		return nil
	}
	e.applying = true
	e.applyDone = 0
	e.applyTotal = len(candidates)
	e.applyCurrent = ""
	e.applyFailures = nil
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
