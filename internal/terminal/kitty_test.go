package terminal

import (
	"errors"
	"strings"
	"testing"
)

type fakeKittyRunner struct {
	calls [][]string
	outs  []string // out returned per call, in order
	errs  []error  // err returned per call, in order
}

func (f *fakeKittyRunner) Run(args ...string) (string, error) {
	n := len(f.calls)
	f.calls = append(f.calls, args)
	var out string
	var err error
	if n < len(f.outs) {
		out = f.outs[n]
	}
	if n < len(f.errs) {
		err = f.errs[n]
	}
	return out, err
}

const lsOneFocusedTab = `[{"tabs":[{"id":7,"is_focused":true},{"id":8,"is_focused":false}]}]`

func TestKittyOpenTabFocusesExistingTab(t *testing.T) {
	fr := &fakeKittyRunner{}
	c := &kittyClient{runner: fr}
	newID, hint, err := c.OpenTab("7", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "7" {
		t.Fatalf("want unchanged tab id, got %q", newID)
	}
	if hint != "" {
		t.Fatalf("want no hint, got %q", hint)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("want exactly one remote call, got %d", len(fr.calls))
	}
	assertContains(t, fr.calls[0], "focus-tab")
	assertContains(t, fr.calls[0], "id:7")
}

func TestKittyOpenTabCreatesWhenTabGone(t *testing.T) {
	fallback := &fakeExec{}
	fr := &fakeKittyRunner{
		errs: []error{errors.New("no matching tabs")},
		outs: []string{"", "", lsOneFocusedTab},
	}
	c := &kittyClient{runner: fr, fallback: &windowOpener{binary: "kitty", args: kittyArgs, exec: fallback.Command}}
	newID, hint, err := c.OpenTab("7", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "7" {
		t.Fatalf("want new tab id from ls, got %q", newID)
	}
	if hint != "" {
		t.Fatalf("want no hint, got %q", hint)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("want focus-tab, launch, ls calls, got %d: %v", len(fr.calls), fr.calls)
	}
	assertContains(t, fr.calls[1], "launch")
	assertContains(t, fr.calls[1], "--type=tab")
	assertContains(t, fr.calls[1], "=moomux-foo")
	assertContains(t, fr.calls[2], "ls")
	if fallback.binary != "" {
		t.Fatalf("fallback should not run when launch succeeds, ran %q", fallback.binary)
	}
}

func TestKittyOpenTabCreatesWhenNoTabIDGiven(t *testing.T) {
	fr := &fakeKittyRunner{outs: []string{"", lsOneFocusedTab}}
	c := &kittyClient{runner: fr}
	newID, _, err := c.OpenTab("", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "7" {
		t.Fatalf("want new tab id, got %q", newID)
	}
	if len(fr.calls) != 2 {
		t.Fatalf("want launch+ls, no focus-tab attempt, got %d: %v", len(fr.calls), fr.calls)
	}
}

func TestKittyOpenTabFallsBackWhenLaunchFails(t *testing.T) {
	fallback := &fakeExec{}
	fr := &fakeKittyRunner{errs: []error{errors.New("remote control disabled")}}
	c := &kittyClient{runner: fr, fallback: &windowOpener{binary: "kitty", args: kittyArgs, exec: fallback.Command}}
	newID, _, err := c.OpenTab("", "moomux-foo", "bar")
	if err != nil {
		t.Fatal(err)
	}
	if newID != "" {
		t.Fatalf("want no tab id when falling back, got %q", newID)
	}
	if fallback.binary != "kitty" {
		t.Fatalf("expected fallback to kitty, got %q", fallback.binary)
	}
}

func TestKittyOpenTabTitleAndSessionEscaping(t *testing.T) {
	fr := &fakeKittyRunner{outs: []string{"", lsOneFocusedTab}}
	c := &kittyClient{runner: fr}
	if _, _, err := c.OpenTab("", "moomux-foo", "feat/bar"); err != nil {
		t.Fatal(err)
	}
	assertContains(t, fr.calls[0], "--tab-title=feat/bar")
	if !strings.Contains(strings.Join(fr.calls[0], " "), "=moomux-foo") {
		t.Fatalf("missing exact-match session target: %v", fr.calls[0])
	}
}
