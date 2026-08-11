package codexhook

import (
	"os"
	"path/filepath"
)

// tagPrompt is installed as the body of the /tag custom prompt — see
// EnsureTagPrompt. Codex's custom-prompt frontmatter only documents
// description/argument-hint, not Claude Code's allowed-tools, so this
// carries no frontmatter Codex doesn't understand.
const tagPrompt = `---
description: Tag this moomux session with its PR (and ticket, if one is already tracked)
---

Find the open pull request for the current branch (e.g. ` + "`gh pr view --json url --jq .url`" + `) and run:

    moomux tag -pr <that PR URL>

Leave out ` + "`-ticket`" + ` — moomux keeps this session's existing ticket automatically
when you don't pass one. If you know a ticket URL for this work that isn't
tracked on the session yet, pass it too:
` + "`moomux tag -pr <PR URL> -ticket <ticket URL>`" + `.

If there's no open PR yet, say so instead of guessing one.
`

// EnsureTagPrompt installs the /tag custom prompt into the user's global
// $CODEX_HOME/prompts/tag.md (here, always ~/.codex/prompts/tag.md — see
// EnsureHooks's doc comment on why this package doesn't honor CODEX_HOME
// elsewhere either), so it's available from every worktree. Unlike
// hooks.json, custom prompts only ever run when the user explicitly types
// /tag, so Codex requires no trust/review step for them the way it does for
// hooks. Safe to call more than once: it only writes when content actually
// changed. changed reports whether this call wrote the file.
func EnsureTagPrompt(home string) (changed bool, err error) {
	path := filepath.Join(home, ".codex", "prompts", "tag.md")

	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == tagPrompt {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := writeFileAtomic(path, []byte(tagPrompt)); err != nil {
		return false, err
	}
	return true, nil
}
