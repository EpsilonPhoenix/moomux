package main

import (
	"strings"
	"testing"
)

// TestRenderScreenCoversAllScenarios drives every registered screen scenario
// through renderScreen, the deterministic Go half of scripts/screenshot.sh's
// pipeline (the pty/HTML/Chromium half needs a real display and isn't
// practical to unit-test). A bad key sequence or an unknown screen name
// (e.g. a typo introduced when adding a new scenario) fails here instead of
// only showing up as a blank or broken screenshot.
func TestRenderScreenCoversAllScenarios(t *testing.T) {
	for name := range screens {
		t.Run(name, func(t *testing.T) {
			out, err := renderScreen(name, 100, 32, "", "")
			if err != nil {
				t.Fatalf("renderScreen(%q): %v", name, err)
			}
			if strings.TrimSpace(out) == "" {
				t.Fatalf("renderScreen(%q): empty view", name)
			}
		})
	}
}

func TestRenderScreenUnknownScreen(t *testing.T) {
	if _, err := renderScreen("does-not-exist", 100, 32, "", ""); err == nil {
		t.Fatal("expected error for unknown screen, got nil")
	}
}
