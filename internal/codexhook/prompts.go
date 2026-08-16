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

// ensurePrompt writes body to $CODEX_HOME/prompts/<name>.md (here, always
// ~/.codex/prompts/ — see EnsureHooks's doc comment on why this package
// doesn't honor CODEX_HOME elsewhere either), creating the directory if
// needed and skipping the write when the content is already there. changed
// reports whether this call wrote the file.
//
// Custom prompts live in the global prompts dir so they're available from
// every worktree. Unlike hooks.json, they only ever run when the user
// explicitly types them, so Codex requires no trust/review step for them
// the way it does for hooks.
func ensurePrompt(home, name, body string) (changed bool, err error) {
	path := filepath.Join(home, ".codex", "prompts", name+".md")

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

// EnsureKillPrompt installs the /kill custom prompt — see ensurePrompt.
func EnsureKillPrompt(home string) (changed bool, err error) {
	return ensurePrompt(home, "kill", killPrompt)
}

// tagPrompt is installed as the body of the /tag custom prompt — see
// EnsureTagPrompt. Codex's custom-prompt frontmatter only documents
// description/argument-hint, not Claude Code's allowed-tools, so this
// carries no frontmatter Codex doesn't understand.
const tagPrompt = `---
description: Tag this moomux session with its PR (and ticket, if one is already tracked)
---

Run ` + "`moomux tag`" + ` with no flags first to see what's already tracked on this
session.

Find the open pull request for the current branch (e.g. ` + "`gh pr view --json url,body --jq '.url + \"\\n\" + .body'`" + `) and run:

    moomux tag -pr <that PR URL>

Leave out ` + "`-ticket`" + ` — moomux keeps this session's existing ticket automatically
when you don't pass one. If ` + "`moomux tag`" + ` showed no ticket tracked yet, look for
a ticket link in the PR title/body, the branch name, and recent commit
messages (` + "`git log --oneline -20`" + `). Recognize common
formats: Asana (` + "`https://app.asana.com/.../task/...`" + `), Jira
(` + "`https://<org>.atlassian.net/browse/<KEY>-<num>`" + ` or a bare ` + "`<KEY>-<num>`" + `
you can expand to that URL), and Linear (` + "`https://linear.app/<org>/issue/<KEY>-<num>`" + `).
If you find one, pass it too:
` + "`moomux tag -pr <PR URL> -ticket <ticket URL>`" + `.

If there's no open PR yet, say so instead of guessing one. Don't guess a
ticket link either — only pass ` + "`-ticket`" + ` when you actually found one.
`

// EnsureTagPrompt installs the /tag custom prompt — see ensurePrompt.
func EnsureTagPrompt(home string) (changed bool, err error) {
	return ensurePrompt(home, "tag", tagPrompt)
}
