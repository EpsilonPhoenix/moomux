package claudehook

import (
	"os"
	"path/filepath"
)

// killCommand is the personal slash command body installed at
// ~/.claude/commands/kill.md. It shells out to moomux's own tab-closing
// KillTmux path (see App.KillTmux) via the plain CLI (`moomux park`), so
// /kill works the same way inside a moomux-managed Claude session in any
// project — not just this repo — without the agent having to decide, from
// ambiguous chat text, whether the word "kill" meant something destructive
// right now.
//
// Despite the name, this parks rather than deletes: it stops the tmux
// session and closes its terminal tab but keeps the worktree/branch, same
// as moomux's own "x" (park) key — the description spells that out so it's
// not a surprise.
const killCommand = "---\n" +
	"description: Park this moomux session (stop tmux, close its tab, keep the worktree/branch)\n" +
	"allowed-tools: Bash(moomux park:*)\n" +
	"---\n" +
	"\n" +
	"!`moomux park`\n"

// EnsureKillCommandInstalled writes the /kill personal slash command to
// ~/.claude/commands/kill.md, mirroring EnsureHooksInstalled's no-op-when-
// unchanged and permission-preserving behavior. changed reports whether
// this call actually wrote the file (new install or content change).
func EnsureKillCommandInstalled(home string) (changed bool, err error) {
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

// EnsureAllInstalled installs both the needs-input hooks and the /kill
// slash command, so needsInputInstallers (see app.go) can keep treating
// "set up this session's Claude integration" as a single function call.
// changed is true if either write actually happened.
func EnsureAllInstalled(home string) (changed bool, err error) {
	hooksChanged, err := EnsureHooksInstalled(home)
	if err != nil {
		return false, err
	}
	cmdChanged, err := EnsureKillCommandInstalled(home)
	if err != nil {
		return false, err
	}
	return hooksChanged || cmdChanged, nil
}
