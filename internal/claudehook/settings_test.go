package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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

func writeSettings(t *testing.T, home, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func commandsFor(t *testing.T, settings map[string]any, event string) []string {
	t.Helper()
	hooks, _ := settings["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	var cmds []string
	for _, raw := range groups {
		group, _ := raw.(map[string]any)
		list, _ := group["hooks"].([]any)
		for _, rc := range list {
			c, _ := rc.(map[string]any)
			if cmd, _ := c["command"].(string); cmd != "" {
				cmds = append(cmds, cmd)
			}
		}
	}
	return cmds
}

// installed is ~/.claude/settings.json as an older moomux left it, next to
// hooks and settings of the user's own that must survive the cleanup.
const installed = `{
	"otherSetting": true,
	"hooks": {
		"Notification": [
			{"matcher": "permission_prompt|idle_prompt|agent_needs_input", "hooks": [{"type": "command", "command": "moomux hook claude set"}]},
			{"hooks": [{"type": "command", "command": "notify-me"}]}
		],
		"PreToolUse": [
			{"matcher": "*", "hooks": [{"type": "command", "command": "moomux hook claude clear"}]},
			{"matcher": "Bash", "hooks": [{"type": "command", "command": "check-bash"}]}
		],
		"UserPromptSubmit": [
			{"hooks": [{"type": "command", "command": "moomux hook claude clear"}, {"type": "command", "command": "log-prompt"}]}
		],
		"Stop": [{"hooks": [{"type": "command", "command": "echo done"}]}]
	}
}`

func TestRemoveHooksDropsOnlyMoomuxHooks(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, installed, 0o644)

	changed, err := RemoveHooks(home)
	if err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	settings := readSettings(t, home)
	if settings["otherSetting"] != true {
		t.Errorf("unrelated setting lost: %v", settings["otherSetting"])
	}
	for event, want := range map[string][]string{
		"Notification":     {"notify-me"},
		"PreToolUse":       {"check-bash"},
		"UserPromptSubmit": {"log-prompt"},
		"Stop":             {"echo done"},
	} {
		if got := commandsFor(t, settings, event); !slices.Equal(got, want) {
			t.Errorf("%s hooks = %v, want %v", event, got, want)
		}
	}
}

func TestRemoveHooksDropsEventsItEmpties(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, `{
		"model": "opus",
		"hooks": {
			"Notification": [{"matcher": "permission_prompt|idle_prompt|agent_needs_input", "hooks": [{"type": "command", "command": "moomux hook claude set"}]}],
			"PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "moomux hook claude clear"}]}],
			"UserPromptSubmit": [{"hooks": [{"type": "command", "command": "moomux hook claude clear"}]}]
		}
	}`, 0o644)

	if _, err := RemoveHooks(home); err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	settings := readSettings(t, home)
	if _, ok := settings["hooks"]; ok {
		t.Errorf("hooks = %v, want the emptied key gone", settings["hooks"])
	}
	if settings["model"] != "opus" {
		t.Errorf("model = %v, want opus", settings["model"])
	}
}

// This runs on every session open, often while some Claude Code session is
// live and reading this same file — a redundant write is a needless window
// for it to observe a half-written settings.json.
func TestRemoveHooksLeavesCleanSettingsUntouched(t *testing.T) {
	home := t.TempDir()
	path := writeSettings(t, home, `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo done"}]}]}}`, 0o644)
	// mtime resolution can be coarse; back-date the file so a real rewrite
	// would be detectable even on filesystems with 1s granularity.
	backdated := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, backdated, backdated); err != nil {
		t.Fatal(err)
	}

	changed, err := RemoveHooks(home)
	if err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	if changed {
		t.Error("changed = true for settings with nothing to remove")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(backdated) {
		t.Errorf("settings.json rewritten: mtime %v, want %v", info.ModTime(), backdated)
	}
}

func TestRemoveHooksWithoutSettingsCreatesNothing(t *testing.T) {
	home := t.TempDir()
	changed, err := RemoveHooks(home)
	if err != nil || changed {
		t.Fatalf("RemoveHooks = %v, %v; want false, nil", changed, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("settings.json created: stat err = %v", err)
	}
}

// ~/.claude/sessions is Claude Code's directory, not moomux's: only files
// with both the name and the content the old hooks wrote may go.
func TestRemoveHooksDeletesOnlyMoomuxMarkers(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"sess1.notify.json": `{"cwd":"/wt/a","status":"needs-input"}`,
		"other.notify.json": `{"cwd":"/wt/a","kind":"someone else's"}`,
		"123.json":          `{"cwd":"/wt/a","status":"idle"}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	changed, err := RemoveHooks(home)
	if err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	if !changed {
		t.Error("changed = false after removing a marker")
	}
	for name, wantGone := range map[string]bool{"sess1.notify.json": true, "other.notify.json": false, "123.json": false} {
		_, err := os.Stat(filepath.Join(dir, name))
		if gone := os.IsNotExist(err); gone != wantGone {
			t.Errorf("%s: gone = %v, want %v", name, gone, wantGone)
		}
	}
}

// TestRemoveHooksPreservesFilePermissions guards against silently loosening
// a settings.json a user has deliberately locked down (it can carry
// sensitive env values) back to a more permissive mode on rewrite.
func TestRemoveHooksPreservesFilePermissions(t *testing.T) {
	home := t.TempDir()
	path := writeSettings(t, home, installed, 0o600)

	if _, err := RemoveHooks(home); err != nil {
		t.Fatalf("RemoveHooks: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected existing 0600 permissions to survive, got %o", got)
	}
}

// TestWriteFileAtomicDefaultsNewFileToRestrictivePermissions: a brand-new
// file shouldn't default to the more permissive 0644, since a Claude config
// file can carry sensitive env values.
func TestWriteFileAtomicDefaultsNewFileToRestrictivePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := writeFileAtomic(path, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected new file to default to 0600, got %o", got)
	}
}
