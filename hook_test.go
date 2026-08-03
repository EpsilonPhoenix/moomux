package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleClaudeHookSetWritesMarker(t *testing.T) {
	home := t.TempDir()
	payload := `{"session_id":"sess1","cwd":"/wt/a"}`
	if err := handleClaudeHook("set", strings.NewReader(payload), home); err != nil {
		t.Fatalf("handleClaudeHook set: %v", err)
	}
	marker := filepath.Join(home, ".claude", "sessions", "sess1.notify.json")
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if !strings.Contains(string(data), "needs-input") || !strings.Contains(string(data), "/wt/a") {
		t.Fatalf("unexpected marker contents: %s", data)
	}
}

func TestHandleClaudeHookClearRemovesMarker(t *testing.T) {
	home := t.TempDir()
	if err := handleClaudeHook("set", strings.NewReader(`{"session_id":"sess1","cwd":"/wt/a"}`), home); err != nil {
		t.Fatalf("handleClaudeHook set: %v", err)
	}
	if err := handleClaudeHook("clear", strings.NewReader(`{"session_id":"sess1","cwd":"/wt/a"}`), home); err != nil {
		t.Fatalf("handleClaudeHook clear: %v", err)
	}
	marker := filepath.Join(home, ".claude", "sessions", "sess1.notify.json")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected marker removed, stat err = %v", err)
	}
}

func TestHandleClaudeHookRejectsMissingSessionID(t *testing.T) {
	home := t.TempDir()
	if err := handleClaudeHook("set", strings.NewReader(`{"cwd":"/wt/a"}`), home); err == nil {
		t.Fatal("expected an error for missing session_id")
	}
}

func TestRunHookRejectsBadArgs(t *testing.T) {
	cases := [][]string{
		nil,
		{"claude"},
		{"claude", "nope"},
		{"unsupported-agent", "set"},
	}
	for _, args := range cases {
		if err := runHook(args); err == nil {
			t.Fatalf("expected an error for args %v", args)
		}
	}
}
