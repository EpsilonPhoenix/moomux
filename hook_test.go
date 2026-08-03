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

func TestHandleCodexHookSetWritesMarker(t *testing.T) {
	home := t.TempDir()
	payload := `{"cwd":"/wt/a"}`
	if err := handleCodexHook("set", strings.NewReader(payload), home); err != nil {
		t.Fatalf("handleCodexHook set: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".codex", "moomux-notify"))
	if err != nil {
		t.Fatalf("read marker dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one marker file, got %v", entries)
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "moomux-notify", entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "needs-input") || !strings.Contains(string(data), "/wt/a") {
		t.Fatalf("unexpected marker contents: %s", data)
	}
}

func TestHandleCodexHookClearRemovesMarker(t *testing.T) {
	home := t.TempDir()
	if err := handleCodexHook("set", strings.NewReader(`{"cwd":"/wt/a"}`), home); err != nil {
		t.Fatalf("handleCodexHook set: %v", err)
	}
	if err := handleCodexHook("clear", strings.NewReader(`{"cwd":"/wt/a"}`), home); err != nil {
		t.Fatalf("handleCodexHook clear: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(home, ".codex", "moomux-notify"))
	if err != nil {
		t.Fatalf("read marker dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected marker removed, dir has %v", entries)
	}
}

func TestHandleCodexHookRejectsMissingCWD(t *testing.T) {
	home := t.TempDir()
	if err := handleCodexHook("set", strings.NewReader(`{"session_id":"sess1"}`), home); err == nil {
		t.Fatal("expected an error for missing cwd")
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
