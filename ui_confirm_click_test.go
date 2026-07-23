package main

import (
	"strings"
	"testing"
)

func TestConfirmDialogClosesOnClickWithoutGlobalDelayShield(t *testing.T) {
	page := string(renderUIPage(pluginName))

	start := strings.Index(page, "function bindConfirmAction(el, value)")
	end := strings.Index(page[start:], "function confirmDialog(")
	if start < 0 || end < 0 {
		t.Fatal("confirm action binding not found")
	}
	binding := page[start : start+end]

	for _, marker := range []string{
		"el.onpointerdown = null;",
		"el.onclick = (ev) => {",
		"ev.preventDefault();",
		"ev.stopPropagation();",
		"ev.stopImmediatePropagation",
	} {
		if !strings.Contains(binding, marker) {
			t.Fatalf("click-only confirm binding missing %q", marker)
		}
	}
	if !strings.Contains(page, "modal.onpointerdown = null;") {
		t.Fatal("confirm overlay must not close on pointerdown")
	}
	for _, forbidden := range []string{
		"el.onpointerdown = fire",
		"installGhostClickShield",
		"confirmArmTimer",
		"ghostClickUntil",
		"okEl.disabled = true",
	} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("confirm dialog must not use delayed global shielding: found %q", forbidden)
		}
	}
}
