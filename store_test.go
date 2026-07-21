package main

import (
	"os"
	"sync"
	"path/filepath"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestNormalizeWorkers(t *testing.T) {
	got, err := normalizeWorkers(0)
	if err != nil || got != 6 {
		t.Fatalf("default workers = %d, %v", got, err)
	}
	got, err = normalizeWorkers(8)
	if err != nil || got != 8 {
		t.Fatalf("workers 8 = %d, %v", got, err)
	}
	if _, err := normalizeWorkers(17); err == nil {
		t.Fatal("expected error for workers=17")
	}
	if _, err := normalizeWorkers(-1); err == nil {
		t.Fatal("expected error for workers=-1")
	}
}

func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	setStoreFilePathForTest(path)
	t.Cleanup(func() { setStoreFilePathForTest("") })

	snap := persistedSnapshot{
		Workers: 4,
		Results: []accountResult{
			{Name: "a@x.com", Classification: "reauth", Action: "delete"},
			{Name: "b@x.com", Classification: "healthy", Action: "keep"},
		},
	}
	if err := savePersistedSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Workers != 4 || len(loaded.Results) != 2 {
		t.Fatalf("loaded = %+v", loaded)
	}
	if loaded.Results[0].Classification != "reauth" {
		t.Fatalf("classification = %s", loaded.Results[0].Classification)
	}
}

func TestCollectCandidatesFilters(t *testing.T) {
	e := &inspectionEngine{
		results: []accountResult{
			{Name: "a", AuthIndex: "1", Classification: "reauth", Action: "delete"},
			{Name: "b", AuthIndex: "2", Classification: "permission_denied", Action: "disable"},
			{Name: "c", AuthIndex: "3", Classification: "healthy", Action: "enable"},
			{Name: "d", AuthIndex: "4", Classification: "healthy", Action: "keep"},
		},
	}
	got, err := e.collectCandidates(applyRequest{
		Actions:         []string{"delete"},
		Classifications: []string{"reauth"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("got = %+v", got)
	}
	got, err = e.collectCandidates(applyRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("all recommended = %d", len(got))
	}
}

func TestFilterNewAuthEntriesIncremental(t *testing.T) {
	known := knownResultKeys([]accountResult{
		{AuthIndex: "old-1", FileName: "old-a.json", Name: "a@x.com"},
		// No auth_index: skip only by file fingerprint (name+size+mtime), never email alone.
		{FileName: "old-b.json", FileSize: 10, FileModUnix: 100},
	})
	files := []pluginapi.HostAuthFileEntry{
		{Provider: "xai", AuthIndex: "old-1", Name: "old-a.json", Email: "a@x.com"}, // known by auth_index
		{Provider: "xai", AuthIndex: "new-2", Name: "new-c.json", Email: "c@x.com"}, // new file
		// Same file name + fingerprint as known, but NEW auth_index → re-inspect (re-import).
		{Provider: "xai", AuthIndex: "new-3", Name: "old-b.json", Email: "b@x.com", Size: 10, ModTime: time.Unix(100, 0)},
		// No auth_index, same fingerprint → still known / skip
		{Provider: "xai", Name: "old-b.json", Size: 10, ModTime: time.Unix(100, 0)},
		{Provider: "openai", AuthIndex: "other", Name: "skip.json"}, // non-xai skipped
	}
	got := filterNewAuthEntries(files, known, false, false)
	if len(got) != 2 {
		t.Fatalf("incremental targets len=%d got=%+v", len(got), got)
	}
	gotIdx := map[string]bool{}
	for _, f := range got {
		gotIdx[f.AuthIndex] = true
	}
	if !gotIdx["new-2"] || !gotIdx["new-3"] {
		t.Fatalf("want new-2 and new-3, got %+v", got)
	}
}

func TestCollectCandidatesForceActionByIndexes(t *testing.T) {
	e := &inspectionEngine{
		results: []accountResult{
			{Name: "a", AuthIndex: "1", FileName: "a.json", Classification: "healthy", Action: "keep"},
			{Name: "b", AuthIndex: "2", FileName: "b.json", Classification: "permission_denied", Action: "disable"},
			{Name: "c", AuthIndex: "3", FileName: "c.json", Classification: "reauth", Action: "delete"},
		},
	}
	got, err := e.collectCandidates(applyRequest{
		ForceAction: "delete",
		AuthIndexes: []string{"1", "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	for _, item := range got {
		if item.Action != "delete" {
			t.Fatalf("force action not applied: %+v", item)
		}
	}
	// force without selection is rejected
	if _, err := e.collectCandidates(applyRequest{ForceAction: "disable"}); err == nil {
		t.Fatal("expected error when force_action has no selection")
	}
}

func TestSavePersistedSnapshotCoalescesConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	setStoreFilePathForTest(path)
	t.Cleanup(func() { setStoreFilePathForTest("") })

	const writers = 12
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap := persistedSnapshot{
				Workers: i + 1,
				Results: []accountResult{{
					Name:           "acc-" + string(rune('a'+i%26)),
					AuthIndex:      "idx-" + string(rune('a'+i%26)),
					Classification: "healthy",
					Action:         "keep",
				}},
			}
			// Use distinct result counts so the last completed generation is easy to spot.
			snap.Results = make([]accountResult, i+1)
			for j := 0; j <= i; j++ {
				snap.Results[j] = accountResult{
					Name:           "n" + string(rune('a'+j%26)),
					AuthIndex:      "i" + string(rune('a'+j%26)),
					Classification: "healthy",
					Action:         "keep",
				}
			}
			if err := savePersistedSnapshot(snap); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Results) == 0 {
		t.Fatal("expected persisted results")
	}
	// Concurrent saves may coalesce; final file must be a complete valid snapshot
	// from one of the writers (result count between 1 and writers).
	if n := len(loaded.Results); n < 1 || n > writers {
		t.Fatalf("unexpected result count %d", n)
	}
	if loaded.Workers < 1 || loaded.Workers > writers {
		t.Fatalf("unexpected workers %d", loaded.Workers)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should not remain: %v", err)
	}
}

func TestSavePersistedSnapshotSerializesWithLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	setStoreFilePathForTest(path)
	t.Cleanup(func() { setStoreFilePathForTest("") })

	if err := savePersistedSnapshot(persistedSnapshot{
		Workers: 3,
		Results: []accountResult{{Name: "seed", AuthIndex: "s1", Classification: "healthy", Action: "keep"}},
	}); err != nil {
		t.Fatal(err)
	}

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n*2)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			snap := persistedSnapshot{
				Workers: (i % 8) + 1,
				Results: make([]accountResult, (i%5)+1),
			}
			for j := range snap.Results {
				snap.Results[j] = accountResult{Name: "x", AuthIndex: "a", Classification: "healthy", Action: "keep"}
			}
			if err := savePersistedSnapshot(snap); err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := loadPersistedSnapshot(); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent save/load failed: %v", err)
	}
	if _, err := loadPersistedSnapshot(); err != nil {
		t.Fatal(err)
	}
}

func TestSavePersistedSnapshotRejectsStaleSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.json")
	setStoreFilePathForTest(path)
	resetStoreIOForTest()
	t.Cleanup(func() {
		setStoreFilePathForTest("")
		resetStoreIOForTest()
	})

	// Simulate finish() writing a newer snapshot, then a delayed persistLocked with older seq.
	newer := persistedSnapshot{
		Workers: 4,
		Results: []accountResult{{Name: "final", AuthIndex: "f1", Classification: "healthy", Action: "keep"}},
		seq:     20,
	}
	older := persistedSnapshot{
		Workers: 2,
		Results: []accountResult{{Name: "partial", AuthIndex: "p1", Classification: "healthy", Action: "keep"}},
		seq:     10,
	}
	if err := savePersistedSnapshot(newer); err != nil {
		t.Fatal(err)
	}
	if err := savePersistedSnapshot(older); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadPersistedSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Results) != 1 || loaded.Results[0].Name != "final" {
		t.Fatalf("stale snapshot overwrote final: %+v", loaded.Results)
	}
	if loaded.Workers != 4 {
		t.Fatalf("workers = %d", loaded.Workers)
	}
}
