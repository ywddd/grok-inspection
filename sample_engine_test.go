package main

import (
	"math/rand"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"grok-inspection/cpasdk/pluginapi"
)

func TestSampleStartRejectsInvalidParams(t *testing.T) {
	e := &inspectionEngine{workers: defaultWorkers}
	err := e.start(startRequest{Workers: 2, Sample: true})
	if err == nil {
		t.Fatal("expected sample params required")
	}
	if statusFromError(err, 0) != http.StatusBadRequest {
		t.Fatalf("status=%d err=%v", statusFromError(err, 0), err)
	}
	err = e.start(startRequest{Workers: 2, Sample: true, SamplePercent: 200})
	if err == nil {
		t.Fatal("expected invalid percent")
	}
	err = e.start(startRequest{Workers: 2, Sample: true, SampleCount: 1, Incremental: true})
	if err == nil {
		t.Fatal("expected sample+incremental reject")
	}
}

func TestSampleInspectKeepsUnsampledHistory(t *testing.T) {
	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	oldRand := newSampleRand
	defer func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
		newSampleRand = oldRand
	}()

	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "a", Name: "a.json", Provider: "xai"},
		{AuthIndex: "b", Name: "b.json", Provider: "xai"},
		{AuthIndex: "c", Name: "c.json", Provider: "xai"},
		{AuthIndex: "d", Name: "d.json", Provider: "xai"},
	}
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: files}, nil
	}
	// Deterministic: always pick first n after shuffle with fixed seed.
	newSampleRand = func() *rand.Rand { return rand.New(rand.NewSource(42)) }

	var probeMu sync.Mutex
	probed := map[string]int{}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		probeMu.Lock()
		probed[file.AuthIndex]++
		probeMu.Unlock()
		return accountResult{
			AuthIndex:      file.AuthIndex,
			Name:           file.Name,
			FileName:       file.Name,
			Classification: "healthy",
			Action:         "keep",
			Reason:         "ok",
		}
	}

	storePath := filepath.Join(t.TempDir(), "results.json")
	setStoreFilePathForTest(storePath)
	defer setStoreFilePathForTest("")

	e := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{AuthIndex: "hist", Name: "hist.json", FileName: "hist.json", Classification: "quota_exhausted", Action: "disable", Reason: "old"},
			{AuthIndex: "a", Name: "a.json", FileName: "a.json", Classification: "probe_error", Action: "keep", Reason: "old-a"},
		},
	}
	if err := e.start(startRequest{Workers: 2, Sample: true, SampleCount: 2, Lang: "en"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !e.snapshot(false).Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.runWG.Wait()
	e.waitAsyncPersist()

	snap := e.snapshot(true)
	if snap.Sample != true {
		t.Fatalf("sample flag missing: %+v", snap)
	}
	if snap.Total != 2 || snap.Done != 2 {
		t.Fatalf("progress=%d/%d", snap.Done, snap.Total)
	}
	probeMu.Lock()
	probedCopy := map[string]int{}
	for k, v := range probed {
		probedCopy[k] = v
	}
	probeMu.Unlock()
	if len(probedCopy) != 2 {
		t.Fatalf("probed=%v want 2 accounts", probedCopy)
	}
	probed = probedCopy
	// History row must remain.
	var hist, a *accountResult
	for i := range snap.Results {
		r := &snap.Results[i]
		if r.AuthIndex == "hist" {
			hist = r
		}
		if r.AuthIndex == "a" {
			a = r
		}
	}
	if hist == nil || hist.Classification != "quota_exhausted" || hist.Reason != "old" {
		t.Fatalf("history not preserved: %+v", hist)
	}
	// If a was sampled it becomes healthy; if not, old-a remains.
	if a == nil {
		t.Fatal("row a missing")
	}
	if probed["a"] > 0 {
		if a.Classification != "healthy" {
			t.Fatalf("sampled a not updated: %+v", a)
		}
	} else if a.Reason != "old-a" {
		t.Fatalf("unsampled a should keep history: %+v", a)
	}
	// Overall list grows only by newly seen sampled accounts, not wiped.
	if len(snap.Results) < 2 {
		t.Fatalf("results wiped? len=%d", len(snap.Results))
	}
}

func TestSampleClassifyScopedKeepsOtherCategories(t *testing.T) {
	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	oldRand := newSampleRand
	defer func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
		newSampleRand = oldRand
	}()
	newSampleRand = func() *rand.Rand { return rand.New(rand.NewSource(7)) }

	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "q1", Name: "q1.json", Provider: "xai"},
		{AuthIndex: "q2", Name: "q2.json", Provider: "xai"},
		{AuthIndex: "q3", Name: "q3.json", Provider: "xai"},
		{AuthIndex: "h1", Name: "h1.json", Provider: "xai"},
	}
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: files}, nil
	}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		return accountResult{
			AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name,
			Classification: "healthy", Action: "keep",
		}
	}
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
	defer setStoreFilePathForTest("")

	e := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{AuthIndex: "q1", Name: "q1.json", FileName: "q1.json", Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "q2", Name: "q2.json", FileName: "q2.json", Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "q3", Name: "q3.json", FileName: "q3.json", Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "h1", Name: "h1.json", FileName: "h1.json", Classification: "healthy", Action: "keep"},
		},
	}
	if err := e.start(startRequest{
		Workers: 2, Sample: true, SampleCount: 1,
		Classifications: []string{"quota_exhausted"},
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !e.snapshot(false).Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.runWG.Wait()
	e.waitAsyncPersist()
	snap := e.snapshot(true)
	if snap.Total != 1 {
		t.Fatalf("sample within category total=%d", snap.Total)
	}
	// healthy history untouched
	var healthyKeep bool
	quotaHealthy := 0
	for _, r := range snap.Results {
		if r.AuthIndex == "h1" && r.Classification == "healthy" {
			healthyKeep = true
		}
		if r.AuthIndex == "q1" || r.AuthIndex == "q2" || r.AuthIndex == "q3" {
			if r.Classification == "healthy" {
				quotaHealthy++
			}
		}
	}
	if !healthyKeep {
		t.Fatal("other category history lost")
	}
	if quotaHealthy != 1 {
		t.Fatalf("expected exactly one quota row updated, got %d", quotaHealthy)
	}
}

func TestSampleCategoryRespectsDisabledFilters(t *testing.T) {
	oldList := callHostAuthListFn
	oldProbe := inspectAccountFn
	oldRand := newSampleRand
	defer func() {
		callHostAuthListFn = oldList
		inspectAccountFn = oldProbe
		newSampleRand = oldRand
	}()
	// Deterministic but order does not matter: only one disabled quota row is eligible.
	newSampleRand = func() *rand.Rand { return rand.New(rand.NewSource(1)) }

	files := []pluginapi.HostAuthFileEntry{
		{AuthIndex: "qd", Name: "qd.json", Provider: "xai", Disabled: true, Status: "disabled"},
		{AuthIndex: "qe", Name: "qe.json", Provider: "xai", Disabled: false, Status: "active"},
		{AuthIndex: "he", Name: "he.json", Provider: "xai", Disabled: false, Status: "active"},
	}
	callHostAuthListFn = func() (authListResponse, error) {
		return authListResponse{Files: files}, nil
	}
	var probeMu sync.Mutex
	probed := map[string]int{}
	inspectAccountFn = func(file pluginapi.HostAuthFileEntry, model string, lang Lang) accountResult {
		probeMu.Lock()
		probed[file.AuthIndex]++
		probeMu.Unlock()
		return accountResult{
			AuthIndex: file.AuthIndex, Name: file.Name, FileName: file.Name,
			Disabled: file.Disabled, Classification: "healthy", Action: "keep",
		}
	}
	setStoreFilePathForTest(filepath.Join(t.TempDir(), "results.json"))
	defer setStoreFilePathForTest("")

	e := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{AuthIndex: "qd", Name: "qd.json", FileName: "qd.json", Disabled: true, Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "qe", Name: "qe.json", FileName: "qe.json", Disabled: false, Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "he", Name: "he.json", FileName: "he.json", Disabled: false, Classification: "healthy", Action: "keep"},
		},
	}
	// Sample within quota category, only disabled accounts.
	if err := e.start(startRequest{
		Workers:         2,
		Sample:          true,
		SampleCount:     10,
		OnlyDisabled:    true,
		Classifications: []string{"quota_exhausted"},
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !e.snapshot(false).Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	e.runWG.Wait()
	e.waitAsyncPersist()

	snap := e.snapshot(true)
	if snap.Total != 1 || snap.Done != 1 {
		t.Fatalf("only disabled quota target expected 1/1, got %d/%d probed=%v", snap.Done, snap.Total, probed)
	}
	probeMu.Lock()
	qd, qe, he := probed["qd"], probed["qe"], probed["he"]
	probeMu.Unlock()
	if qd != 1 || qe != 0 || he != 0 {
		t.Fatalf("probed=%v, want only qd", map[string]int{"qd": qd, "qe": qe, "he": he})
	}
	// Enabled quota row and other category must stay as history.
	for _, r := range snap.Results {
		if r.AuthIndex == "qe" && r.Classification != "quota_exhausted" {
			t.Fatalf("unsampled enabled quota row changed: %+v", r)
		}
		if r.AuthIndex == "he" && r.Classification != "healthy" {
			t.Fatalf("other category changed: %+v", r)
		}
	}

	// Include-disabled sample of category should allow both quota rows (count 2).
	probeMu.Lock()
	probed = map[string]int{}
	probeMu.Unlock()
	e2 := &inspectionEngine{
		workers: defaultWorkers,
		results: []accountResult{
			{AuthIndex: "qd", Name: "qd.json", FileName: "qd.json", Disabled: true, Classification: "quota_exhausted", Action: "disable"},
			{AuthIndex: "qe", Name: "qe.json", FileName: "qe.json", Disabled: false, Classification: "quota_exhausted", Action: "disable"},
		},
	}
	if err := e2.start(startRequest{
		Workers:         2,
		Sample:          true,
		SampleCount:     10,
		IncludeDisabled: true,
		Classifications: []string{"quota_exhausted"},
	}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !e2.snapshot(false).Running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	e2.runWG.Wait()
	e2.waitAsyncPersist()
	if e2.snapshot(false).Total != 2 {
		t.Fatalf("include disabled category sample total=%d probed=%v", e2.snapshot(false).Total, probed)
	}
	probeMu.Lock()
	qd, qe = probed["qd"], probed["qe"]
	probeMu.Unlock()
	if qd != 1 || qe != 1 {
		t.Fatalf("include-disabled should probe both quota rows: qd=%d qe=%d", qd, qe)
	}
}

