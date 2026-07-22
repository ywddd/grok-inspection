package main

import (
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestSyncManualInspectionBanCreatesManualEntry(t *testing.T) {
	store := newBanStore()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	target := &pluginapi.HostAuthFileEntry{
		AuthIndex: "auth-index",
		Name:      "account.json",
		Provider:  "xai",
	}

	if !syncManualInspectionBan(store, target, "alias", now) {
		t.Fatal("expected manual inspection ban to be added")
	}
	entry, ok := store.Get("account.json")
	if !ok {
		t.Fatal("manual inspection ban missing")
	}
	if entry.ErrorCode != manualInspectionBanErrorCode {
		t.Fatalf("error code=%q", entry.ErrorCode)
	}
	if entry.ResetSource != manualInspectionBanResetSource {
		t.Fatalf("reset source=%q", entry.ResetSource)
	}
	if !entry.ResetAt.After(now) {
		t.Fatalf("manual ban must remain active: reset_at=%s", entry.ResetAt)
	}
	if !entry.CpaSynced {
		t.Fatal("manual inspection ban should already be synced after CPA succeeds")
	}
}

func TestSyncManualInspectionBanUsesStableFallbackIdentity(t *testing.T) {
	store := newBanStore()
	now := time.Now()
	target := &pluginapi.HostAuthFileEntry{AuthIndex: "auth-index"}
	if !syncManualInspectionBan(store, target, "fallback.json", now) {
		t.Fatal("expected fallback identity to be used")
	}
	if _, ok := store.Get("fallback.json"); !ok {
		t.Fatal("fallback identity missing")
	}
}

func TestManualInspectionBanCategoryAndReason(t *testing.T) {
	if got := banCategoryOf(manualInspectionBanErrorCode); got != "manual" {
		t.Fatalf("category=%q", got)
	}
	store := newBanStore()
	now := time.Now()
	if !syncManualInspectionBan(store, nil, "manual.json", now) {
		t.Fatal("manual inspection ban should be recorded")
	}
	entry, ok := store.Get("manual.json")
	if !ok || entry.ErrorCode != manualInspectionBanErrorCode {
		t.Fatalf("manual inspection reason missing: %+v", entry)
	}
}
