package claudehook

import (
	"os"
	"path/filepath"
)

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

// EnsureTagCommand installs the /tag custom command into the user's global
// ~/.claude/commands/tag.md, so it's available from every worktree with no
// per-project trust approval — unlike claudehook's hooks, a custom command
// only ever runs when the user explicitly types /tag, so Claude Code
// requires none. Safe to call more than once: it only writes when content
// actually changed. changed reports whether this call wrote the file.
func EnsureTagCommand(home string) (changed bool, err error) {
	path := filepath.Join(home, ".claude", "commands", "tag.md")

	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == tagCommand {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := writeFileAtomic(path, []byte(tagCommand)); err != nil {
		return false, err
	}
	return true, nil
}
