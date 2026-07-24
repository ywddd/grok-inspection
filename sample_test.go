package main

import (
	"math/rand"
	"testing"

	"grok-inspection/cpasdk/pluginapi"
)

func TestResolveSampleSizeCountOnly(t *testing.T) {
	n, err := resolveSampleSize(100, 20, 0)
	if err != nil || n != 20 {
		t.Fatalf("count-only: n=%d err=%v", n, err)
	}
	n, err = resolveSampleSize(10, 50, 0)
	if err != nil || n != 10 {
		t.Fatalf("count capped by population: n=%d err=%v", n, err)
	}
}

func TestResolveSampleSizePercentOnly(t *testing.T) {
	n, err := resolveSampleSize(200, 0, 10)
	if err != nil || n != 20 {
		t.Fatalf("percent-only: n=%d err=%v", n, err)
	}
	// 10% of 7 floors to 0, but a positive percent still yields at least 1.
	n, err = resolveSampleSize(7, 0, 10)
	if err != nil || n != 1 {
		t.Fatalf("small population percent min 1: n=%d err=%v", n, err)
	}
	n, err = resolveSampleSize(50, 0, 1)
	if err != nil || n != 1 {
		t.Fatalf("1%% of 50 should be 1: n=%d err=%v", n, err)
	}
}

func TestResolveSampleSizeBothTakesMin(t *testing.T) {
	// 50% of 100 = 50, count 30 → 30
	n, err := resolveSampleSize(100, 30, 50)
	if err != nil || n != 30 {
		t.Fatalf("min(count,percent)=30: n=%d err=%v", n, err)
	}
	// 10% of 100 = 10, count 40 → 10
	n, err = resolveSampleSize(100, 40, 10)
	if err != nil || n != 10 {
		t.Fatalf("min(count,percent)=10: n=%d err=%v", n, err)
	}
	// count 5 and 1% of 50 -> percent becomes 1, min is 1
	n, err = resolveSampleSize(50, 5, 1)
	if err != nil || n != 1 {
		t.Fatalf("min(count,percent min1)=1: n=%d err=%v", n, err)
	}
}

func TestResolveSampleSizeInvalid(t *testing.T) {
	if _, err := resolveSampleSize(10, -1, 0); err == nil {
		t.Fatal("expected negative count error")
	}
	if _, err := resolveSampleSize(10, 0, 101); err == nil {
		t.Fatal("expected percent>100 error")
	}
	if _, err := resolveSampleSize(10, 0, 0); err == nil {
		t.Fatal("expected params required error")
	}
	n, err := resolveSampleSize(0, 5, 10)
	if err != nil || n != 0 {
		t.Fatalf("empty population: n=%d err=%v", n, err)
	}
}

func TestSampleAuthEntriesDeterministic(t *testing.T) {
	entries := make([]pluginapi.HostAuthFileEntry, 10)
	for i := range entries {
		entries[i] = pluginapi.HostAuthFileEntry{AuthIndex: string(rune('a' + i)), Name: string(rune('a'+i)) + ".json", Provider: "xai"}
	}
	rnd := rand.New(rand.NewSource(1))
	got := sampleAuthEntries(entries, 3, rnd)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	// Same seed → same subset order from partial shuffle.
	rnd2 := rand.New(rand.NewSource(1))
	got2 := sampleAuthEntries(entries, 3, rnd2)
	for i := range got {
		if got[i].AuthIndex != got2[i].AuthIndex {
			t.Fatalf("non-deterministic at %d: %q vs %q", i, got[i].AuthIndex, got2[i].AuthIndex)
		}
	}
	// Input not mutated.
	if entries[0].AuthIndex != "a" {
		t.Fatal("input slice mutated")
	}
	all := sampleAuthEntries(entries, 100, rand.New(rand.NewSource(2)))
	if len(all) != 10 {
		t.Fatalf("n>=len should return all, got %d", len(all))
	}
	if sampleAuthEntries(entries, 0, nil) != nil {
		t.Fatal("n=0 should be nil")
	}
}

func TestNormalizeSampleRequest(t *testing.T) {
	c, p, err := normalizeSampleRequest(false, 5, 10, LangZH)
	if err != nil || c != 0 || p != 0 {
		t.Fatalf("non-sample should clear: c=%d p=%d err=%v", c, p, err)
	}
	c, p, err = normalizeSampleRequest(true, 5, 10, LangZH)
	if err != nil || c != 5 || p != 10 {
		t.Fatalf("sample ok: c=%d p=%d err=%v", c, p, err)
	}
	if _, _, err = normalizeSampleRequest(true, 0, 0, LangEN); err == nil {
		t.Fatal("sample without params must fail")
	}
}
