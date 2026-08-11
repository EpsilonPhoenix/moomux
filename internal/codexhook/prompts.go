package codexhook

import (
	"os"
	"path/filepath"
)

// killPrompt is the custom prompt installed at ~/.codex/prompts/kill.md,
// Codex's analog of claudehook's /kill slash command.
//
// Unlike Claude Code's bang-backtick `!command` syntax, Codex custom prompts have no
// bash-execution-during-expansion mechanism — per
// developers.openai.com/codex/custom-prompts, frontmatter only supports
// description/argument-hint, and the body is plain text with $1../
// $ARGUMENTS placeholders. So invoking this just hands Codex an
// instruction; it still has to decide to actually run the command via its
// own shell tool, same as any other prompt. The wording below is
// deliberately direct ("run it now, don't ask") to make that one extra
// hop as reliable as it can be.
//
// Also worth knowing: OpenAI marks custom prompts deprecated in favor of
// "skills" as of this writing, and invocation syntax has drifted across
// CLI versions (bare /kill in some, /prompts:kill in others per
// github.com/openai/codex/issues/15941) — this is the best mechanism
// available today, not a guaranteed-stable one.
const killPrompt = `---
description: Park this moomux session (stop tmux, close its tab, keep the worktree/branch)
---

Run ` + "`moomux park`" + ` in a shell now — don't ask for confirmation first — to park this moomux session. It stops the tmux session and closes its terminal tab, but keeps the worktree/branch so the session can be reopened later (same as moomux's own "x" key). Then report what it printed.
`

// EnsureKillPromptInstalled writes the /kill custom prompt to
// ~/.codex/prompts/kill.md, mirroring EnsureHooks's no-op-when-unchanged
// behavior. changed reports whether this call actually wrote the file.
func EnsureKillPromptInstalled(home string) (changed bool, err error) {
	path := filepath.Join(home, ".codex", "prompts", "kill.md")

	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == killPrompt {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := writeFileAtomic(path, []byte(killPrompt)); err != nil {
		return false, err
	}
	return true, nil
}

// EnsureAllInstalled installs both the needs-input hooks and the /kill
// custom prompt, so needsInputInstallers (see app.go) can keep treating
// "set up this session's Codex integration" as a single function call.
// changed is true if either write actually happened.
func EnsureAllInstalled(home string) (changed bool, err error) {
	hooksChanged, err := EnsureHooks(home)
	if err != nil {
		return false, err
	}
	promptChanged, err := EnsureKillPromptInstalled(home)
	if err != nil {
		return false, err
	}
	return hooksChanged || promptChanged, nil
}
