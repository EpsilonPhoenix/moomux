package terminal

import (
	"errors"
	"strings"
	"testing"
)

type fakeWeztermRunner struct {
	calls [][]string
	outs  []string // one entry per call, in order
	errs  []error  // one entry per call, in order
}

func (f *fakeWeztermRunner) run(args ...string) (string, error) {
	i := len(f.calls)
	f.calls = append(f.calls, args)
	var out string
	var err error
	if i < len(f.outs) {
		out = f.outs[i]
	}
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return out, err
}

func TestWeztermOpenTabActivatesExistingPane(t *testing.T) {
	fr := &fakeWeztermRunner{}
	c := &weztermClient{run: fr.run}
	newID, hint, err := c.OpenTab("42", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "42" {
		t.Fatalf("want unchanged pane id, got %q", newID)
	}
	if hint != "" {
		t.Fatalf("want no hint, got %q", hint)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("want exactly one wezterm call, got %d", len(fr.calls))
	}
	if !strings.Contains(strings.Join(fr.calls[0], " "), "activate-pane --pane-id 42") {
		t.Fatalf("missing activate-pane call: %v", fr.calls[0])
	}
}

func TestWeztermOpenTabSpawnsWhenPaneGone(t *testing.T) {
	fr := &fakeWeztermRunner{
		errs: []error{errors.New("no such pane")},
		outs: []string{"", "7"},
	}
	c := &weztermClient{run: fr.run}
	newID, hint, err := c.OpenTab("42", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "7" {
		t.Fatalf("want new pane id, got %q", newID)
	}
	if hint != "" {
		t.Fatalf("want no hint, got %q", hint)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("want activate attempt followed by spawn, got %d calls", len(fr.calls))
	}
	second := strings.Join(fr.calls[1], " ")
	if !strings.Contains(second, "cli spawn") || !strings.Contains(second, "tmux attach -t =moomux-foo") {
		t.Fatalf("second call should spawn+attach a new pane: %s", second)
	}
}

func TestWeztermOpenTabSpawnsWhenNoTabIDGiven(t *testing.T) {
	fr := &fakeWeztermRunner{outs: []string{"7"}}
	c := &weztermClient{run: fr.run}
	newID, _, err := c.OpenTab("", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "7" {
		t.Fatalf("want new pane id, got %q", newID)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("want a single spawn call, no activate attempt, got %d", len(fr.calls))
	}
}

func TestWeztermOpenTabFallsBackWhenSpawnFails(t *testing.T) {
	fr := &fakeWeztermRunner{errs: []error{errors.New("mux unreachable")}}
	fb := &fakeOpener{hint: "fallback hint"}
	c := &weztermClient{run: fr.run, fallback: fb}
	newID, hint, err := c.OpenTab("", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "" {
		t.Fatalf("want no new tab id on fallback, got %q", newID)
	}
	if hint != "fallback hint" {
		t.Fatalf("want fallback hint, got %q", hint)
	}
	if fb.tmuxSession != "moomux-foo" || fb.title != "bar" {
		t.Fatalf("fallback called with wrong args: %+v", fb)
	}
}

type fakeOpener struct {
	hint        string
	err         error
	tmuxSession string
	title       string
}

func (f *fakeOpener) OpenSession(tmuxSession, title string) (string, error) {
	f.tmuxSession = tmuxSession
	f.title = title
	return f.hint, f.err
}
