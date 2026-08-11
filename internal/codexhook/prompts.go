package codexhook

import (
	"os"
	"path/filepath"
)

// killPrompt is installed as the body of the /kill custom prompt — see
// EnsureKillPrompt. Codex's custom-prompt frontmatter only documents
// description/argument-hint, not Claude Code's allowed-tools, and has no
// bash-execution-during-expansion syntax either (Claude Code's bang-backtick
// `!command`) — per developers.openai.com/codex/custom-prompts — so this
// hands Codex an instruction rather than running anything directly; it
// still has to decide to actually run the command via its own shell tool,
// same as any other prompt. The wording below is deliberately direct ("run
// it now, don't ask") to make that one extra hop as reliable as it can be.
//
// Also worth knowing: OpenAI marks custom prompts deprecated in favor of
// "skills" as of this writing, and invocation syntax has drifted across CLI
// versions (bare /kill in some, /prompts:kill in others per
// github.com/openai/codex/issues/15941) — this is the best mechanism
// available today, not a guaranteed-stable one.
const killPrompt = `---
description: Park this moomux session (stop tmux, close its tab, keep the worktree/branch)
---

Run ` + "`moomux park`" + ` in a shell now — don't ask for confirmation first — to park this moomux session. It stops the tmux session and closes its terminal tab, but keeps the worktree/branch so the session can be reopened later (same as moomux's own "x" key). Then report what it printed.
`

// EnsureKillPrompt installs the /kill custom prompt into the user's global
// $CODEX_HOME/prompts/kill.md (here, always ~/.codex/prompts/kill.md — see
// EnsureHooks's doc comment on why this package doesn't honor CODEX_HOME
// elsewhere either), so it's available from every worktree. Unlike
// hooks.json, custom prompts only ever run when the user explicitly types
// /kill, so Codex requires no trust/review step for them the way it does
// for hooks. Safe to call more than once: it only writes when content
// actually changed. changed reports whether this call wrote the file.
func EnsureKillPrompt(home string) (changed bool, err error) {
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
