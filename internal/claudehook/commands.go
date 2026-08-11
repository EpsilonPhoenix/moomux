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

// ensureCommand writes body to ~/.claude/commands/<name>.md, creating the
// directory if needed and skipping the write when the content is already
// there. changed reports whether this call wrote the file.
//
// Custom commands live in the user's global commands dir so they're
// available from every worktree with no per-project trust approval — unlike
// claudehook's hooks, a custom command only ever runs when the user
// explicitly types it, so Claude Code requires none.
func ensureCommand(home, name, body string) (changed bool, err error) {
	path := filepath.Join(home, ".claude", "commands", name+".md")

	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == body {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := writeFileAtomic(path, []byte(body)); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureKillCommand installs the /kill custom command — see ensureCommand.
func EnsureKillCommand(home string) (changed bool, err error) {
	return ensureCommand(home, "kill", killCommand)
}

// tagCommand is installed as the body of the /tag custom command — see
// EnsureTagCommand.
const tagCommand = `---
description: Tag this moomux session with its PR (and ticket, if one is already tracked)
allowed-tools: Bash(gh pr view:*), Bash(moomux tag:*)
---

Find the open pull request for the current branch (e.g. ` + "`gh pr view --json url --jq .url`" + `) and run:

    moomux tag -pr <that PR URL>

Leave out ` + "`-ticket`" + ` — moomux keeps this session's existing ticket automatically
when you don't pass one. If you know a ticket URL for this work that isn't
tracked on the session yet, pass it too:
` + "`moomux tag -pr <PR URL> -ticket <ticket URL>`" + `.

If there's no open PR yet, say so instead of guessing one.
`

// EnsureTagCommand installs the /tag custom command — see ensureCommand.
func EnsureTagCommand(home string) (changed bool, err error) {
	return ensureCommand(home, "tag", tagCommand)
}
