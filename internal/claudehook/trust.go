package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// TrustDirectory pre-approves dir in Claude Code's own trust store
// (~/.claude.json) so `claude` doesn't stop at its "Do you trust the files
// in this folder?" dialog on first launch. Every moomux session runs in a
// brand-new worktree directory Claude has never seen, so without this the
// dialog blocks the agent's real input on every session — including
// anything typed in programmatically (see App.StartFirstPrompt), since
// there's nobody there to click through it.
//
// Unlike EnsureHooksInstalled's settings.json, this file is keyed per
// project directory by design (that's the whole point of workspace trust),
// so this only ever touches the entry for dir — every other project's trust
// state is left untouched.
func TrustDirectory(home, dir string) error {
	path := filepath.Join(home, ".claude.json")

	root := map[string]any{}
	existing, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(existing, &root); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	entry, _ := root[dir].(map[string]any)
	if entry == nil {
		entry = map[string]any{}
	}
	entry["hasTrustDialogAccepted"] = true
	entry["hasCompletedProjectOnboarding"] = true
	root[dir] = entry

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}
