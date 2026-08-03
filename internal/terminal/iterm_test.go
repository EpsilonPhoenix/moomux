package terminal

import (
	"strings"
	"testing"
)

type fakeRunner struct {
	script  string   // last script run
	scripts []string // every script run, in order
	out     string   // out to return on the first call; outs takes over from the second
	outs    []string
	err     error
}

func (f *fakeRunner) Run(script string) (string, error) {
	f.script = script
	f.scripts = append(f.scripts, script)
	if n := len(f.scripts); n > 1 && len(f.outs) >= n-1 {
		return f.outs[n-2], f.err
	}
	return f.out, f.err
}

func TestITermOpenSessionAttachesAndSetsTitle(t *testing.T) {
	fr := &fakeRunner{}
	c := &itermClient{runner: fr}
	if _, err := c.OpenSession("moomux-foo", "feat/bar"); err != nil {
		t.Fatal(err)
	}
	// The target must be single-quoted: it is typed into an interactive
	// shell, and zsh's EQUALS expansion turns a bare leading "=" into a
	// command-path lookup ("zsh: moomux-foo not found").
	if !strings.Contains(fr.script, `tmux attach -t '=moomux-foo'`) {
		t.Fatalf("missing quoted attach: %s", fr.script)
	}
	if !strings.Contains(fr.script, "iTerm2") {
		t.Fatalf("missing iTerm2 target: %s", fr.script)
	}
	if !strings.Contains(fr.script, `set name to "feat/bar"`) {
		t.Fatalf("missing tab title: %s", fr.script)
	}
}

func TestITermOpenSessionEscapesTmuxSession(t *testing.T) {
	// tmuxSession is currently always moomux-<name>-<hash> (never
	// attacker-controlled), but the AppleScript write-text argument gets
	// the same escaping as the title for defense-in-depth.
	fr := &fakeRunner{}
	c := &itermClient{runner: fr}
	if _, err := c.OpenSession(`moomux-foo"; do shell script "rm`, "bar"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fr.script, `tmux attach -t '=moomux-foo\"; do shell script \"rm'`) {
		t.Fatalf("tmux session not escaped: %s", fr.script)
	}
}

func TestITermOpenSessionOmitsTitleWhenEmpty(t *testing.T) {
	fr := &fakeRunner{}
	c := &itermClient{runner: fr}
	if _, err := c.OpenSession("moomux-foo", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fr.script, "set name to") {
		t.Fatalf("should not set name when title empty: %s", fr.script)
	}
}

func TestITermOpenTabSelectsExistingTab(t *testing.T) {
	fr := &fakeRunner{out: "found"}
	c := &itermClient{runner: fr}
	newID, hint, err := c.OpenTab("tab-1", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "tab-1" {
		t.Fatalf("want unchanged tab id, got %q", newID)
	}
	if hint != "" {
		t.Fatalf("want no hint, got %q", hint)
	}
	if len(fr.scripts) != 1 {
		t.Fatalf("want exactly one applescript call, got %d", len(fr.scripts))
	}
	if !strings.Contains(fr.script, `id of sess is "tab-1"`) {
		t.Fatalf("missing tab id lookup: %s", fr.script)
	}
	if strings.Contains(fr.script, "tmux attach") {
		t.Fatalf("should not re-attach when tab still exists: %s", fr.script)
	}
}

func TestITermOpenTabFallsBackWhenTabGone(t *testing.T) {
	fr := &fakeRunner{out: "notfound", outs: []string{"new-tab-id"}}
	c := &itermClient{runner: fr}
	newID, _, err := c.OpenTab("tab-1", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "new-tab-id" {
		t.Fatalf("want new tab id, got %q", newID)
	}
	if len(fr.scripts) != 2 {
		t.Fatalf("want a select attempt followed by a create, got %d calls", len(fr.scripts))
	}
	if !strings.Contains(fr.scripts[1], `tmux attach -t '=moomux-foo'`) {
		t.Fatalf("second call should create+attach a new tab: %s", fr.scripts[1])
	}
}

func TestITermOpenTabCreatesWhenNoTabIDGiven(t *testing.T) {
	fr := &fakeRunner{out: "new-tab-id"}
	c := &itermClient{runner: fr}
	newID, _, err := c.OpenTab("", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "new-tab-id" {
		t.Fatalf("want new tab id, got %q", newID)
	}
	if len(fr.scripts) != 1 {
		t.Fatalf("want a single create call, no select attempt, got %d", len(fr.scripts))
	}
}

func TestITermEscapesAppleScript(t *testing.T) {
	fr := &fakeRunner{}
	c := &itermClient{runner: fr}
	if _, err := c.OpenSession("moomux-foo", `branch"with\special`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fr.script, `branch\"with\\special`) {
		t.Fatalf("backslash/quote not escaped: %s", fr.script)
	}
}
