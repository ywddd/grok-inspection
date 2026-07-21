package main

import "testing"

// isolateActiveStore clears the process-global ban store for a test and restores
// previous entries on cleanup. Never rebinds the activeStore pointer so the
// background restore loop cannot race on the pointer itself.
// Restored entries get new revisions via Set; that is fine for these tests.
func isolateActiveStore(t *testing.T) {
	t.Helper()
	snap := activeStore.All()
	activeStore.Clear()
	t.Cleanup(func() {
		activeStore.Clear()
		for _, entry := range snap {
			activeStore.Set(entry)
		}
	})
}
