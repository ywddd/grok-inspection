package main

import (
	"strings"
	"time"
)

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
		Sample:           e.sampleMode,
		SampleCount:      e.sampleCount,
		SamplePercent:    e.samplePercent,
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
