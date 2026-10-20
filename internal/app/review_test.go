package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/tmux"
)

// The review window diffs exactly the commit Review resolved: no shell
// fallbacks keyed on a diff's exit status, which a pager quit early (141)
// used to trip into showing `git diff HEAD` as the review. The commit is
// quoted anyway; the script goes through a shell.
func TestReviewScriptDiffsTheResolvedCommit(t *testing.T) {
	got := ReviewScript("3f9c")
	if !strings.HasPrefix(got, "git diff 3f9c; git status --short --branch;") || strings.Contains(got, "||") {
		t.Fatalf("script:\n%s", got)
	}
	if got := ReviewScript("a'b; echo pwned"); !strings.HasPrefix(got, `git diff 'a'\''b; echo pwned';`) {
		t.Fatalf("unquoted:\n%s", got)
	}
}

func newReviewApp(t *testing.T, fr *fakeTmuxRunner, sessions ...session.Session) *App {
	t.Helper()
	a := &App{
		Cfg: &config.Config{Projects: map[string]config.Project{
			"proj":  {Repo: "/repo", BaseBranch: "trunk"},
			"notes": {Kind: "plain", Repo: "/notes"},
		}},
		Store: &session.Store{Path: filepath.Join(t.TempDir(), "sessions.json")},
		Tmux:  &tmux.Client{Runner: fr},
	}
	if err := a.Store.Load(); err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if err := a.Store.Put(s); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// Capture's whole job on the wire is the id-to-tmux-name mapping: a front
// end asks about session ids and must never need to know what moomux called
// the tmux session. Unknown ids drop out rather than erroring.
func TestCaptureKeysBySessionIDAndSkipsUnknownIDs(t *testing.T) {
	fr := &fakeTmuxRunner{out: map[string]string{
		"list-sessions -F #{session_name}": "moomux-a\nmoomux-b\n",
	}}
	a := newReviewApp(t, fr,
		session.Session{ID: "proj:a", Project: "proj", TmuxSession: "moomux-a"},
		session.Session{ID: "proj:b", Project: "proj", TmuxSession: "moomux-b"},
	)
	got := a.Capture([]string{"proj:a", "proj:b", "proj:gone"})
	if _, ok := got["proj:gone"]; ok {
		t.Fatalf("unknown id should be absent, got %v", got)
	}
	// One invocation for the batch, then one fallback each (the fake runner
	// returns empty output, so nothing parses out of the batch). The
	// per-session window lookups ahead of it are cached, one per session.
	var batch []string
	for _, call := range fr.calls {
		if len(call) >= 8 {
			batch = call
			break
		}
	}
	if batch == nil {
		t.Fatalf("expected a batched capture, got %v", fr.calls)
	}
	joined := strings.Join(batch, " ")
	if !strings.Contains(joined, "=moomux-a:^") || !strings.Contains(joined, "=moomux-b:^") {
		t.Fatalf("batch did not target both tmux sessions: %s", joined)
	}
}

// The session's own base is what its branch was cut from; the project's is
// the fallback for a resumed branch that never recorded one. Either way the
// window diffs the commit Diff would: their merge base.
func TestReviewPrefersTheSessionBaseThenTheProjects(t *testing.T) {
	wt := diffBaseRepo(t)
	fr := &fakeTmuxRunner{}
	a := newReviewApp(t, fr,
		session.Session{ID: "proj:a", Project: "proj", TmuxSession: "moomux-a", WorktreePath: wt, BaseBranch: "trunk"},
		session.Session{ID: "proj:b", Project: "proj", TmuxSession: "moomux-b", WorktreePath: wt},
	)
	a.Cfg.Projects["proj"] = config.Project{Repo: wt, BaseBranch: "main"}
	sha := func(ref string) string {
		out, err := runGit(wt, "rev-parse", ref)
		if err != nil {
			t.Fatal(err)
		}
		return "git diff " + strings.TrimSpace(out) + ";"
	}
	for id, want := range map[string]string{"proj:a": sha("trunk"), "proj:b": sha("main")} {
		fr.calls = nil
		if _, err := a.Review(id); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var all []string
		for _, c := range fr.calls {
			all = append(all, strings.Join(c, " "))
		}
		if !strings.Contains(strings.Join(all, "\n"), want) {
			t.Fatalf("%s: want %q in %v", id, want, all)
		}
	}
}

// respawn-window and new-window both fail on a parked session with nothing
// but an exit status, and reviving it would relaunch the agent — an
// expensive surprise from "show me the diff". So it has to be named.
func TestReviewOnAParkedSessionSaysSoAndDoesNotReviveIt(t *testing.T) {
	fr := &fakeTmuxRunner{failOn: map[string]bool{"has-session -t =moomux-a": true}}
	a := newReviewApp(t, fr,
		session.Session{ID: "proj:a", Project: "proj", Name: "a", TmuxSession: "moomux-a", WorktreePath: "/wt/a"})
	_, err := a.Review("proj:a")
	if err == nil || !strings.Contains(err.Error(), "parked") {
		t.Fatalf("err = %v, want it to name the session as parked", err)
	}
	for _, c := range fr.calls {
		if c[0] == "new-session" || c[0] == "respawn-window" || c[0] == "new-window" {
			t.Fatalf("review must not revive or open anything: %v", fr.calls)
		}
	}
}

func TestReviewRefusesAPlainProject(t *testing.T) {
	fr := &fakeTmuxRunner{}
	a := newReviewApp(t, fr, session.Session{ID: "notes:a", Project: "notes", TmuxSession: "moomux-n"})
	if _, err := a.Review("notes:a"); err == nil {
		t.Fatal("a plain project has nothing to diff")
	}
	if _, err := a.Review("proj:nope"); err == nil {
		t.Fatal("unknown session should error")
	}
}

// A session whose project has been removed from the config keeps its store
// entry. Without the lookup's ok, the zero Project is neither plain nor
// based on anything, so review would open a diff against "main" in a
// worktree the core no longer manages.
func TestReviewRefusesASessionWhoseProjectIsGone(t *testing.T) {
	fr := &fakeTmuxRunner{}
	a := newReviewApp(t, fr, session.Session{ID: "old:a", Project: "old", TmuxSession: "moomux-a", WorktreePath: "/wt/a"})
	_, err := a.Review("old:a")
	if err == nil || !strings.Contains(err.Error(), "old") {
		t.Fatalf("err = %v, want it to name the missing project", err)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("nothing should have been run: %v", fr.calls)
	}
}

// Parking keeps the session's tmux name, and a dead name in the batch
// aborts the chained sequence — every session after it would then cost a
// fork of its own, every poll tick.
func TestCaptureSkipsParkedSessions(t *testing.T) {
	fr := &fakeTmuxRunner{out: map[string]string{
		"list-sessions -F #{session_name}": "moomux-a\n",
	}}
	a := newReviewApp(t, fr,
		session.Session{ID: "proj:a", Project: "proj", TmuxSession: "moomux-a"},
		session.Session{ID: "proj:parked", Project: "proj", TmuxSession: "moomux-parked"},
	)
	a.Capture([]string{"proj:a", "proj:parked"})
	for _, call := range fr.calls {
		if strings.Contains(strings.Join(call, " "), "moomux-parked") {
			t.Fatalf("parked session reached tmux: %v", call)
		}
	}
}

// diffBaseRepo is a worktree on feat, cut from trunk, which is itself one
// commit (trunk.txt) ahead of main — so whether trunk.txt is in the diff
// says which base it was taken against.
func diffBaseRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	id := []string{"-c", "user.name=moomux", "-c", "user.email=moomux@localhost"}
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, append(id, "commit", "--allow-empty", "-m", "init")...)
	mustGit(t, dir, "checkout", "-b", "trunk")
	if err := os.WriteFile(filepath.Join(dir, "trunk.txt"), []byte("trunk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "trunk.txt")
	mustGit(t, dir, append(id, "commit", "-m", "trunk")...)
	mustGit(t, dir, "checkout", "-b", "feat")
	if err := os.WriteFile(filepath.Join(dir, "feat.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Diff resolves its base exactly as Review does: the session's, then the
// branch the checkout tracks, then the project's, then main.
func TestDiffPrefersTheSessionBaseThenTheProjectsThenMain(t *testing.T) {
	wt := diffBaseRepo(t)
	for _, tc := range []struct {
		name, sessionBase, projectBase, wantBase string
		wantTrunkFile                            bool
	}{
		{"session base wins", "trunk", "main", "trunk", false},
		{"project base is the fallback", "", "trunk", "trunk", false},
		{"a tracked branch beats the project's", "", "main", "trunk", false},
		{"main is the last resort", "", "", "main", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "a tracked branch beats the project's" {
				mustGit(t, wt, "branch", "--set-upstream-to=trunk", "feat")
				defer mustGit(t, wt, "branch", "--unset-upstream", "feat")
			}
			a := newReviewApp(t, &fakeTmuxRunner{},
				session.Session{ID: "p:a", Project: "p", WorktreePath: wt, BaseBranch: tc.sessionBase})
			a.Cfg.Projects["p"] = config.Project{Repo: wt, BaseBranch: tc.projectBase}
			p, ok, err := a.Diff("p:a")
			if err != nil || !ok {
				t.Fatalf("Diff: ok %v, err %v", ok, err)
			}
			if p.Base != tc.wantBase {
				t.Fatalf("Base = %q, want %q", p.Base, tc.wantBase)
			}
			if got := strings.Contains(p.Text, "b/trunk.txt"); got != tc.wantTrunkFile {
				t.Fatalf("trunk.txt in the diff = %v, want %v:\n%s", got, tc.wantTrunkFile, p.Text)
			}
			if !strings.Contains(p.Text, "+feat\n") {
				t.Fatalf("the untracked feat.txt is missing:\n%s", p.Text)
			}
		})
	}
}

// Unlike Review, Diff has no window to open, so a parked session — tmux
// long gone — diffs exactly like a live one, and tmux is never asked.
func TestDiffOnAParkedSessionNeverTouchesTmux(t *testing.T) {
	wt := diffBaseRepo(t)
	fr := &fakeTmuxRunner{failOn: map[string]bool{"has-session -t =moomux-a": true}}
	a := newReviewApp(t, fr,
		session.Session{ID: "proj:a", Project: "proj", Name: "a", TmuxSession: "moomux-a", WorktreePath: wt})
	p, ok, err := a.Diff("proj:a")
	if err != nil || !ok || !strings.Contains(p.Text, "+feat\n") {
		t.Fatalf("Diff = ok %v, err %v:\n%s", ok, err, p.Text)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("Diff must not touch tmux, called %v", fr.calls)
	}
}

// Not a git repo is ok=false, not an error, like ChangeSummary; an
// unknown session is still an error.
func TestDiffOnANonRepoIsNotOK(t *testing.T) {
	a := newReviewApp(t, &fakeTmuxRunner{},
		session.Session{ID: "notes:a", Project: "notes", WorktreePath: t.TempDir()},
		session.Session{ID: "proj:gone", Project: "proj", WorktreePath: filepath.Join(t.TempDir(), "missing")},
	)
	for _, id := range []string{"notes:a", "proj:gone"} {
		if _, ok, err := a.Diff(id); ok || err != nil {
			t.Fatalf("%s: ok %v, err %v, want not ok and no error", id, ok, err)
		}
	}
	if _, _, err := a.Diff("proj:nope"); err == nil {
		t.Fatal("unknown session should error")
	}
}

// ReadBaseFile reads at the base Diff resolves — the session's, then the
// project's, then main — and, pinned to a Diff's Commit, at that commit
// whatever the base branch has done since. A parked session is read
// without asking tmux anything.
func TestReadBaseFileReadsDiffsBase(t *testing.T) {
	wt := diffBaseRepo(t)
	fr := &fakeTmuxRunner{failOn: map[string]bool{"has-session -t =moomux-a": true}}
	a := newReviewApp(t, fr,
		session.Session{ID: "p:a", Project: "p", TmuxSession: "moomux-a", WorktreePath: wt, BaseBranch: "trunk"},
		session.Session{ID: "p:b", Project: "p", WorktreePath: wt})
	a.Cfg.Projects["p"] = config.Project{Repo: wt}

	// Against trunk, trunk.txt is in the base; against main it is new.
	p, _, err := a.Diff("p:a")
	if err != nil {
		t.Fatal(err)
	}
	data, commit, ok, err := a.ReadBaseFile("p:a", "trunk.txt", "")
	if err != nil || !ok || string(data) != "trunk\n" || commit != p.Commit {
		t.Fatalf("ReadBaseFile(trunk) = %q %q %v %v, want trunk.txt at %s", data, commit, ok, err, p.Commit)
	}
	if _, _, ok, err := a.ReadBaseFile("p:b", "trunk.txt", ""); ok || err != nil {
		t.Fatalf("ReadBaseFile(main, trunk.txt): ok %v, err %v; want not in the base", ok, err)
	}
	if _, _, ok, err := a.ReadBaseFile("p:a", "feat.txt", ""); ok || err != nil {
		t.Fatalf("ReadBaseFile(untracked feat.txt): ok %v, err %v; want not in the base", ok, err)
	}

	// trunk moves on (here, by taking feat's work); the pinned read
	// doesn't follow it, a fresh one does.
	if err := os.WriteFile(filepath.Join(wt, "trunk.txt"), []byte("moved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := []string{"-c", "user.name=moomux", "-c", "user.email=moomux@localhost"}
	mustGit(t, wt, append(id, "commit", "-am", "edit")...)
	mustGit(t, wt, "branch", "-f", "trunk", "HEAD")
	if data, _, _, err := a.ReadBaseFile("p:a", "trunk.txt", p.Commit); err != nil || string(data) != "trunk\n" {
		t.Fatalf("pinned ReadBaseFile = %q, err %v; want the commit Diff answered", data, err)
	}
	if data, commit, _, err := a.ReadBaseFile("p:a", "trunk.txt", ""); err != nil || string(data) != "moved\n" || commit == p.Commit {
		t.Fatalf("fresh ReadBaseFile = %q at %s, err %v; want the moved base", data, commit, err)
	}
	if len(fr.calls) != 0 {
		t.Fatalf("ReadBaseFile must not touch tmux, called %v", fr.calls)
	}
}

func TestReadBaseFileErrors(t *testing.T) {
	a := newReviewApp(t, &fakeTmuxRunner{},
		session.Session{ID: "notes:a", Project: "notes", WorktreePath: t.TempDir()},
		session.Session{ID: "proj:gone", Project: "proj", Name: "gone", WorktreePath: filepath.Join(t.TempDir(), "missing")})
	// Not ok=false, as Diff answers: here that would read as "new file".
	for _, commit := range []string{"", strings.Repeat("a", 40)} {
		if _, _, ok, err := a.ReadBaseFile("proj:gone", "x.md", commit); ok || err == nil || !strings.Contains(err.Error(), "gone has no git worktree") {
			t.Fatalf("missing worktree, commit %q: ok %v, err %v", commit, ok, err)
		}
	}
	if _, _, _, err := a.ReadBaseFile("notes:a", "x.md", ""); err == nil {
		t.Fatal("a plain project has no base; want an error")
	}
	if _, _, _, err := a.ReadBaseFile("proj:nope", "x.md", ""); err == nil {
		t.Fatal("unknown session should error")
	}
}

// The point of SelectWindow is to not type keys into the pane, so it must
// be a next/previous-window aimed at the exact tmux session.
func TestSelectWindowTargetsTheExactSession(t *testing.T) {
	fr := &fakeTmuxRunner{}
	a := newReviewApp(t, fr, session.Session{ID: "proj:a", Project: "proj", TmuxSession: "moomux-a"})
	for delta, want := range map[int]string{1: "next-window -t =moomux-a:", -1: "previous-window -t =moomux-a:"} {
		fr.calls = nil
		if err := a.SelectWindow("proj:a", delta); err != nil {
			t.Fatalf("delta %d: %v", delta, err)
		}
		if got := strings.Join(fr.calls[len(fr.calls)-1], " "); got != want {
			t.Fatalf("delta %d: ran %q, want %q", delta, got, want)
		}
	}
}

// Like Review: a parked session is named, not revived, and an unknown id or
// a delta other than ±1 never reaches tmux.
func TestSelectWindowRefusesParkedUnknownAndBadDelta(t *testing.T) {
	fr := &fakeTmuxRunner{failOn: map[string]bool{"has-session -t =moomux-a": true}}
	a := newReviewApp(t, fr, session.Session{ID: "proj:a", Project: "proj", Name: "a", TmuxSession: "moomux-a"})
	if err := a.SelectWindow("proj:a", 1); err == nil || !strings.Contains(err.Error(), "parked") {
		t.Fatalf("err = %v, want it to name the session as parked", err)
	}
	if err := a.SelectWindow("proj:nope", 1); err == nil {
		t.Fatal("unknown session should error")
	}
	if err := a.SelectWindow("proj:a", 0); err == nil {
		t.Fatal("delta 0 should error")
	}
	for _, c := range fr.calls {
		if c[0] != "has-session" {
			t.Fatalf("only the liveness check may run: %v", fr.calls)
		}
	}
}
