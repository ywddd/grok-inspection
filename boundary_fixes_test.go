package main

import (
	"testing"

	"grok-inspection/cpasdk/pluginapi"
)

func TestCommitRunPlanLockedStaleRunIDDoesNotClear(t *testing.T) {
	e := &inspectionEngine{}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runID = 2 // stop already bumped past this run
	e.running = true
	e.fullClearPending = true
	e.results = []accountResult{{
		AuthIndex:      "old",
		Name:           "old.json",
		Classification: "healthy",
		Action:         "keep",
	}}
	targets := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "a", Name: "a.json", Provider: "xai"},
	}

	cont, cleared := e.commitRunPlanLocked(1, targets, nil, "grok-4.5", false)
	if cont {
		t.Fatal("stale runID must not continue")
	}
	if cleared {
		t.Fatal("stale runID must not clear prior results")
	}
	if len(e.results) != 1 || e.results[0].AuthIndex != "old" {
		t.Fatalf("results = %#v, want prior history preserved", e.results)
	}
	if e.fullClearPending != true {
		t.Fatal("fullClearPending should stay true when commit is rejected")
	}
	if e.total != 0 || len(e.runTargets) != 0 {
		t.Fatalf("stale commit must not register plan total=%d targets=%d", e.total, len(e.runTargets))
	}
}

func TestCommitRunPlanLockedStopAfterClearWritesCancelledRows(t *testing.T) {
	e := &inspectionEngine{}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runID = 5
	e.running = true
	e.stopped = true
	e.fullClearPending = true
	e.results = []accountResult{{
		AuthIndex:      "old",
		Name:           "old.json",
		Classification: "healthy",
		Action:         "keep",
	}}
	targets := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "a", Name: "a.json", Provider: "xai"},
		{AuthIndex: "b", Name: "b.json", Provider: "xai"},
	}

	cont, cleared := e.commitRunPlanLocked(5, targets, nil, "grok-4.5", false)
	if cont {
		t.Fatal("stopped run must not continue probing")
	}
	if !cleared {
		t.Fatal("full inspect must clear when targets are committed")
	}
	// Old healthy row is gone; cancelled rows for a/b must be present.
	if len(e.results) != 2 {
		t.Fatalf("results len=%d want 2 cancelled rows; %#v", len(e.results), e.results)
	}
	for _, item := range e.results {
		if item.Classification != "probe_error" {
			t.Fatalf("expected cancelled probe_error, got %#v", item)
		}
		if item.AuthIndex != "a" && item.AuthIndex != "b" {
			t.Fatalf("unexpected result %#v", item)
		}
	}
	if e.running {
		t.Fatal("abort should leave running=false")
	}
	if e.total != 2 || e.probeDone != 2 {
		t.Fatalf("total=%d probeDone=%d want 2/2", e.total, e.probeDone)
	}
	if e.fullClearPending {
		t.Fatal("fullClearPending should be cleared after commit")
	}
}

func TestCommitRunPlanLockedContinueRegistersTargetsWithoutPrematureClearFlag(t *testing.T) {
	e := &inspectionEngine{}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runID = 9
	e.running = true
	e.stopped = false
	e.fullClearPending = true
	e.results = []accountResult{{AuthIndex: "old", Name: "old.json", Classification: "healthy"}}
	targets := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "n1", Name: "n1.json", Provider: "xai"},
	}

	cont, cleared := e.commitRunPlanLocked(9, targets, nil, "grok-4.5", false)
	if !cont {
		t.Fatal("active run should continue")
	}
	if !cleared {
		t.Fatal("full inspect should clear")
	}
	if len(e.results) != 0 {
		t.Fatalf("results should be cleared before probes; got %#v", e.results)
	}
	if e.total != 1 || len(e.runTargets) != 1 || e.runTargets[0].AuthIndex != "n1" {
		t.Fatalf("plan not registered: total=%d targets=%#v", e.total, e.runTargets)
	}
	if e.fullClearPending {
		t.Fatal("fullClearPending should be false after commit")
	}
	// Immediate stop after commit can abort using registered targets.
	e.stopped = true
	e.abortRunLocked()
	if e.running {
		t.Fatal("expected stopped")
	}
	if len(e.results) != 1 || e.results[0].AuthIndex != "n1" {
		t.Fatalf("stop after commit should write cancelled for registered targets: %#v", e.results)
	}
}
