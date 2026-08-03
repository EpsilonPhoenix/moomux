package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readSettings(t *testing.T, worktreePath string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(worktreePath, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal settings.json: %v", err)
	}
	return m
}

func hookCommands(t *testing.T, settings map[string]any, event string) []string {
	t.Helper()
	hooks, _ := settings["hooks"].(map[string]any)
	entries, _ := hooks[event].([]any)
	var cmds []string
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		for _, rc := range asSlice(entry["hooks"]) {
			c, ok := rc.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := c["command"].(string); cmd != "" {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

func TestEnsureWorktreeHooksCreatesSettings(t *testing.T) {
	wt := t.TempDir()
	if err := EnsureWorktreeHooks(wt); err != nil {
		t.Fatalf("EnsureWorktreeHooks: %v", err)
	}
	settings := readSettings(t, wt)
	if got := hookCommands(t, settings, "Notification"); len(got) != 1 || got[0] != "moomux hook claude set" {
		t.Fatalf("Notification hooks = %v", got)
	}
	if got := hookCommands(t, settings, "PreToolUse"); len(got) != 1 || got[0] != "moomux hook claude clear" {
		t.Fatalf("PreToolUse hooks = %v", got)
	}
	if got := hookCommands(t, settings, "UserPromptSubmit"); len(got) != 1 || got[0] != "moomux hook claude clear" {
		t.Fatalf("UserPromptSubmit hooks = %v", got)
	}
}

func TestEnsureWorktreeHooksIsIdempotent(t *testing.T) {
	wt := t.TempDir()
	if err := EnsureWorktreeHooks(wt); err != nil {
		t.Fatalf("first EnsureWorktreeHooks: %v", err)
	}
	if err := EnsureWorktreeHooks(wt); err != nil {
		t.Fatalf("second EnsureWorktreeHooks: %v", err)
	}
	settings := readSettings(t, wt)
	if got := hookCommands(t, settings, "Notification"); len(got) != 1 {
		t.Fatalf("expected no duplicate Notification hook, got %v", got)
	}
}

// This runs on every OpenSession (see App.repairNeedsInputHooks), often
// against a worktree whose Claude Code process is live and reading this same
// file — a redundant write is a needless window for it to observe a
// half-written settings.json, so a no-op call must not touch the file.
func TestEnsureWorktreeHooksSkipsNoopWrite(t *testing.T) {
	wt := t.TempDir()
	if err := EnsureWorktreeHooks(wt); err != nil {
		t.Fatalf("first EnsureWorktreeHooks: %v", err)
	}
	path := filepath.Join(wt, ".claude", "settings.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// mtime resolution can be coarse; back-date the file so a real rewrite
	// would be detectable even on filesystems with 1s granularity.
	backdated := before.ModTime().Add(-time.Hour)
	if err := os.Chtimes(path, backdated, backdated); err != nil {
		t.Fatal(err)
	}

	if err := EnsureWorktreeHooks(wt); err != nil {
		t.Fatalf("second EnsureWorktreeHooks: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(backdated) {
		t.Fatalf("expected no-op call to leave the file untouched, mtime changed from %v to %v", backdated, after.ModTime())
	}
}

func TestEnsureWorktreeHooksPreservesExistingConfig(t *testing.T) {
	wt := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wt, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
		"otherSetting": true,
		"hooks": {
			"Stop": [{"hooks": [{"type": "command", "command": "echo done"}]}]
		}
	}`
	if err := os.WriteFile(filepath.Join(wt, ".claude", "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureWorktreeHooks(wt); err != nil {
		t.Fatalf("EnsureWorktreeHooks: %v", err)
	}
	settings := readSettings(t, wt)
	if settings["otherSetting"] != true {
		t.Fatalf("expected unrelated setting to survive, got %v", settings["otherSetting"])
	}
	if got := hookCommands(t, settings, "Stop"); len(got) != 1 || got[0] != "echo done" {
		t.Fatalf("expected existing Stop hook to survive, got %v", got)
	}
	if got := hookCommands(t, settings, "Notification"); len(got) != 1 || got[0] != "moomux hook claude set" {
		t.Fatalf("Notification hooks = %v", got)
	}
}
