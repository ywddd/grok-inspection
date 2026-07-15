package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	scheduleVersion     = 1
	defaultScheduleCron = "0 3 * * *"
	maxAutoFailKeep     = 20
)

// scheduleConfig is the persisted auto-inspection configuration.
type scheduleConfig struct {
	Version                      int    `json:"version"`
	Enabled                      bool   `json:"enabled"`
	Cron                         string `json:"cron"`
	Workers                      int    `json:"workers"`
	AutoDeletePermissionDenied   bool   `json:"auto_delete_permission_denied"`
	AutoDisableQuotaExhausted    bool   `json:"auto_disable_quota_exhausted"`
	AutoEnableHealthyDisabled    bool   `json:"auto_enable_healthy_disabled"`
}

// scheduleRuntime is in-memory observability for the last / next timed run.
type scheduleRuntime struct {
	NextRunAt        string   `json:"next_run_at,omitempty"`
	LastRunAt        string   `json:"last_run_at,omitempty"`
	LastRunStatus    string   `json:"last_run_status,omitempty"`
	LastSkipReason   string   `json:"last_skip_reason,omitempty"`
	LastAutoFailures []string `json:"last_auto_failures,omitempty"`
	// Successful auto-action counts from the last timed run (not attempted).
	LastAutoDeleted  int `json:"last_auto_deleted"`
	LastAutoDisabled int `json:"last_auto_disabled"`
	LastAutoEnabled  int `json:"last_auto_enabled"`
}

// autoActionCounts is the success tally for one timed auto-dispose pass.
type autoActionCounts struct {
	Deleted  int
	Disabled int
	Enabled  int
}

// scheduleView is returned by GET /schedule.
type scheduleView struct {
	scheduleConfig
	scheduleRuntime
}

var (
	scheduleMu           sync.Mutex
	schedulePathOverride string
	scheduleCronParser   = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
)

func defaultScheduleConfig() scheduleConfig {
	return scheduleConfig{
		Version: scheduleVersion,
		Enabled: false,
		Cron:    defaultScheduleCron,
		Workers: defaultWorkers,
	}
}

func scheduleFilePath() string {
	scheduleMu.Lock()
	defer scheduleMu.Unlock()
	if schedulePathOverride != "" {
		return schedulePathOverride
	}
	if dir := firstNonEmpty(os.Getenv("GROK_INSPECTION_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "schedule.json")
	}
	return filepath.Join("data", "grok-inspection", "schedule.json")
}

func setScheduleFilePathForTest(path string) {
	scheduleMu.Lock()
	defer scheduleMu.Unlock()
	schedulePathOverride = path
}

func validateScheduleConfig(cfg *scheduleConfig) error {
	if cfg == nil {
		return fmt.Errorf("schedule config is required")
	}
	cfg.Cron = strings.TrimSpace(cfg.Cron)
	if cfg.Cron == "" {
		return fmt.Errorf("cron is required")
	}
	if _, err := scheduleCronParser.Parse(cfg.Cron); err != nil {
		return fmt.Errorf("invalid cron: %w", err)
	}
	workers, errWorkers := normalizeWorkers(cfg.Workers)
	if errWorkers != nil {
		return errWorkers
	}
	cfg.Workers = workers
	cfg.Version = scheduleVersion
	return nil
}

func loadScheduleConfig() (scheduleConfig, error) {
	path := scheduleFilePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultScheduleConfig(), nil
		}
		return scheduleConfig{}, err
	}
	var cfg scheduleConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return scheduleConfig{}, err
	}
	if strings.TrimSpace(cfg.Cron) == "" {
		cfg.Cron = defaultScheduleCron
	}
	if cfg.Workers == 0 {
		cfg.Workers = defaultWorkers
	}
	if err := validateScheduleConfig(&cfg); err != nil {
		// Corrupt/invalid on disk: fall back to safe defaults rather than crash.
		return defaultScheduleConfig(), nil
	}
	return cfg, nil
}

func saveScheduleConfig(cfg scheduleConfig) error {
	if err := validateScheduleConfig(&cfg); err != nil {
		return err
	}
	path := scheduleFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cfg.Version = scheduleVersion
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func nextCronTime(expr string, from time.Time) (time.Time, error) {
	sched, err := scheduleCronParser.Parse(strings.TrimSpace(expr))
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from.In(time.Local)), nil
}

func formatTimeRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format(time.RFC3339)
}
