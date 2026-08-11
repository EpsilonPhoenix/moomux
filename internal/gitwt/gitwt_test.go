package gitwt

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type fakeRunner struct {
	calls [][]string
	out   string
	// errAt, keyed by call index (0-based), makes that call fail instead of
	// returning out — used to simulate e.g. "no upstream configured".
	errAt map[int]error
}

func (f *fakeRunner) Run(dir string, args ...string) (string, error) {
	c := append([]string{"@" + dir}, args...)
	idx := len(f.calls)
	f.calls = append(f.calls, c)
	if err, ok := f.errAt[idx]; ok {
		return "", err
	}
	return f.out, nil
}

func TestFetch(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.Fetch("/repo", "main"); err != nil {
		t.Fatal(err)
	}
	want := []string{"@/repo", "fetch", "origin", "main"}
	if !reflect.DeepEqual(fr.calls[0], want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}

func TestAddWorktree(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.AddWorktree("/repo", "/wt/foo", "user/foo", "main"); err != nil {
		t.Fatal(err)
	}
	want := []string{"@/repo", "worktree", "add", "/wt/foo", "-b", "user/foo", "origin/main"}
	if !reflect.DeepEqual(fr.calls[len(fr.calls)-1], want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}

func TestAddWorktreeExisting(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.AddWorktreeExisting("/repo", "/wt/foo", "user/foo"); err != nil {
		t.Fatal(err)
	}
	want := []string{"@/repo", "worktree", "add", "/wt/foo", "user/foo"}
	if !reflect.DeepEqual(fr.calls[len(fr.calls)-1], want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}

func TestRemoveWorktree(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.RemoveWorktree("/repo", "/wt/foo"); err != nil {
		t.Fatal(err)
	}
	want := []string{"@/repo", "worktree", "remove", "/wt/foo", "--force"}
	if !reflect.DeepEqual(fr.calls[0], want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}

// TestRemoveWorktreeStatPermissionErrorSurfaces simulates os.Stat failing
// for a reason other than "gone" (e.g. permission denied on a parent dir).
// RemoveWorktree must not treat that as "already removed" and report
// success — it doesn't actually know whether the worktree is there.
func TestRemoveWorktreeStatPermissionErrorSurfaces(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission bits")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(locked, "worktree")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	c := &Client{Runner: &erringRunner{}}
	if err := c.RemoveWorktree("/repo", target); err == nil {
		t.Fatal("expected an error, not silent success, for an unresolvable stat")
	}
}

// erringRunner fails every git call, simulating e.g. "worktree remove"
// refusing to touch a main working tree.
type erringRunner struct{ calls int }

func (f *erringRunner) Run(dir string, args ...string) (string, error) {
	f.calls++
	return "", errors.New("git refused")
}

func TestRemoveWorktreeGitFailureKeepsDirectory(t *testing.T) {
	dir := t.TempDir() // stands in for a real repo the user still needs
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Client{Runner: &erringRunner{}}
	if err := c.RemoveWorktree("/repo", dir); err == nil {
		t.Fatal("expected the git error to surface")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory was deleted despite git failure: %v", err)
	}
}

// TestRemoveWorktreeMissingGitSelfCleans covers a worktree directory that
// was never linked to git at all (e.g. worktree creation died before `git
// worktree add` ran, or something manually deleted the .git file). git
// itself has no registration to lose track of, so RemoveWorktree must clean
// it up rather than surface git's "not a working tree" error forever.
func TestRemoveWorktreeMissingGitSelfCleans(t *testing.T) {
	dir := t.TempDir()
	c := &Client{Runner: &erringRunner{}}
	if err := c.RemoveWorktree("/repo", dir); err != nil {
		t.Fatalf("expected self-cleanup, got error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected directory to be removed, stat err = %v", err)
	}
}

// TestExecRunnerRespectsRunTimeout replaces "git" on PATH with a fake that
// sleeps far longer than runTimeout. Without exec.CommandContext bounding
// the subprocess, execRunner.Run would block for the full sleep instead of
// giving up once the timeout elapses (e.g. a fetch against a dead remote
// hanging the whole app).
func TestExecRunnerRespectsRunTimeout(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	fakeDir := t.TempDir()
	fake := filepath.Join(fakeDir, "git")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	old := runTimeout
	runTimeout = 50 * time.Millisecond
	t.Cleanup(func() { runTimeout = old })

	start := time.Now()
	if _, err := (execRunner{}).Run(t.TempDir(), "status"); err == nil {
		t.Fatal("expected error once runTimeout elapsed")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("Run took %v, want to return shortly after runTimeout", elapsed)
	}
}

func TestWorktreeForBranch(t *testing.T) {
	fr := &fakeRunner{
		out: "worktree /wt/foo\nbranch refs/heads/user/foo\n\nworktree /wt/bar\nbranch refs/heads/user/bar\n",
	}
	c := &Client{Runner: fr}
	got, err := c.WorktreeForBranch("/repo", "user/bar")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/wt/bar" {
		t.Fatalf("got %q, want /wt/bar", got)
	}
	got, err = c.WorktreeForBranch("/repo", "user/missing")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestIsWorktreeClean(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	clean, err := c.IsWorktreeClean("/wt/foo")
	if err != nil || !clean {
		t.Fatalf("clean=%v err=%v, want clean", clean, err)
	}

	fr.out = " M some/file.go\n"
	clean, err = c.IsWorktreeClean("/wt/foo")
	if err != nil || clean {
		t.Fatalf("clean=%v err=%v, want dirty", clean, err)
	}
}

func TestHasUnpushedCommits(t *testing.T) {
	fr := &fakeRunner{out: "0\n"}
	c := &Client{Runner: fr}
	unpushed, err := c.HasUnpushedCommits("/wt/foo")
	if err != nil || unpushed {
		t.Fatalf("unpushed=%v err=%v, want false (up to date with upstream)", unpushed, err)
	}

	fr = &fakeRunner{out: "2\n"}
	c = &Client{Runner: fr}
	unpushed, err = c.HasUnpushedCommits("/wt/foo")
	if err != nil || !unpushed {
		t.Fatalf("unpushed=%v err=%v, want true (ahead of upstream)", unpushed, err)
	}

	// call 0: @{u} fails (no upstream). call 1: rev-parse HEAD succeeds.
	// call 2: config lookup also fails, meaning upstream was never configured.
	fr = &fakeRunner{errAt: map[int]error{
		0: errors.New("no upstream configured"),
		2: errors.New("key not found"),
	}}
	c = &Client{Runner: fr}
	unpushed, err = c.HasUnpushedCommits("/wt/foo")
	if err != nil || !unpushed {
		t.Fatalf("unpushed=%v err=%v, want true (no upstream)", unpushed, err)
	}

	// call 0: @{u} fails, but call 2's config lookup succeeds — the upstream
	// was configured and then pruned (e.g. remote branch deleted post-merge),
	// which must not read the same as "never pushed".
	fr = &fakeRunner{errAt: map[int]error{0: errors.New("upstream gone")}}
	c = &Client{Runner: fr}
	unpushed, err = c.HasUnpushedCommits("/wt/foo")
	if err != nil || unpushed {
		t.Fatalf("unpushed=%v err=%v, want false (upstream gone, nothing left to compare)", unpushed, err)
	}
}

func TestDeleteBranch(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.DeleteBranch("/repo", "user/foo"); err != nil {
		t.Fatal(err)
	}
	want := []string{"@/repo", "branch", "-D", "user/foo"}
	if !reflect.DeepEqual(fr.calls[0], want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}
