package claudehook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/erickgnclvs/moomux/internal/atomicfile"
)

// EnsureHooksInstalled merges the Notification/PreToolUse/UserPromptSubmit
// hooks that report "needs input" into the user's global ~/.claude/settings.json,
// preserving any hooks already configured there. Safe to call more than
// once: it won't add a duplicate entry for a hook it already installed.
// changed reports whether this call actually wrote the file (new install or
// content change) as opposed to a no-op.
//
// This is global rather than per-worktree deliberately, mirroring
// codexhook.EnsureHooks: Claude Code requires workspace-trust approval
// before it will run hooks defined in a *project-local* .claude/settings.json
// — trust is scoped per directory, so a per-worktree file would mean
// re-approving on every new worktree. Hooks in the global, user-level
// settings.json carry no such prompt (they're the user's own account-level
// config, not repo-controlled), so installing there once covers every
// worktree with no trust dialog at all — unlike Codex, there's no
// equivalent re-approval step needed after this changes, so callers don't
// need to show a hint the way they do for codexhook.EnsureHooks.
//
// Deliberately does NOT install a Stop hook. Claude Code has no dedicated
// hook for "the agent asked a plain-text question via AskUserQuestion and is
// waiting on a reply" (see anthropics/claude-code#59908, open as of this
// writing) — Stop would close that gap, but it fires unconditionally at the
// end of every turn, not just ones ending in a question. Since NeedsInput
// outranks Done in scanDir's max-merge (see internal/watcher/watcher.go),
// that would make Done unreachable for Claude sessions — every finished
// turn would show needs-input until the next message, which is exactly the
// "Waiting never carried any stuck meaning" distinction commit d9593ed
// established between Done and NeedsInput. codexhook.EnsureHooks has the
// identical gap and the identical reason for leaving it unfixed. Left as a
// known gap rather than reintroducing that.
func EnsureHooksInstalled(home string) (changed bool, err error) {
	path := filepath.Join(home, ".claude", "settings.json")

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
		// just once at creation — often while some other Claude Code session
		// is live and reading this same global file. Skipping a no-op write
		// avoids most opportunities for it to observe a half-written file.
		return false, nil
	}
	if err := writeFileAtomic(path, data); err != nil {
		return false, err
	}
	return true, nil
}

// writeFileAtomic writes data to path atomically (so a concurrent reader —
// Claude Code reloading its own hook config — never observes a
// partially-written file), preserving the existing file's permission bits
// rather than hardcoding one: settings.json can carry sensitive env values,
// and a user who's locked it down to 0600 shouldn't have that silently
// loosened back to 0644 the next time this runs. A brand-new file defaults
// to 0600 rather than the more permissive 0644.
func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return atomicfile.Write(path, data, mode)
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
