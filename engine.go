package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func (e *inspectionEngine) shutdown() {
	// Permanent gate first under engine.mu so start/startApply/startAction/claimUnbanSlot
	// cannot Add to WaitGroups after Wait begins (and cannot clear stopped).
	// Normal stop() only sets stopped; it must not set shuttingDown.
	e.mu.Lock()
	e.shuttingDown = true
	e.stopped = true
	e.runID++
	e.applyRunID++
	e.applying = false
	e.applyDraining = true
	e.mu.Unlock()

	// Independent workers: no runWG/unbanJob.wg Add after gate above.
	stopBanDisposeWorkers()
	stopUnbanJob()

	// Wait only after gate; entrypoints that Add hold engine.mu and check shuttingDown first.
	e.runWG.Wait()
	unbanJob.wg.Wait()
	// Async results.json writers must finish before TempDir/teardown.
	e.waitAsyncPersist()
	// Final ban-state flush only after all ban-store producers have exited so a
	// late Set cannot land after persist worker stop with no further flush.
	stopBanPersistWorker()
	// Soft-timeout probes may still hold abandoned host.http.do callbacks.
	// Wait until they finish before clearing host/dlclose (unbounded; host ABI).
	waitHostCallsForShutdown(5 * time.Second)
	e.mu.Lock()
	// Keep shuttingDown permanent; only clear applyDraining so status is idle-looking.
	e.applyDraining = false
	e.mu.Unlock()
}

func (e *inspectionEngine) run(runID uint64, workers int, includeDisabled, onlyDisabled, incremental bool, classifications []string) {
	defer e.finish(runID)

	list, errList := callHostAuthListFn()
	if errList != nil {
		// Keep previous results on list failure (full inspect no longer wipes early).
		// Use a stable auth_index so repeated failures do not accumulate fake rows.
		// Do NOT mark list OK: scheduled auto-actions must not act on stale results.
		write := e.upsertResult
		listReason := T(e.lang, "list_accounts_failed", errList.Error())
		if errors.Is(errList, errListAccountsTimeout) {
			listReason = T(e.lang, "list_accounts_timeout")
		}
		e.noteRunListOutcome(runID, false, listReason)
		write(runID, accountResult{
			AuthIndex:      "__system_list_error__",
			Name:           "system",
			Classification: "probe_error",
			Action:         "keep",
			Reason:         listReason,
		})
		return
	}
	// Account list obtained for this runID; individual probe_error later is not a list failure.
	e.noteRunListOutcome(runID, true, "")

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
		e.mu.Lock()
		sampleModeNow := e.sampleMode
		e.mu.Unlock()
		filtered := make([]pluginapi.HostAuthFileEntry, 0, len(targets))
		for _, file := range targets {
			if !isXAIEntry(file.Provider, file.Name, file.Type) {
				continue
			}
			// Sample+category follows the page disabled filters; plain category
			// re-inspect keeps matching every account currently on that card.
			if sampleModeNow && !shouldInspectEntry(file.Provider, file.Name, file.Type, file.Disabled, file.Status, includeDisabled, onlyDisabled) {
				continue
			}
			filtered = append(filtered, file)
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

	// Sample mode: keep history, only update probed rows (same merge path as category).
	e.mu.Lock()
	sampleMode := e.sampleMode
	sampleCount := e.sampleCount
	samplePercent := e.samplePercent
	e.mu.Unlock()
	mergeResults := classifyScoped || sampleMode
	if sampleMode {
		size, errSample := resolveSampleSize(len(targets), sampleCount, samplePercent)
		if errSample != nil {
			write := e.upsertResult
			write(runID, accountResult{
				AuthIndex:      "__system_sample_error__",
				Name:           "system",
				Classification: "probe_error",
				Action:         "keep",
				Reason:         errSample.Error(),
			})
			return
		}
		targets = sampleAuthEntries(targets, size, nil)
		// Sample only updates chosen live targets; do not invent missing-row updates.
		missing = nil
	}

	writeResult := e.appendResult
	if mergeResults {
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
	cont, cleared := e.commitRunPlanLocked(runID, targets, missing, model, mergeResults)
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
