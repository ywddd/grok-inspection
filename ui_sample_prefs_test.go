package main

import (
	"strings"
	"testing"
)

func TestUISampleInputsPersistAcrossRefresh(t *testing.T) {
	if !strings.Contains(uiScriptCore, "prefs.sampleCount") ||
		!strings.Contains(uiScriptCore, "$('sampleCount').value") {
		t.Fatal("sample count input must be restored from saved prefs on page load")
	}
	if !strings.Contains(uiScriptCore, "prefs.samplePercent") ||
		!strings.Contains(uiScriptCore, "$('samplePercent').value") {
		t.Fatal("sample percent input must be restored from saved prefs on page load")
	}
	if !strings.Contains(uiScriptWire, "sampleCount") ||
		!strings.Contains(uiScriptWire, "samplePercent") ||
		!strings.Contains(uiScriptWire, "saveSamplePrefs") {
		t.Fatal("sample inputs must save prefs when edited")
	}
	if !strings.Contains(uiScriptInspect, "sampleCount: sample.count") ||
		!strings.Contains(uiScriptInspect, "samplePercent: sample.percent") {
		t.Fatal("starting a sample run must persist the submitted sample inputs")
	}
}
