package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	scheduleStoreVersion   = 1
	defaultIntervalMinutes = 60
	minIntervalMinutes     = 5
	maxIntervalMinutes     = 24 * 60
	scheduleIdlePoll       = 2 * time.Second
	scheduleRunTimeout     = 6 * time.Hour
)

// scheduleConfig is the user-facing schedule settings + runtime status.
// Safety defaults: auto_apply off; delete not in default allowed actions.
// interval default 60 minutes; workers default = defaultWorkers (6); incremental default false (full run).
type scheduleConfig struct {
	Enabled          bool     `json:"enabled"`
	IntervalMinutes  int      `json:"interval_minutes"`
	Incremental      bool     `json:"incremental"`
	IncludeDisabled  bool     `json:"include_disabled"`
	OnlyDisabled     bool     `json:"only_disabled"`
	Workers          int      `json:"workers"`
	AutoApply        bool     `json:"auto_apply"`
	AutoApplyActions []string `json:"auto_apply_actions"` // disable | enable | delete
	// Auto-captured from the page Authorization header when schedule is saved via the
	// management panel, enabling headless auto_apply without MANAGEMENT_PASSWORD env.
	// Never returned in API responses (stripped in snapshot).
	ManagementPassword string `json:"management_password,omitempty"`
	// Runtime (read-only for clients; set by scheduler)
	NextRunAt      string `json:"next_run_at,omitempty"`
	LastRunAt      string `json:"last_run_at,omitempty"`
	LastFinishedAt string `json:"last_finished_at,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	LastApplyDone  int    `json:"last_apply_done,omitempty"`
	LastApplyTotal int    `json:"last_apply_total,omitempty"`
	LastApplyOK    bool   `json:"last_apply_ok,omitempty"`
	Running        bool   `json:"running,omitempty"` // scheduler tick in progress
}

type persistedSchedule struct {
	Version int            `json:"version"`
	Config  scheduleConfig `json:"config"`
	SavedAt string         `json:"saved_at"`
}

type scheduleManager struct {
	mu     sync.Mutex
	cfg    scheduleConfig
	stopCh chan struct{}
	wakeCh chan struct{}
	once   sync.Once
}

var scheduler = &scheduleManager{
	cfg:    defaultScheduleConfig(),
	stopCh: make(chan struct{}),
	wakeCh: make(chan struct{}, 1),
}

func defaultScheduleConfig() scheduleConfig {
	return scheduleConfig{
		Enabled:          false,
		IntervalMinutes:  defaultIntervalMinutes,
		Incremental:      false, // unchecked = full inspection
		IncludeDisabled:  false,
		OnlyDisabled:     false,
		Workers:          defaultWorkers, // 6
		AutoApply:        false,
		AutoApplyActions: []string{"disable", "enable"}, // delete excluded by default
	}
}

func scheduleFilePath() string {
	if dir := firstNonEmpty(os.Getenv("GROK_INSPECTION_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "schedule.json")
	}
	return filepath.Join("data", "grok-inspection", "schedule.json")
}

func normalizeScheduleConfig(in scheduleConfig) (scheduleConfig, error) {
	out := defaultScheduleConfig()
	out.Enabled = in.Enabled
	out.Incremental = in.Incremental
	out.IncludeDisabled = in.IncludeDisabled
	out.OnlyDisabled = in.OnlyDisabled
	if out.OnlyDisabled {
		out.IncludeDisabled = false
	}
	out.AutoApply = in.AutoApply

	interval := in.IntervalMinutes
	if interval == 0 {
		interval = defaultIntervalMinutes
	}
	if interval < minIntervalMinutes || interval > maxIntervalMinutes {
		return scheduleConfig{}, fmt.Errorf("interval_minutes must be between %d and %d", minIntervalMinutes, maxIntervalMinutes)
	}
	out.IntervalMinutes = interval

	workers, errWorkers := normalizeWorkers(in.Workers)
	if errWorkers != nil {
		return scheduleConfig{}, errWorkers
	}
	out.Workers = workers

	actions := normalizeAutoApplyActions(in.AutoApplyActions)
	if len(actions) == 0 {
		actions = []string{"disable", "enable"}
	}
	out.AutoApplyActions = actions

	// Preserve runtime fields from current if caller omitted them (POST body).
	out.NextRunAt = strings.TrimSpace(in.NextRunAt)
	out.LastRunAt = strings.TrimSpace(in.LastRunAt)
	out.LastFinishedAt = strings.TrimSpace(in.LastFinishedAt)
	out.LastError = strings.TrimSpace(in.LastError)
	out.LastApplyDone = in.LastApplyDone
	out.LastApplyTotal = in.LastApplyTotal
	out.LastApplyOK = in.LastApplyOK
	out.Running = in.Running
	// Preserve management password if provided (auto-captured from request headers).
	out.ManagementPassword = in.ManagementPassword
	return out, nil
}

func normalizeAutoApplyActions(actions []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	for _, a := range actions {
		a = strings.ToLower(strings.TrimSpace(a))
		switch a {
		case "disable", "enable", "delete":
			if _, ok := seen[a]; ok {
				continue
			}
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	return out
}

func loadScheduleFromDisk() (scheduleConfig, error) {
	raw, err := os.ReadFile(scheduleFilePath())
	if err != nil {
		return scheduleConfig{}, err
	}
	var snap persistedSchedule
	if err := json.Unmarshal(raw, &snap); err != nil {
		return scheduleConfig{}, err
	}
	cfg, errNorm := normalizeScheduleConfig(snap.Config)
	if errNorm != nil {
		return scheduleConfig{}, errNorm
	}
	return cfg, nil
}

func saveScheduleToDisk(cfg scheduleConfig) error {
	path := scheduleFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Do not persist ephemeral running flag.
	toSave := cfg
	toSave.Running = false
	payload := persistedSchedule{
		Version: scheduleStoreVersion,
		Config:  toSave,
		SavedAt: time.Now().Format(time.RFC3339),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *scheduleManager) init() {
	s.once.Do(func() {
		if cfg, err := loadScheduleFromDisk(); err == nil {
			s.mu.Lock()
			s.cfg = cfg
			if s.cfg.Enabled && s.cfg.NextRunAt == "" {
				s.cfg.NextRunAt = time.Now().Add(time.Duration(s.cfg.IntervalMinutes) * time.Minute).Format(time.RFC3339)
			}
			s.mu.Unlock()
		}
		go s.loop()
	})
}

func (s *scheduleManager) snapshot() scheduleConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := cloneScheduleConfig(s.cfg)
	out.ManagementPassword = "" // never leak management password in API responses
	return out
}

func cloneScheduleConfig(cfg scheduleConfig) scheduleConfig {
	out := cfg
	out.AutoApplyActions = append([]string(nil), cfg.AutoApplyActions...)
	return out
}

func (s *scheduleManager) setConfig(in scheduleConfig) (scheduleConfig, error) {
	cfg, err := normalizeScheduleConfig(in)
	if err != nil {
		return scheduleConfig{}, err
	}
	s.mu.Lock()
	// Keep last run stats when updating settings unless explicitly cleared.
	if cfg.LastRunAt == "" {
		cfg.LastRunAt = s.cfg.LastRunAt
		cfg.LastFinishedAt = s.cfg.LastFinishedAt
		cfg.LastError = s.cfg.LastError
		cfg.LastApplyDone = s.cfg.LastApplyDone
		cfg.LastApplyTotal = s.cfg.LastApplyTotal
		cfg.LastApplyOK = s.cfg.LastApplyOK
	}
	if cfg.Enabled {
		// Reschedule from now when enabling or changing interval.
		cfg.NextRunAt = time.Now().Add(time.Duration(cfg.IntervalMinutes) * time.Minute).Format(time.RFC3339)
	} else {
		cfg.NextRunAt = ""
		cfg.Running = false
	}
	cfg.Running = s.cfg.Running
	s.cfg = cfg
	// Save full config (including management password) to disk, but return
	// a copy with the password stripped for the API response.
	toDisk := cloneScheduleConfig(s.cfg)
	s.mu.Unlock()
	_ = saveScheduleToDisk(toDisk)
	snap := cloneScheduleConfig(s.cfg)
	snap.ManagementPassword = ""
	s.wake()
	return snap, nil
}

func (s *scheduleManager) wake() {
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}

func (s *scheduleManager) shutdown() {
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
}

func (s *scheduleManager) loop() {
	for {
		wait := s.nextWait()
		timer := time.NewTimer(wait)
		select {
		case <-s.stopCh:
			timer.Stop()
			return
		case <-s.wakeCh:
			timer.Stop()
			continue
		case <-timer.C:
			s.runOnce(false)
		}
	}
}

// runNow starts one scheduled inspection immediately (uses current schedule
// params + optional auto-apply). Resets next_run_at right away so UI countdown
// restarts; does not require schedule.enabled.
// password is auto-captured from request headers; stored so headless ticks can use it.
func (s *scheduleManager) runNow(password string) (scheduleConfig, error) {
	s.init()
	snap := engine.snapshot(false)
	if snap.Running || snap.Applying || snap.ActionInFlight > 0 {
		return scheduleConfig{}, fmt.Errorf("inspection is busy")
	}
	s.mu.Lock()
	if s.cfg.Running {
		s.mu.Unlock()
		return scheduleConfig{}, fmt.Errorf("scheduled inspection already running")
	}
	// Store management password if provided (auto-captured from request headers).
	if password != "" {
		s.cfg.ManagementPassword = password
	}
	// Restart countdown immediately on trigger (also rewritten when run finishes).
	if s.cfg.Enabled {
		s.cfg.NextRunAt = time.Now().Add(time.Duration(s.cfg.IntervalMinutes) * time.Minute).Format(time.RFC3339)
	}
	out := cloneScheduleConfig(s.cfg)
	out.ManagementPassword = "" // strip from API response
	s.mu.Unlock()
	_ = saveScheduleToDisk(cloneScheduleConfig(s.cfg))
	s.wake()
	go s.runOnce(true)
	return out, nil
}

func (s *scheduleManager) nextWait() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.cfg.Enabled {
		return time.Hour
	}
	if s.cfg.NextRunAt == "" {
		return time.Duration(s.cfg.IntervalMinutes) * time.Minute
	}
	t, err := time.Parse(time.RFC3339, s.cfg.NextRunAt)
	if err != nil {
		return time.Duration(s.cfg.IntervalMinutes) * time.Minute
	}
	d := time.Until(t)
	if d < time.Second {
		return time.Second
	}
	return d
}

// runOnce executes one scheduled tick. force=true skips the enabled check
// (used by the UI "run now" button).
func (s *scheduleManager) runOnce(force bool) {
	s.mu.Lock()
	if !force && !s.cfg.Enabled {
		s.mu.Unlock()
		return
	}
	if s.cfg.Running {
		s.mu.Unlock()
		return
	}
	cfg := cloneScheduleConfig(s.cfg)
	s.cfg.Running = true
	s.cfg.LastRunAt = time.Now().Format(time.RFC3339)
	s.cfg.LastError = ""
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.cfg.Running = false
		s.cfg.LastFinishedAt = time.Now().Format(time.RFC3339)
		if s.cfg.Enabled {
			s.cfg.NextRunAt = time.Now().Add(time.Duration(s.cfg.IntervalMinutes) * time.Minute).Format(time.RFC3339)
		} else {
			s.cfg.NextRunAt = ""
		}
		snap := cloneScheduleConfig(s.cfg)
		s.mu.Unlock()
		_ = saveScheduleToDisk(snap)
		s.wake()
	}()

	req := startRequest{
		Workers:         cfg.Workers,
		IncludeDisabled: cfg.IncludeDisabled,
		OnlyDisabled:    cfg.OnlyDisabled,
		Incremental:     cfg.Incremental,
	}
	if err := engine.start(req); err != nil {
		// Busy or invalid: skip this tick, try next interval.
		s.setLastError(err.Error())
		return
	}
	if !waitEngineIdle(scheduleRunTimeout) {
		s.setLastError("timed out waiting for inspection to finish")
		return
	}

	if !cfg.AutoApply {
		return
	}

	// Headless apply uses stored management password (auto-captured from
	// the page Authorization header when schedule was last saved via UI),
	// with env fallback (MANAGEMENT_PASSWORD / CPA_MANAGEMENT_KEY) for
	// container-only setups that never use the management panel.
	// Recommended actions only; force_action is never used by schedule.
	// No per-run action cap — apply all matching recommendations in the whitelist.
	actions := append([]string(nil), cfg.AutoApplyActions...)
	indexes, errPick := engine.pickRecommendedIndexes(actions)
	if errPick != nil {
		s.setLastError(errPick.Error())
		return
	}
	if len(indexes) == 0 {
		s.mu.Lock()
		s.cfg.LastApplyDone = 0
		s.cfg.LastApplyTotal = 0
		s.cfg.LastApplyOK = true
		s.mu.Unlock()
		return
	}
	if err := engine.startApply(applyRequest{
		AuthIndexes: indexes,
		Actions:     actions,
	}, cfg.ManagementPassword, nil); err != nil {
		// "no recommended actions" is not an error for schedule.
		if strings.Contains(err.Error(), "no recommended") {
			s.mu.Lock()
			s.cfg.LastApplyDone = 0
			s.cfg.LastApplyTotal = 0
			s.cfg.LastApplyOK = true
			s.mu.Unlock()
			return
		}
		s.setLastError("auto apply: " + err.Error())
		return
	}
	if !waitEngineIdle(scheduleRunTimeout) {
		s.setLastError("timed out waiting for apply to finish")
		return
	}
	snap := engine.snapshot(false)
	s.mu.Lock()
	s.cfg.LastApplyDone = snap.ApplyDone
	s.cfg.LastApplyTotal = snap.ApplyTotal
	s.cfg.LastApplyOK = len(snap.ApplyFailures) == 0
	if len(snap.ApplyFailures) > 0 {
		s.cfg.LastError = "apply failures: " + strings.Join(snap.ApplyFailures, "; ")
		if len(s.cfg.LastError) > 500 {
			s.cfg.LastError = s.cfg.LastError[:500] + "..."
		}
	}
	s.mu.Unlock()
}

func (s *scheduleManager) setLastError(msg string) {
	s.mu.Lock()
	s.cfg.LastError = strings.TrimSpace(msg)
	s.mu.Unlock()
}

// waitEngineIdle polls until inspection/apply/row-actions are idle.
func waitEngineIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	// Brief grace so start() goroutine can flip running=true.
	time.Sleep(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		snap := engine.snapshot(false)
		if !snap.Running && !snap.Applying && snap.ActionInFlight == 0 {
			return true
		}
		select {
		case <-scheduler.stopCh:
			return false
		case <-time.After(scheduleIdlePoll):
		}
	}
	return false
}

// pickRecommendedIndexes returns all auth indexes with matching recommended actions.
func (e *inspectionEngine) pickRecommendedIndexes(actions []string) ([]string, error) {
	actionSet := stringSet(actions)
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0)
	for _, item := range e.results {
		if item.Action != "disable" && item.Action != "enable" && item.Action != "delete" {
			continue
		}
		if len(actionSet) > 0 {
			if _, ok := actionSet[item.Action]; !ok {
				continue
			}
		}
		key := firstNonEmpty(item.AuthIndex, item.FileName, item.Name, item.Email)
		if key == "" {
			continue
		}
		out = append(out, key)
	}
	return out, nil
}
