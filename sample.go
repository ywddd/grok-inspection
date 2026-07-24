package main

import (
	"fmt"
	"math/rand"
	"net/http"

	"grok-inspection/cpasdk/pluginapi"
)

// resolveSampleSize picks how many accounts to probe in a sample run.
// - count only: min(count, population)
// - percent only: floor(population * percent / 100), at least 1 when population > 0
// - both: the smaller of the two (still capped by population)
// percent is an integer in [1,100] when used; count is a positive integer when used.
func resolveSampleSize(population, count, percent int) (int, error) {
	if population < 0 {
		population = 0
	}
	if count < 0 {
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("sample_count_invalid"))
	}
	if percent < 0 || percent > 100 {
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("sample_percent_invalid"))
	}
	if count == 0 && percent == 0 {
		return 0, httpErr(http.StatusBadRequest, fmt.Errorf("sample_params_required"))
	}
	if population == 0 {
		return 0, nil
	}

	size := population
	if count > 0 {
		size = count
		if size > population {
			size = population
		}
	}
	if percent > 0 {
		// Floor division can yield 0 for small populations; keep at least one
		// account when the operator asked for a positive percent sample.
		fromPercent := population * percent / 100
		if fromPercent < 1 {
			fromPercent = 1
		}
		if count > 0 {
			if fromPercent < size {
				size = fromPercent
			}
		} else {
			size = fromPercent
		}
	}
	if size < 0 {
		size = 0
	}
	if size > population {
		size = population
	}
	return size, nil
}

// sampleAuthEntries returns a random subset of size n (or all entries when n >= len).
// rnd may be nil; a non-deterministic source is used then. The input slice is not modified.
func sampleAuthEntries(entries []pluginapi.HostAuthFileEntry, n int, rnd *rand.Rand) []pluginapi.HostAuthFileEntry {
	if n <= 0 || len(entries) == 0 {
		return nil
	}
	if n >= len(entries) {
		out := make([]pluginapi.HostAuthFileEntry, len(entries))
		copy(out, entries)
		return out
	}
	if rnd == nil {
		rnd = newSampleRand()
	}
	// Partial Fisher–Yates on a copy so caller's order is preserved for non-sampled paths.
	work := make([]pluginapi.HostAuthFileEntry, len(entries))
	copy(work, entries)
	for i := 0; i < n; i++ {
		j := i + rnd.Intn(len(work)-i)
		work[i], work[j] = work[j], work[i]
	}
	out := make([]pluginapi.HostAuthFileEntry, n)
	copy(out, work[:n])
	return out
}

// newSampleRand is replaced in tests for deterministic sampling.
var newSampleRand = func() *rand.Rand {
	return rand.New(rand.NewSource(rand.Int63()))
}

func normalizeSampleRequest(sample bool, count, percent int, lang Lang) (sampleCount, samplePercent int, err error) {
	if !sample {
		return 0, 0, nil
	}
	if count < 0 {
		return 0, 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "sample_count_invalid")))
	}
	if percent < 0 || percent > 100 {
		return 0, 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "sample_percent_invalid")))
	}
	if count == 0 && percent == 0 {
		return 0, 0, httpErr(http.StatusBadRequest, fmt.Errorf("%s", T(lang, "sample_params_required")))
	}
	return count, percent, nil
}
