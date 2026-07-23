package main

import "testing"

// isolateActiveStore clears the process-global ban store for a test and restores
// previous entries on cleanup. Never rebinds the activeStore pointer so the
// background restore loop cannot race on the pointer itself.
// Restored entries get new revisions via Set; that is fine for these tests.
func isolateActiveStore(t *testing.T) {
	t.Helper()
	// Ensure package TestMain data isolation is active even if a prior test
	// unset GROK_INSPECTION_DATA_DIR.
	if packageTestDataDir != "" {
		restorePackageTestDataEnv()
	}
	snap := activeStore.All()
	activeStore.Clear()
	t.Cleanup(func() {
		activeStore.Clear()
		for _, entry := range snap {
			activeStore.Set(entry)
		}
		if packageTestDataDir != "" {
			restorePackageTestDataEnv()
		}
	})
}

// ---- package-level test hooks (production code must not call these) ----

func setStoreFilePathForTest(path string) {
	storeMu.Lock()
	defer storeMu.Unlock()
	if path == "" {
		storePathOverride = testStorePathDefault
		return
	}
	storePathOverride = path
}

// setTestStorePathDefaultForTest is used by TestMain only.
func setTestStorePathDefaultForTest(path string) {
	storeMu.Lock()
	defer storeMu.Unlock()
	testStorePathDefault = path
	if path != "" {
		storePathOverride = path
	}
}

func resetStoreIOForTest() {
	storeIOMu.Lock()
	defer storeIOMu.Unlock()
	storePending = nil
	storeSaveGen = 0
	storeWrittenGen = 0
	storeWriting = false
}

func clearManagementCredentialCacheForTest() {
	managementCredentialCache.mu.Lock()
	managementCredentialCache.key = ""
	managementCredentialCache.mu.Unlock()
}

func stopBanPersistWorkerForTest() {
	stopBanPersistWorker()
	// Allow restart in later tests of the same process.
	w := globalBanPersist
	w.mu.Lock()
	w.stop = false
	w.started = false
	w.dirty = false
	w.path = ""
	w.mu.Unlock()
}

func applyBanDisposeForTest(authID string, expectedRev uint64) error {
	return applyBanDisposeWithStore(activeStore, authID, expectedRev)
}

// rearmBanDisposeWorkersForTest resets the process-global dispose queue after a stop
// so later tests can start workers again.
func rearmBanDisposeWorkersForTest() {
	q := globalBanDispose
	q.mu.Lock()
	q.stopping = false
	q.started = false
	q.testHold = false
	q.testNoStart = false
	q.pending = make(map[string]uint64)
	q.order = q.order[:0]
	q.queued = 0
	q.inFlight = 0
	q.mu.Unlock()
}
