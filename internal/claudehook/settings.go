// Package claudehook installs moomux's Claude Code integrations — custom
// commands and workspace trust — and removes the needs-input hooks older
// versions installed (see RemoveHooks).
package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/erickgnclvs/moomux/internal/atomicfile"
)

// hookCommands are the commands older moomux versions installed as Claude
// Code hooks in ~/.claude/settings.json.
var hookCommands = []string{"moomux hook claude set", "moomux hook claude clear"}

// RemoveHooks deletes the needs-input hooks older moomux versions installed
// into ~/.claude/settings.json, and the <session-id>.notify.json markers
// those hooks wrote into ~/.claude/sessions. Claude Code reports the same
// thing itself now: its session file says "waiting" while a dialog is open,
// and its transcript shows a turn that ended on a question (see
// internal/watcher). The hooks cost a process spawn on every tool call,
// reported permission prompts 6s late, turned every finished turn into
// needs-input after a 60s idle timer, and left markers that outlived the
// sessions that wrote them.
//
// Runs on every session open (see app.agentInstallers), so an upgrade cleans
// up after itself; once nothing is left it writes nothing. changed reports
// whether it removed anything.
func RemoveHooks(home string) (changed bool, err error) {
	changed, err = removeHookEntries(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return changed, err
	}
	n, err := removeMarkers(filepath.Join(home, ".claude", "sessions"))
	return changed || n > 0, err
}

// removeHookEntries drops moomux's own hooks from the settings file at path,
// leaving every other hook, and every other setting, as it was.
func removeHookEntries(path string) (bool, error) {
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	settings := map[string]any{}
	if err := json.Unmarshal(existing, &settings); err != nil {
		return false, err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if !removeCommands(hooks) {
		// This runs on every session open, often while some Claude Code
		// session is live and reading this same file — never rewrite it for
		// nothing.
		return false, nil
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	return true, writeFileAtomic(path, append(data, '\n'))
}

// removeCommands deletes every hook whose command is one of hookCommands,
// then any matcher group, and any event, that this left empty. It reports
// whether it removed anything.
func removeCommands(hooks map[string]any) bool {
	removed := false
	for event, raw := range hooks {
		groups, ok := raw.([]any)
		if !ok {
			continue
		}
		var kept []any
		touched := false
		for _, g := range groups {
			group, _ := g.(map[string]any)
			list, ok := group["hooks"].([]any)
			if !ok {
				kept = append(kept, g)
				continue
			}
			rest := slices.DeleteFunc(slices.Clone(list), func(h any) bool {
				c, _ := h.(map[string]any)
				cmd, _ := c["command"].(string)
				return slices.Contains(hookCommands, cmd)
			})
			if len(rest) == len(list) {
				kept = append(kept, g)
				continue
			}
			touched = true
			if len(rest) > 0 {
				group["hooks"] = rest
				kept = append(kept, group)
			}
		}
		if !touched {
			continue
		}
		removed = true
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
		}
	}
	return removed
}

// removeMarkers deletes the needs-input markers the old hooks wrote into dir,
// returning how many. A file only counts as one when it has both the name
// and the content moomux gave it — this is Claude Code's directory, not
// moomux's.
func removeMarkers(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".notify.json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var m struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(data, &m) != nil || m.Status != "needs-input" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return n, err
		}
		n++
	}
	return n, nil
}

// writeFileAtomic writes data to path atomically (so a concurrent reader —
// Claude Code reloading its own config — never observes a partially-written
// file), preserving the existing file's permission bits rather than
// hardcoding one: settings.json can carry sensitive env values, and a user
// who's locked it down to 0600 shouldn't have that silently loosened back to
// 0644 the next time this runs. A brand-new file defaults to 0600 rather
// than the more permissive 0644.
func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return atomicfile.Write(path, data, mode)
}
