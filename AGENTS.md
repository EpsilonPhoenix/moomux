# moomux

A TUI (Bubble Tea) for managing Claude Code / codex / opencode sessions across git worktrees. See README.md for what it does and how to build/run it.

## UI changes

This is a terminal UI — you can't see it render just by reading the Go source. After any change to `internal/tui/` (new fields, layout tweaks, new modes, copy changes, etc.), capture a screenshot of the affected screen(s) and look at it before considering the change done:

```bash
./scripts/screenshot.sh <screen> /tmp/<screen>.png
```

`<screen>` is one of the scenarios `cmd/uishot` knows about — a sample, not the full list (it grows over time): `list`, `new-session`, `new-project`, `tag`, `confirm-delete`, `confirm-delete-project`, `all-sessions`, `edit-project-emoji`, `project-picker`. Run `go run ./cmd/uishot -screen=x` to see the current full list. It renders the real `tui.Model` against a fake backend with canned sample data, so no real projects, git repos, or tmux sessions are needed.

If a change adds a new mode or scenario that isn't covered, add it to the `screens` map in `cmd/uishot/main.go` (drive it there with the same key-press sequence a user would use) rather than skipping the screenshot.

This app is used over mobile/remote SSH clients as narrow as ~40-60 columns (see `narrowWidthBreak` in `internal/tui/view.go`), and that's a real source of bugs: overlay footers, headers, and any hint text that isn't wrapped through `overlayWidth`/a width-aware fallback (see `formFooter`/`helpFooter` for the pattern) will hard-clip mid-word instead of degrading gracefully. For any change touching an overlay, header, or footer, screenshot it at both the default width and a narrow one (`./scripts/screenshot.sh <screen> /tmp/<screen>-narrow.png 60 24`, and a tighter one like `40 20` if there's any doubt) and confirm nothing is truncated or overflowing before considering the change done.

Send the resulting PNG(s) to the user so they can see the change, the same way you'd report a code diff — surface it as a clickable `file://` link (e.g. `[all-sessions.png](file:///tmp/all-sessions.png)`) rather than just viewing it inline, since inline rendering isn't guaranteed to reach the user.

## Bug fixes and logic changes

Every bug fix or non-trivial logic change needs a test that fails without the fix and passes with it — check this by temporarily reverting the fix and confirming the test goes red before restoring it. Add the test in the same commit/PR as the fix, not as a follow-up. Skip only for pure UI/copy tweaks (covered by the screenshot rule above) or one-line changes with no meaningful branch/edge case.
