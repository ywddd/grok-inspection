package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// inspectionScheduler owns the process-local cron for timed full inspections.
type inspectionScheduler struct {
	mu      sync.Mutex
	cron    *cron.Cron
	entryID cron.EntryID
	cfg     scheduleConfig
	runtime scheduleRuntime
}

var scheduler = &inspectionScheduler{
	cfg: defaultScheduleConfig(),
}

func init() {
	cfg, err := loadScheduleConfig()
	if err != nil {
		cfg = defaultScheduleConfig()
	}
	scheduler.applyConfig(cfg, false)
}

func (s *inspectionScheduler) applyConfig(cfg scheduleConfig, persist bool) error {
	if err := validateScheduleConfig(&cfg); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	if persist {
		if err := saveScheduleConfig(cfg); err != nil {
			return err
		}
	}
	s.rebuildLocked()
	return nil
}

func (s *inspectionScheduler) rebuildLocked() {
	if s.cron != nil {
		s.cron.Stop()
		s.cron = nil
		s.entryID = 0
	}
	s.runtime.NextRunAt = ""
	if !s.cfg.Enabled {
		return
	}
	c := cron.New(cron.WithParser(scheduleCronParser), cron.WithLocation(time.Local))
	id, err := c.AddFunc(s.cfg.Cron, s.onTick)
	if err != nil {
		s.runtime.LastRunStatus = "start_failed"
		s.runtime.LastSkipReason = "invalid cron: " + err.Error()
		return
	}
	s.cron = c
	s.entryID = id
	c.Start()
	s.refreshNextLocked()
}

func (s *inspectionScheduler) refreshNextLocked() {
	if s.cron == nil || s.entryID == 0 {
		s.runtime.NextRunAt = ""
		return
	}
	entry := s.cron.Entry(s.entryID)
	if entry.ID == 0 || entry.Next.IsZero() {
		s.runtime.NextRunAt = ""
		return
	}
	s.runtime.NextRunAt = formatTimeRFC3339(entry.Next)
}

func (s *inspectionScheduler) onTick() {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	if !cfg.Enabled {
		return
	}

	if engine.isBusy() {
		s.recordSkip("skipped_busy", "inspection or apply in progress")
		return
	}

	err := engine.startScheduled(cfg)
	if err != nil {
		if err.Error() == "inspection already running" || err.Error() == "busy: row action in progress" || err.Error() == "busy" {
			s.recordSkip("skipped_busy", err.Error())
			return
		}
		s.recordSkip("start_failed", err.Error())
		return
	}
	s.mu.Lock()
	s.runtime.LastRunAt = formatTimeRFC3339(time.Now())
	s.runtime.LastRunStatus = "running"
	s.runtime.LastSkipReason = ""
	s.runtime.LastAutoFailures = nil
	s.runtime.LastAutoDeleted = 0
	s.runtime.LastAutoDisabled = 0
	s.runtime.LastAutoEnabled = 0
	s.refreshNextLocked()
	s.mu.Unlock()
}

func (s *inspectionScheduler) recordSkip(status, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtime.LastRunAt = formatTimeRFC3339(time.Now())
	s.runtime.LastRunStatus = status
	s.runtime.LastSkipReason = reason
	s.runtime.LastAutoDeleted = 0
	s.runtime.LastAutoDisabled = 0
	s.runtime.LastAutoEnabled = 0
	s.runtime.LastAutoFailures = nil
	s.refreshNextLocked()
}

func (s *inspectionScheduler) recordFinished(status string, failures []string, counts autoActionCounts) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtime.LastRunStatus = status
	if status != "skipped_busy" && status != "start_failed" {
		s.runtime.LastSkipReason = ""
	}
	if len(failures) > maxAutoFailKeep {
		failures = append([]string(nil), failures[len(failures)-maxAutoFailKeep:]...)
	} else if failures != nil {
		failures = append([]string(nil), failures...)
	}
	s.runtime.LastAutoFailures = failures
	s.runtime.LastAutoDeleted = counts.Deleted
	s.runtime.LastAutoDisabled = counts.Disabled
	s.runtime.LastAutoEnabled = counts.Enabled
	s.refreshNextLocked()
}

func (s *inspectionScheduler) view() scheduleView {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshNextLocked()
	return scheduleView{
		scheduleConfig:  s.cfg,
		scheduleRuntime: cloneRuntime(s.runtime),
	}
}

func (s *inspectionScheduler) configSnapshot() scheduleConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *inspectionScheduler) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
		s.cron = nil
		s.entryID = 0
	}
	s.runtime.NextRunAt = ""
}

func cloneRuntime(r scheduleRuntime) scheduleRuntime {
	out := r
	if r.LastAutoFailures != nil {
		out.LastAutoFailures = append([]string(nil), r.LastAutoFailures...)
	}
	return out
}

func (e *inspectionEngine) isBusy() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running || e.applying || e.actionInFlight > 0
}

func (e *inspectionEngine) startScheduled(cfg scheduleConfig) error {
	workers, errWorkers := normalizeWorkers(cfg.Workers)
	if errWorkers != nil {
		return errWorkers
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
	e.running = true
	e.stopped = false
	e.applying = false
	e.scheduledRun = true
	e.runTargets = nil
	e.runModel = ""
	e.runClassifyScoped = false
	e.incremental = false
	e.classifications = nil
	e.workers = workers
	e.includeDisabled = true
	e.onlyDisabled = false
	e.results = nil
	e.bumpResultsLocked()
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
	e.persistLocked()
	e.mu.Unlock()

	e.runWG.Add(1)
	go func() {
		defer e.runWG.Done()
		e.run(runID, workers, true, false, false, nil)
	}()
	return nil
}
