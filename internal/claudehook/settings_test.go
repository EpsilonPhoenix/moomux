package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readSettings(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
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

func TestEnsureHooksInstalledCreatesSettings(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureHooksInstalled(home)
	if err != nil {
		t.Fatalf("EnsureHooksInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for a brand-new install")
	}
	settings := readSettings(t, home)
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

func TestEnsureHooksInstalledIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHooksInstalled(home); err != nil {
		t.Fatalf("first EnsureHooksInstalled: %v", err)
	}
	changed, err := EnsureHooksInstalled(home)
	if err != nil {
		t.Fatalf("second EnsureHooksInstalled: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when hooks are already installed")
	}
	settings := readSettings(t, home)
	if got := hookCommands(t, settings, "Notification"); len(got) != 1 {
		t.Fatalf("expected no duplicate Notification hook, got %v", got)
	}
}

// This runs on every OpenSession (see App.repairNeedsInputHooks), often
// while some other Claude Code session is live and reading this same global
// file — a redundant write is a needless window for it to observe a
// half-written settings.json, so a no-op call must not touch the file.
func TestEnsureHooksInstalledSkipsNoopWrite(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHooksInstalled(home); err != nil {
		t.Fatalf("first EnsureHooksInstalled: %v", err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
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

	changed, err := EnsureHooksInstalled(home)
	if err != nil {
		t.Fatalf("second EnsureHooksInstalled: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false for a no-op call")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(backdated) {
		t.Fatalf("expected no-op call to leave the file untouched, mtime changed from %v to %v", backdated, after.ModTime())
	}
}

func TestEnsureHooksInstalledPreservesExistingConfig(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
		"otherSetting": true,
		"hooks": {
			"Stop": [{"hooks": [{"type": "command", "command": "echo done"}]}]
		}
	}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureHooksInstalled(home); err != nil {
		t.Fatalf("EnsureHooksInstalled: %v", err)
	}
	settings := readSettings(t, home)
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

// TestEnsureHooksInstalledReportsChangedWhenANewEventIsAdded mirrors
// codexhook's test of the same name: guards the scenario that motivated the
// changed return value — an older moomux build installed a subset of these
// hooks, and a newer build adds one more. changed must report true for that
// case, not just for "file didn't exist before."
func TestEnsureHooksInstalledReportsChangedWhenANewEventIsAdded(t *testing.T) {
	home := t.TempDir()
	partial := `{
		"hooks": {
			"Notification": [{"matcher": "permission_prompt|idle_prompt|agent_needs_input", "hooks": [{"type": "command", "command": "moomux hook claude set"}]}],
			"PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "moomux hook claude clear"}]}]
		}
	}`
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(partial), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureHooksInstalled(home)
	if err != nil {
		t.Fatalf("EnsureHooksInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when UserPromptSubmit is newly added to an already-partially-installed file")
	}
	settings := readSettings(t, home)
	if got := hookCommands(t, settings, "UserPromptSubmit"); len(got) != 1 || got[0] != "moomux hook claude clear" {
		t.Fatalf("UserPromptSubmit hooks = %v", got)
	}
}

// TestEnsureHooksInstalledPreservesFilePermissions guards against silently
// loosening a settings.json a user has deliberately locked down (it can
// carry sensitive env values) back to a more permissive mode on rewrite.
func TestEnsureHooksInstalledPreservesFilePermissions(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"otherSetting": true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureHooksInstalled(home); err != nil {
		t.Fatalf("EnsureHooksInstalled: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected existing 0600 permissions to survive, got %o", got)
	}
}

// TestEnsureHooksInstalledDefaultsNewFileToRestrictivePermissions guards the
// other half of the same fix: a brand-new settings.json shouldn't default to
// the more permissive 0644 either, since it can carry sensitive env values.
func TestEnsureHooksInstalledDefaultsNewFileToRestrictivePermissions(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureHooksInstalled(home); err != nil {
		t.Fatalf("EnsureHooksInstalled: %v", err)
	}
	info, err := os.Stat(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected new file to default to 0600, got %o", got)
	}
}
