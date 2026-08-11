package claudehook

import (
	"os"
	"path/filepath"
)

// killCommand is installed as the body of the /kill custom command — see
// EnsureKillCommand. Despite the name it parks rather than deletes: it
// stops the tmux session and closes its terminal tab but keeps the
// worktree/branch, same as moomux's own "x" (park) key — the description
// spells that out so it's not a surprise. Runs `moomux park` (App.KillTmux's
// CLI backend) rather than leaving the agent to decide, from ambiguous chat
// text, whether the word "kill" meant something destructive right now.
const killCommand = `---
description: Park this moomux session (stop tmux, close its tab, keep the worktree/branch)
allowed-tools: Bash(moomux park:*)
---

!` + "`moomux park`" + `
`

// EnsureKillCommand installs the /kill custom command into the user's
// global ~/.claude/commands/kill.md, so it's available from every worktree
// with no per-project trust approval — unlike claudehook's hooks, a custom
// command only ever runs when the user explicitly types /kill, so Claude
// Code requires none. Safe to call more than once: it only writes when
// content actually changed. changed reports whether this call wrote the
// file.
func EnsureKillCommand(home string) (changed bool, err error) {
	path := filepath.Join(home, ".claude", "commands", "kill.md")

	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == killCommand {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := writeFileAtomic(path, []byte(killCommand)); err != nil {
		return false, err
	}
	return true, nil
}
