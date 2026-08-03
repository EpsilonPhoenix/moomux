package claudehook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

// EnsureWorktreeHooks merges the Notification/PreToolUse/UserPromptSubmit
// hooks that report "needs input" into the worktree's .claude/settings.json,
// preserving any hooks already configured there. Safe to call more than
// once: it won't add a duplicate entry for a hook it already installed.
// changed reports whether this call actually wrote the file (new install or
// content change) as opposed to a no-op — callers can use that to decide
// whether anything needs telling the user about (e.g. Claude re-prompting to
// trust changed project hooks).
func EnsureWorktreeHooks(worktreePath string) (changed bool, err error) {
	path := filepath.Join(worktreePath, ".claude", "settings.json")

	settings := map[string]any{}
	existing, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(existing, &settings); err != nil {
			return false, err
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	addHook(hooks, "Notification", "permission_prompt|idle_prompt|agent_needs_input", "moomux hook claude set")
	addHook(hooks, "PreToolUse", "*", "moomux hook claude clear")
	addHook(hooks, "UserPromptSubmit", "", "moomux hook claude clear")
	settings["hooks"] = hooks

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	if bytes.Equal(data, existing) {
		// This runs on every session open (see App.repairNeedsInputHooks), not
		// just once at creation — often against a worktree whose Claude Code
		// process is live and reading this same file. Skipping a no-op write
		// avoids most opportunities for it to observe a half-written file.
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return false, err
	}
	return true, nil
}

// writeFileAtomic writes data via a temp file + rename in path's directory,
// so a concurrent reader (Claude Code reloading its own hook config) never
// observes a partially-written file.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once the rename below succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// addHook appends a {matcher, hooks:[{type:command,command}]} entry for
// event, unless one with the same matcher and command already exists.
func addHook(hooks map[string]any, event, matcher, command string) {
	list, _ := hooks[event].([]any)
	for _, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if m, _ := entry["matcher"].(string); m != matcher {
			continue
		}
		for _, rc := range asSlice(entry["hooks"]) {
			c, ok := rc.(map[string]any)
			if !ok {
				continue
			}
			if cmd, _ := c["command"].(string); cmd == command {
				return // already present
			}
		}
	}
	entry := map[string]any{
		"hooks": []any{map[string]any{"type": "command", "command": command}},
	}
	if matcher != "" {
		entry["matcher"] = matcher
	}
	hooks[event] = append(list, entry)
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
