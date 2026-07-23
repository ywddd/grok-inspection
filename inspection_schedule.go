package main

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// schedulePersistError is returned when schedule.json could not be written.
// Callers must not treat message text as a protocol.
type schedulePersistError struct{ err error }

func (e *schedulePersistError) Error() string {
	if e == nil || e.err == nil {
		return "schedule persist failed"
	}
	return e.err.Error()
}
func (e *schedulePersistError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// scheduleTxnMu serializes user schedule updates and runtime status
// write-back (read -> build -> persist -> memory commit). Disk I/O must
// not run while engine.mu is held.
var scheduleTxnMu sync.Mutex

const (
	defaultInspectionScheduleIntervalMinutes = 60
	minInspectionScheduleIntervalMinutes     = 1
	maxInspectionScheduleIntervalMinutes     = 7 * 24 * 60

	scheduled403Disable = "disable"
	scheduled403Delete  = "delete"
	scheduled402Disable = "disable"
	scheduled402Delete  = "delete"
)

type persistedInspectionSchedule struct {
	Enabled                bool   `json:"enabled"`
	IntervalMinutes        int    `json:"interval_minutes"`
	Workers                int    `json:"workers"`
	IncludeDisabled        bool   `json:"include_disabled"`
	OnlyDisabled           bool   `json:"only_disabled,omitempty"`
	PermissionDeniedAction string `json:"permission_denied_action"`
	SpendingLimitAction    string `json:"spending_limit_action"`
	LastRunAt              string `json:"last_run_at,omitempty"`
	NextRunAt              string `json:"next_run_at,omitempty"`
	LastStatus             string `json:"last_status,omitempty"`
	LastError              string `json:"last_error,omitempty"`
	LastMatched            int    `json:"last_matched,omitempty"`
	LastDisabled           int    `json:"last_disabled,omitempty"`
	LastDeleted            int    `json:"last_deleted,omitempty"`
	LastFailed             int    `json:"last_failed,omitempty"`
	LastMatched403         int    `json:"last_matched_403,omitempty"`
	LastDisabled403        int    `json:"last_disabled_403,omitempty"`
	LastDeleted403         int    `json:"last_deleted_403,omitempty"`
	LastFailed403          int    `json:"last_failed_403,omitempty"`
	LastMatched402         int    `json:"last_matched_402,omitempty"`
	LastDisabled402        int    `json:"last_disabled_402,omitempty"`
	LastDeleted402         int    `json:"last_deleted_402,omitempty"`
	LastFailed402          int    `json:"last_failed_402,omitempty"`
}

type inspectionScheduleUpdate struct {
	Enabled                *bool   `json:"enabled"`
	IntervalMinutes        *int    `json:"interval_minutes"`
	Workers                *int    `json:"workers"`
	IncludeDisabled        *bool   `json:"include_disabled"`
	PermissionDeniedAction *string `json:"permission_denied_action"`
	SpendingLimitAction    *string `json:"spending_limit_action"`
}

var inspectionScheduleRuntime = struct {
	once     sync.Once
	stopOnce sync.Once
	wake     chan struct{}
	stop     chan struct{}
	done     chan struct{}
}{
	wake: make(chan struct{}, 1),
	stop: make(chan struct{}),
	done: make(chan struct{}),
}

func defaultInspectionSchedule() persistedInspectionSchedule {
	return persistedInspectionSchedule{
		IntervalMinutes:        defaultInspectionScheduleIntervalMinutes,
		Workers:                defaultWorkers,
		PermissionDeniedAction: scheduled403Disable,
		SpendingLimitAction:    scheduled402Disable,
	}
}

func normalizeInspectionScheduleInterval(minutes int) (int, error) {
	if minutes == 0 {
		return defaultInspectionScheduleIntervalMinutes, nil
	}
	if minutes < minInspectionScheduleIntervalMinutes || minutes > maxInspectionScheduleIntervalMinutes {
		return 0, fmt.Errorf("interval_minutes must be between %d and %d", minInspectionScheduleIntervalMinutes, maxInspectionScheduleIntervalMinutes)
	}
	return minutes, nil
}

func normalizeScheduled403Action(action string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return scheduled403Disable, nil
	}
	switch action {
	case scheduled403Disable, scheduled403Delete:
		return action, nil
	default:
		return "", fmt.Errorf("permission_denied_action must be disable or delete")
	}
}

func normalizeScheduled402Action(action string) (string, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return scheduled402Disable, nil
	}
	switch action {
	case scheduled402Disable, scheduled402Delete:
		return action, nil
	default:
		return "", fmt.Errorf("spending_limit_action must be disable or delete")
	}
}

func normalizePersistedInspectionSchedule(cfg persistedInspectionSchedule) persistedInspectionSchedule {
	defaults := defaultInspectionSchedule()
	if interval, err := normalizeInspectionScheduleInterval(cfg.IntervalMinutes); err == nil {
		cfg.IntervalMinutes = interval
	} else {
		cfg.IntervalMinutes = defaults.IntervalMinutes
	}
	if workers, err := normalizeWorkers(cfg.Workers); err == nil {
		cfg.Workers = workers
	} else {
		cfg.Workers = defaults.Workers
	}
	if action, err := normalizeScheduled403Action(cfg.PermissionDeniedAction); err == nil {
		cfg.PermissionDeniedAction = action
	} else {
		cfg.PermissionDeniedAction = defaults.PermissionDeniedAction
	}
	if action, err := normalizeScheduled402Action(cfg.SpendingLimitAction); err == nil {
		cfg.SpendingLimitAction = action
	} else {
		cfg.SpendingLimitAction = defaults.SpendingLimitAction
	}
	if !cfg.Enabled {
		cfg.NextRunAt = ""
	}
	return cfg
}

func inspectionScheduleDue(now, next time.Time) bool {
	return next.IsZero() || !now.Before(next)
}

func nextInspectionScheduleRun(now time.Time, intervalMinutes int) time.Time {
	interval, err := normalizeInspectionScheduleInterval(intervalMinutes)
	if err != nil {
		interval = defaultInspectionScheduleIntervalMinutes
	}
	return now.Add(time.Duration(interval) * time.Minute)
}

func scheduledInspectionRequest(cfg persistedInspectionSchedule) startRequest {
	workers, err := normalizeWorkers(cfg.Workers)
	if err != nil {
		workers = defaultWorkers
	}
	return startRequest{
		Lang:            "zh",
		Workers:         workers,
		IncludeDisabled: cfg.IncludeDisabled,
		OnlyDisabled:    false,
		Incremental:     false,
		Classifications: nil,
	}
}

func inspectionScheduleSnapshot() persistedInspectionSchedule {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return normalizePersistedInspectionSchedule(engine.schedule)
}

func updateInspectionSchedule(req inspectionScheduleUpdate) (persistedInspectionSchedule, error) {
	// Full user-config transaction: read -> build -> persist -> commit.
	// Serialized against concurrent POSTs and runtime status write-backs.
	scheduleTxnMu.Lock()
	defer scheduleTxnMu.Unlock()

	engine.mu.Lock()
	prev := normalizePersistedInspectionSchedule(engine.schedule)
	engine.mu.Unlock()

	cfg := prev
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.IntervalMinutes != nil {
		interval, err := normalizeInspectionScheduleInterval(*req.IntervalMinutes)
		if err != nil {
			return cfg, err
		}
		cfg.IntervalMinutes = interval
	}
	if req.Workers != nil {
		workers, err := normalizeWorkers(*req.Workers)
		if err != nil {
			return cfg, fmt.Errorf("workers must be between %d and %d", minWorkers, maxWorkers)
		}
		cfg.Workers = workers
	}
	if req.IncludeDisabled != nil {
		cfg.IncludeDisabled = *req.IncludeDisabled
	}
	if req.PermissionDeniedAction != nil {
		action, err := normalizeScheduled403Action(*req.PermissionDeniedAction)
		if err != nil {
			return cfg, err
		}
		cfg.PermissionDeniedAction = action
	}
	if req.SpendingLimitAction != nil {
		action, err := normalizeScheduled402Action(*req.SpendingLimitAction)
		if err != nil {
			return cfg, err
		}
		cfg.SpendingLimitAction = action
	}
	if cfg.Enabled {
		cfg.NextRunAt = nextInspectionScheduleRun(time.Now(), cfg.IntervalMinutes).Format(time.RFC3339)
		cfg.LastStatus = "waiting"
	} else {
		cfg.NextRunAt = ""
		cfg.LastStatus = "disabled"
	}
	cfg.LastError = ""
	// Persist before publishing to the run loop; no engine.mu during disk I/O.
	if err := saveInspectionScheduleSync(cfg); err != nil {
		return prev, &schedulePersistError{err: err}
	}
	engine.mu.Lock()
	engine.schedule = cfg
	engine.mu.Unlock()
	wakeInspectionScheduleLoop()
	return cfg, nil
}

func rememberInspectionScheduleManagementKey(key string) {
	// Back-compat alias: schedule routes share the plugin-level Management cache.
	rememberManagementCredential(key)
}

func inspectionScheduleManagementKey() string {
	return cpaManagementPasswordOrCached()
}

func inspectionScheduleStatus() map[string]any {
	cfg := inspectionScheduleSnapshot()
	return map[string]any{
		"enabled":                  cfg.Enabled,
		"interval_minutes":         cfg.IntervalMinutes,
		"workers":                  cfg.Workers,
		"include_disabled":         cfg.IncludeDisabled,
		"permission_denied_action": cfg.PermissionDeniedAction,
		"spending_limit_action":    cfg.SpendingLimitAction,
		"last_run_at":              cfg.LastRunAt,
		"next_run_at":              cfg.NextRunAt,
		"last_status":              cfg.LastStatus,
		"last_error":               cfg.LastError,
		"last_matched":             cfg.LastMatched,
		"last_disabled":            cfg.LastDisabled,
		"last_deleted":             cfg.LastDeleted,
		"last_failed":              cfg.LastFailed,
		"last_matched_403":         cfg.LastMatched403,
		"last_disabled_403":        cfg.LastDisabled403,
		"last_deleted_403":         cfg.LastDeleted403,
		"last_failed_403":          cfg.LastFailed403,
		"last_matched_402":         cfg.LastMatched402,
		"last_disabled_402":        cfg.LastDisabled402,
		"last_deleted_402":         cfg.LastDeleted402,
		"last_failed_402":          cfg.LastFailed402,
		"action_ready":             inspectionScheduleManagementKey() != "",
	}
}

func wakeInspectionScheduleLoop() {
	select {
	case inspectionScheduleRuntime.wake <- struct{}{}:
	default:
	}
}

func startInspectionScheduleLoop() {
	inspectionScheduleRuntime.once.Do(func() {
		go inspectionScheduleLoop()
	})
}

func stopInspectionScheduleLoop() {
	inspectionScheduleRuntime.stopOnce.Do(func() {
		startInspectionScheduleLoop()
		close(inspectionScheduleRuntime.stop)
		<-inspectionScheduleRuntime.done
	})
}

func inspectionScheduleLoop() {
	defer close(inspectionScheduleRuntime.done)
	for {
		cfg := inspectionScheduleSnapshot()
		if !cfg.Enabled {
			select {
			case <-inspectionScheduleRuntime.wake:
				continue
			case <-inspectionScheduleRuntime.stop:
				return
			}
		}

		next, _ := time.Parse(time.RFC3339, cfg.NextRunAt)
		now := time.Now()
		if !inspectionScheduleDue(now, next) {
			timer := time.NewTimer(time.Until(next))
			select {
			case <-timer.C:
			case <-inspectionScheduleRuntime.wake:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				continue
			case <-inspectionScheduleRuntime.stop:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			}
		}
		runScheduledInspection(cfg)
	}
}

type scheduledActionResult int

const (
	scheduledActionOK scheduledActionResult = iota
	scheduledActionFailed
	scheduledActionStopped
)

func runScheduledInspection(cfg persistedInspectionSchedule) {
	started := time.Now()
	setInspectionScheduleRuntimeStatus("running", "", started, scheduleRunStats{})
	if err := engine.start(scheduledInspectionRequest(cfg)); err != nil {
		setInspectionScheduleRuntimeStatus("skipped", err.Error(), started, scheduleRunStats{})
		return
	}
	// Capture this run's token immediately after start (no concurrent start while running).
	expectedRunID := engine.latestRunID()

	for {
		select {
		case <-inspectionScheduleRuntime.stop:
			setInspectionScheduleRuntimeStatus("stopped", "", started, scheduleRunStats{})
			return
		case <-time.After(250 * time.Millisecond):
		}
		snap := engine.snapshot(false)
		if !snap.Running {
			if snap.Stopped {
				setInspectionScheduleRuntimeStatus("stopped", "", started, scheduleRunStats{})
				return
			}
			break
		}
	}

	// Only act on this run's published outcome. Stale 402/403 rows from a prior
	// successful inspect must not drive disable/delete when this run failed to list.
	listOK, listError, fullInspect, found := engine.finishedRunOutcome(expectedRunID)
	if !found {
		setInspectionScheduleRuntimeStatus("failed", "inspection run outcome missing or superseded", started, scheduleRunStats{})
		return
	}
	if !fullInspect || !listOK {
		errMsg := listError
		if errMsg == "" {
			if !fullInspect {
				errMsg = "scheduled auto-actions require a successful full inspection"
			} else {
				errMsg = "account list was not obtained for this run"
			}
		}
		setInspectionScheduleRuntimeStatus("failed", errMsg, started, scheduleRunStats{})
		return
	}

	stats := scheduleRunStats{}
	password := inspectionScheduleManagementKey()
	runAction := func(action, errorCode string, targets []string, matched, disabled, deleted, failed *int) scheduledActionResult {
		if len(targets) == 0 {
			return scheduledActionOK
		}
		*matched = len(targets)
		if password == "" {
			*failed = len(targets)
			return scheduledActionFailed
		}
		req := applyRequest{
			Lang:         "zh",
			AuthIndexes:  targets,
			ForceAction:  action,
			BanErrorCode: errorCode,
		}
		if err := engine.startApply(req, password, nil); err != nil {
			if engine.snapshot(false).Stopped {
				return scheduledActionStopped
			}
			*failed = len(targets)
			stats.errors = append(stats.errors, err.Error())
			return scheduledActionFailed
		}
		for {
			select {
			case <-inspectionScheduleRuntime.stop:
				// Count progress so far, then stop sequential disposal.
				recordScheduledActionProgress(targets, action, disabled, deleted, failed)
				return scheduledActionStopped
			case <-time.After(250 * time.Millisecond):
			}
			snap := engine.snapshot(false)
			if !snap.Applying {
				if snap.Stopped {
					recordScheduledActionProgress(targets, action, disabled, deleted, failed)
					return scheduledActionStopped
				}
				break
			}
		}
		recordScheduledActionProgress(targets, action, disabled, deleted, failed)
		finalSnap := engine.snapshot(false)
		if len(finalSnap.ApplyFailures) > 0 {
			stats.errors = append(stats.errors, finalSnap.ApplyFailures...)
		}
		if *failed > 0 {
			return scheduledActionFailed
		}
		return scheduledActionOK
	}

	action403, err := normalizeScheduled403Action(cfg.PermissionDeniedAction)
	if err != nil {
		action403 = scheduled403Disable
	}
	action402, err := normalizeScheduled402Action(cfg.SpendingLimitAction)
	if err != nil {
		action402 = scheduled402Disable
	}
	result403 := runAction(action403, permissionDeniedErrorCode, scheduledPermissionDeniedTargets(action403),
		&stats.matched403, &stats.disabled403, &stats.deleted403, &stats.failed403)
	if result403 == scheduledActionStopped {
		setInspectionScheduleRuntimeStatus("stopped", strings.Join(stats.errors, "; "), started, stats)
		return
	}
	result402 := runAction(action402, spendingLimitErrorCode, scheduledSpendingLimitTargets(action402),
		&stats.matched402, &stats.disabled402, &stats.deleted402, &stats.failed402)
	if result402 == scheduledActionStopped {
		setInspectionScheduleRuntimeStatus("stopped", strings.Join(stats.errors, "; "), started, stats)
		return
	}

	if password == "" && stats.failed403+stats.failed402 > 0 {
		stats.errors = append(stats.errors, "CPA management password is unavailable")
	}
	if stats.matched403 == 0 && stats.matched402 == 0 {
		setInspectionScheduleRuntimeStatus("completed", "", started, stats)
		return
	}
	status := "completed"
	if stats.failed403+stats.failed402 > 0 {
		status = "completed_with_errors"
	}
	setInspectionScheduleRuntimeStatus(status, strings.Join(stats.errors, "; "), started, stats)
}

func recordScheduledActionProgress(targets []string, action string, disabled, deleted, failed *int) {
	succeeded := scheduledActionSuccessCount(targets, action)
	*failed = len(targets) - succeeded
	if action == scheduled403Delete || action == scheduled402Delete {
		*deleted = succeeded
	} else {
		*disabled = succeeded
	}
}

func scheduledPermissionDeniedTargets(action string) []string {
	return scheduledTargets(403, "permission_denied", permissionDeniedErrorCode, action)
}

func scheduledSpendingLimitTargets(action string) []string {
	return scheduledTargetsExact(402, "spending_limit", spendingLimitErrorCode, action)
}

func scheduledTargets(status int, classification, errorCode, action string) []string {
	return scheduledTargetsMatch(status, classification, errorCode, action, false)
}

func scheduledTargetsExact(status int, classification, errorCode, action string) []string {
	return scheduledTargetsMatch(status, classification, errorCode, action, true)
}

func scheduledTargetsMatch(status int, classification, errorCode, action string, exactCode bool) []string {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	targets := make([]string, 0)
	wantCode := strings.ToLower(strings.TrimSpace(errorCode))
	for _, item := range engine.results {
		code := strings.ToLower(strings.TrimSpace(item.ErrorCode))
		if item.HTTPStatus != status || item.Classification != classification {
			continue
		}
		if exactCode {
			if code != wantCode {
				continue
			}
		} else if code != wantCode && !strings.Contains(code, wantCode) {
			continue
		}
		if action == "disable" && item.Disabled {
			continue
		}
		if id := firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email); id != "" {
			targets = append(targets, id)
		}
	}
	return targets
}

func scheduledActionSuccessCount(targets []string, action string) int {
	wanted := stringSet(targets)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	matched := 0
	isDelete := action == scheduled403Delete || action == scheduled402Delete
	isDisable := action == scheduled403Disable || action == scheduled402Disable
	for _, item := range engine.results {
		if !itemSelected(item, wanted, nil) {
			continue
		}
		if isDelete || (isDisable && item.Disabled) {
			matched++
		}
	}
	if isDelete {
		return len(targets) - matched
	}
	return matched
}

type scheduleRunStats struct {
	matched403, disabled403, deleted403, failed403 int
	matched402, disabled402, deleted402, failed402 int
	errors                                         []string
}

func setInspectionScheduleRuntimeStatus(status, lastError string, started time.Time, stats scheduleRunStats) {
	// Same transaction lock as user updates so runtime write-back cannot clobber
	// a concurrent POST (or vice versa). Disk I/O stays outside engine.mu.
	scheduleTxnMu.Lock()
	defer scheduleTxnMu.Unlock()

	engine.mu.Lock()
	cfg := normalizePersistedInspectionSchedule(engine.schedule)
	engine.mu.Unlock()

	cfg.LastRunAt = started.Format(time.RFC3339)
	cfg.LastStatus = status
	cfg.LastError = strings.TrimSpace(lastError)
	cfg.LastMatched403 = stats.matched403
	cfg.LastDisabled403 = stats.disabled403
	cfg.LastDeleted403 = stats.deleted403
	cfg.LastFailed403 = stats.failed403
	cfg.LastMatched402 = stats.matched402
	cfg.LastDisabled402 = stats.disabled402
	cfg.LastDeleted402 = stats.deleted402
	cfg.LastFailed402 = stats.failed402
	cfg.LastMatched = stats.matched403 + stats.matched402
	cfg.LastDisabled = stats.disabled403 + stats.disabled402
	cfg.LastDeleted = stats.deleted403 + stats.deleted402
	cfg.LastFailed = stats.failed403 + stats.failed402
	if cfg.Enabled {
		cfg.NextRunAt = nextInspectionScheduleRun(time.Now(), cfg.IntervalMinutes).Format(time.RFC3339)
	} else {
		cfg.NextRunAt = ""
	}
	if err := saveInspectionScheduleSync(cfg); err != nil {
		slog.Warn("grok-inspection: failed to persist schedule runtime status", "error", err)
		// Keep memory consistent with last durable user settings if disk fails:
		// still commit runtime fields in memory so operators see last run, matching
		// prior behavior, but under the txn lock so it cannot race a user save.
	}
	engine.mu.Lock()
	engine.schedule = cfg
	engine.mu.Unlock()
}
