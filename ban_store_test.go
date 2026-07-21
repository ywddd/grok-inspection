package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testEntry(authID string, resetAt time.Time) banEntry {
	return banEntry{
		AuthID:      authID,
		Provider:    "xai",
		ErrorCode:   exhaustedErrorCode,
		BannedAt:    resetAt.Add(-24 * time.Hour),
		ResetAt:     resetAt,
		ResetSource: "test",
		CpaSynced:   true,
	}
}

func TestBanStoreKeepsLaterReset(t *testing.T) {
	store := newBanStore()
	first := testEntry("auth-1", time.Unix(100, 0))
	later := testEntry("auth-1", time.Unix(200, 0))
	earlier := testEntry("auth-1", time.Unix(50, 0))
	store.Set(first)
	store.Set(later)
	store.Set(earlier)

	got, ok := store.Get("auth-1")
	if !ok || !got.ResetAt.Equal(later.ResetAt) {
		t.Fatalf("entry = %#v, ok=%v", got, ok)
	}
}

func TestBanStoreClearsExpiredAndCopiesList(t *testing.T) {
	store := newBanStore()
	store.Set(testEntry("expired", time.Unix(10, 0)))
	store.Set(testEntry("active", time.Unix(200, 0)))
	expired := store.ClearExpired(time.Unix(100, 0))
	if len(expired) != 1 || expired[0] != "expired" {
		t.Fatalf("expired = %#v", expired)
	}

	if _, ok := store.Get("expired"); ok {
		t.Fatal("expired entry remains")
	}
	items := store.List(time.Unix(100, 0))
	if len(items) != 1 || items[0].AuthID != "active" {
		t.Fatalf("items = %#v", items)
	}
	items[0].AuthID = "mutated"
	got, _ := store.Get("active")
	if got.AuthID != "active" {
		t.Fatal("List returned mutable internal state")
	}
}

func TestBanStorePersistenceRoundTripAndCorruptRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bans.json")
	store := newBanStore()
	store.Set(testEntry("auth-1", time.Unix(200, 0)))
	if err := store.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	store.Set(testEntry("auth-2", time.Unix(300, 0)))
	if err := store.Save(path); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	reloaded := newBanStore()
	if err := reloaded.Load(path, time.Unix(100, 0)); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if _, ok := reloaded.Get("auth-1"); !ok {
		t.Fatal("round-tripped entry missing")
	}
	if _, ok := reloaded.Get("auth-2"); !ok {
		t.Fatal("updated entry missing after second save")
	}

	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	// Live store must keep previous bans when load fails.
	live := newBanStore()
	live.Set(testEntry("keep-me", time.Unix(400, 0)))
	if err := live.Load(path, time.Now()); err == nil {
		t.Fatal("corrupt JSON returned nil error")
	}
	if _, ok := live.Get("keep-me"); !ok {
		t.Fatal("corrupt load cleared live memory")
	}
	recovered := newBanStore()
	if err := recovered.Load(path, time.Now()); err == nil {
		t.Fatal("corrupt JSON returned nil error")
	}
	if len(recovered.List(time.Now())) != 0 {
		t.Fatal("corrupt load populated empty store")
	}
}

func TestBanStoreDeleteAndClear(t *testing.T) {
	store := newBanStore()
	store.Set(testEntry("auth-1", time.Unix(200, 0)))
	store.Set(testEntry("auth-2", time.Unix(200, 0)))
	if !store.Delete("auth-1") {
		t.Fatal("Delete() = false")
	}
	if store.Delete("missing") {
		t.Fatal("Delete(missing) = true")
	}
	store.Clear()
	if len(store.List(time.Now())) != 0 {
		t.Fatal("Clear() did not empty store")
	}
}
