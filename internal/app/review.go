package app

import (
	"errors"
	"fmt"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/filecap"
	"github.com/erickgnclvs/moomux/internal/gitwt"
	"github.com/erickgnclvs/moomux/internal/session"
)

// Capture returns the visible text of each session's active pane, keyed by
// session id — the session grid a front end draws, without it needing a tmux
// binary or moomux's tmux session names.
//
// One tmux invocation for the whole set (see tmux.CapturePanes); ids the
// core doesn't know, and sessions whose tmux is gone, are simply absent from
// the result rather than an error, because a caller polling this every few
// seconds must not read a failure as "the pane is empty".
func (a *App) Capture(ids []string) map[string]string {
	// Parking keeps the session's tmux name, so a name is not liveness —
	// and one dead name aborts the rest of the chained tmux sequence,
	// dropping every session after it onto the per-session fallback. One
	// fork to ask is what makes batching the other N worth it.
	live := a.Tmux.LiveSessions()
	names := make([]string, 0, len(ids))
	byName := make(map[string]string, len(ids))
	for _, id := range ids {
		s, ok := a.Store.Get(id)
		if !ok || s.TmuxSession == "" || !live[s.TmuxSession] {
			continue
		}
		if _, dup := byName[s.TmuxSession]; dup {
			continue
		}
		byName[s.TmuxSession] = id
		names = append(names, s.TmuxSession)
	}
	out := make(map[string]string, len(names))
	for name, screen := range a.Tmux.CapturePanes(names) {
		out[byName[name]] = screen
	}
	return out
}

// Review opens (or reuses) a window named "review" in the session's tmux
// session, running its diff against the base branch.
//
// A tmux window rather than a patch rendered by the front end: the output
// goes to a real tty, so git colours and pages it with the user's own pager
// — a configured delta is honoured, which is most of the argument for
// reviewing here at all — and the window is somewhere to run `git add -p`
// from afterwards. It also works from a client that isn't attached; the
// returned hint is that client's only signal.
func (a *App) Review(id string) (string, error) {
	s, found := a.Store.Get(id)
	if !found {
		return "", fmt.Errorf("unknown session %q", id)
	}
	proj, ok := a.project(s.Project)
	if !ok {
		return "", fmt.Errorf("unknown project %q", s.Project)
	}
	if proj.IsPlain() {
		return "", fmt.Errorf("%s is not a git project", s.Project)
	}
	// A review window needs a live tmux session to be added to, and both
	// respawn-window and new-window fail on a parked one with nothing but an
	// exit status. Deliberately *not* EnsureTmux, unlike Attach: reviving a
	// session relaunches its agent, which is a surprising and expensive side
	// effect of asking to look at a diff. Say what's wrong instead and let
	// the caller open it.
	if s.TmuxSession == "" {
		return "", fmt.Errorf("session %q has no tmux session", id)
	}
	if live, err := a.Tmux.HasSession(s.TmuxSession); err != nil {
		return "", err
	} else if !live {
		return "", fmt.Errorf("%s is parked; open it before reviewing", s.Name)
	}
	// Resolved here, by the same BaseCommit as Diff, so the window and a
	// phone's patch never disagree about what the base is.
	_, commit, err := gitwt.BaseCommit(s.WorktreePath, reviewBase(s, proj))
	if err != nil {
		return "", fmt.Errorf("review: %w", err)
	}
	if err := a.Tmux.ReviewWindow(s.TmuxSession, s.WorktreePath, ReviewScript(commit)); err != nil {
		return "", fmt.Errorf("review window: %w", err)
	}
	return "Opened a review window in " + s.TmuxSession + ".", nil
}

// SelectWindow switches the session's tmux session to its next (delta +1)
// or previous (delta -1) window. A front end used to do this by typing the
// prefix and n/p into the pane, which lands in the agent's prompt for anyone
// with a remapped prefix. Like Review, a parked session is an error, not
// revived.
func (a *App) SelectWindow(id string, delta int) error {
	if delta != 1 && delta != -1 {
		return fmt.Errorf("delta must be 1 or -1, got %d", delta)
	}
	s, found := a.Store.Get(id)
	if !found {
		return fmt.Errorf("unknown session %q", id)
	}
	if s.TmuxSession == "" {
		return fmt.Errorf("session %q has no tmux session", id)
	}
	if live, err := a.Tmux.HasSession(s.TmuxSession); err != nil {
		return err
	} else if !live {
		return fmt.Errorf("%s is parked; open it before switching windows", s.Name)
	}
	return a.Tmux.SelectWindow(s.TmuxSession, delta)
}

// reviewBase is the branch a session's work is diffed against. The
// session's own base is what its branch was actually cut from; the
// project's is the fallback for a resumed branch that never recorded one.
func reviewBase(s session.Session, proj config.Project) string {
	if s.BaseBranch != "" {
		return s.BaseBranch
	}
	if proj.BaseBranch != "" {
		return proj.BaseBranch
	}
	return "main"
}

// diffLimit caps Diff's patch: plenty to read on a phone, and well short of
// a response a client would choke on.
const diffLimit = 2 << 20

// Diff is the session's worktree diffed against the same base Review uses,
// as a raw patch a front end renders itself — the phone has no tty to open
// a review window on. See gitwt.Diff for what is in it and how it is cut
// at the limit.
//
// It reads the worktree and nothing else, so a parked session diffs the
// same as a live one and nothing is revived. ok is false, not an error, for
// a worktree that isn't a git repo (a plain project, or one that has gone
// missing), like ChangeSummary.
func (a *App) Diff(id string) (gitwt.Patch, bool, error) {
	s, found := a.Store.Get(id)
	if !found {
		return gitwt.Patch{}, false, fmt.Errorf("unknown session %q", id)
	}
	proj, found := a.project(s.Project)
	if !found {
		return gitwt.Patch{}, false, fmt.Errorf("unknown project %q", s.Project)
	}
	if proj.IsPlain() {
		return gitwt.Patch{}, false, nil
	}
	p, err := gitwt.Diff(s.WorktreePath, reviewBase(s, proj), diffLimit)
	if errors.Is(err, gitwt.ErrNotGitRepo) {
		return gitwt.Patch{}, false, nil
	}
	if err != nil {
		return gitwt.Patch{}, false, err
	}
	return p, true, nil
}

// ReadBaseFile is path as it was at the base Diff diffs against — the
// "before" side of a changed file, for a front end that renders both. path
// is repo-relative, the patch's a/ side (a rename's old path). commit pins
// the read to the Commit a Diff answered, so the base branch moving between
// the two calls can't pair this with a different patch; empty resolves the
// base afresh, exactly as Diff does. It answers the commit it read.
//
// ok is false, with no error, when the base has no such file — new or
// untracked since. Like Diff it reads the repo and nothing else, so a
// parked session works and nothing is revived; unlike Diff, a plain project
// is an error, since there is no base to ask about.
func (a *App) ReadBaseFile(id, path, commit string) ([]byte, string, bool, error) {
	s, found := a.Store.Get(id)
	if !found {
		return nil, "", false, fmt.Errorf("unknown session %q", id)
	}
	proj, found := a.project(s.Project)
	if !found {
		return nil, "", false, fmt.Errorf("unknown project %q", s.Project)
	}
	if proj.IsPlain() {
		return nil, "", false, fmt.Errorf("%s is not a git project", s.Project)
	}
	if commit == "" {
		var err error
		_, commit, err = gitwt.BaseCommit(s.WorktreePath, reviewBase(s, proj))
		if errors.Is(err, gitwt.ErrNotGitRepo) {
			return nil, "", false, fmt.Errorf("%s has no git worktree at %s", s.Name, s.WorktreePath)
		}
		if err != nil {
			return nil, "", false, err
		}
	}
	data, ok, err := gitwt.ReadAt(s.WorktreePath, commit, path, filecap.Max)
	if errors.Is(err, gitwt.ErrNotGitRepo) {
		// Diff answers ok=false here, but for this method ok=false means
		// "new file", which would be a lie.
		return nil, "", false, fmt.Errorf("%s has no git worktree at %s", s.Name, s.WorktreePath)
	}
	return data, commit, ok, err
}

// ReviewScript is the shell line a review window runs: the session's work
// diffed against commit, which Review resolves with gitwt.BaseCommit.
//
// `git diff <merge base>` is everything not yet on the base branch —
// commits since the merge base *and* uncommitted work — in one command.
// The base is resolved before the window opens, never in the shell: a
// paged diff exits 141 when the user quits the pager early, and a shell
// fallback chain keyed on exit status once re-ran the diff and ended on
// `git diff HEAD`, quietly showing uncommitted work only as the review.
// The `git status` line is not decoration: untracked files are invisible to
// every diff, an agent's new files are usually untracked, and `--branch`
// guarantees at least one line of output so a clean worktree reads as
// "nothing to review" rather than as a window that failed to run anything.
//
// No --color and no `| less`: output goes straight to a tty, so git colours
// and pages it with the user's own pager. It ends in a shell so the window
// survives the pager.
func ReviewScript(commit string) string {
	return "git diff " + shellQuote(commit) + ";" +
		" git status --short --branch;" +
		` exec "${SHELL:-/bin/sh}"`
}
