package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	calls  [][]string
	out    map[string]string
	failOn map[string]bool
}

func (f *fakeRunner) Run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, append([]string(nil), args...))
	if f.failOn[key] {
		return "", exitErr{code: 1}
	}
	return f.out[key], nil
}

type exitErr struct{ code int }

func (e exitErr) Error() string { return "exit" }
func (e exitErr) ExitCode() int { return e.code }

// TestExecRunnerRespectsRunTimeout replaces "tmux" on PATH with a fake that
// sleeps far longer than runTimeout. Without exec.CommandContext bounding
// the subprocess, execRunner.Run would block for the full sleep instead of
// giving up once the timeout elapses (e.g. an unresponsive tmux server
// hanging the whole app).
func TestExecRunnerRespectsRunTimeout(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	fakeDir := t.TempDir()
	fake := filepath.Join(fakeDir, "tmux")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	old := runTimeout
	runTimeout = 50 * time.Millisecond
	t.Cleanup(func() { runTimeout = old })

	start := time.Now()
	if _, err := (execRunner{}).Run("list-sessions"); err == nil {
		t.Fatal("expected error once runTimeout elapsed")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Run took %v, want to return shortly after runTimeout", elapsed)
	}
}

func TestNewSession(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{"list-panes -t =moomux-foo: -F #{pane_id}": "%3\n"}}
	c := &Client{Runner: fr}
	if err := c.NewSession("moomux-foo", "/tmp/wt", "claude", "foo"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"new-session", "-d", "-s", "moomux-foo", "-c", "/tmp/wt", "-n", "foo"},
		{"set-window-option", "-t", "=moomux-foo:", "automatic-rename", "off"},
		{"set-option", "-t", "=moomux-foo:", "set-titles", "on"},
		{"set-option", "-t", "=moomux-foo:", "set-titles-string", "#{window_name}"},
		{"set-option", "-t", "=moomux-foo:", "mouse", "on"},
		{"list-panes", "-t", "=moomux-foo:", "-F", "#{pane_id}"},
		{"split-window", "-h", "-t", "=moomux-foo:", "-c", "/tmp/wt", "-l", "33%"},
		{"select-pane", "-t", "%3"},
		{"send-keys", "-t", "%3", "claude", "Enter"},
	}
	if !reflect.DeepEqual(fr.calls, want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}

func TestNewSessionNoWindowName(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{"list-panes -t =moomux-foo: -F #{pane_id}": "%3\n"}}
	c := &Client{Runner: fr}
	if err := c.NewSession("moomux-foo", "/tmp/wt", "claude", ""); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"new-session", "-d", "-s", "moomux-foo", "-c", "/tmp/wt"},
		{"set-option", "-t", "=moomux-foo:", "mouse", "on"},
		{"list-panes", "-t", "=moomux-foo:", "-F", "#{pane_id}"},
		{"split-window", "-h", "-t", "=moomux-foo:", "-c", "/tmp/wt", "-l", "33%"},
		{"select-pane", "-t", "%3"},
		{"send-keys", "-t", "%3", "claude", "Enter"},
	}
	if !reflect.DeepEqual(fr.calls, want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}

func TestNewSessionNoCmd(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{"list-panes -t =moomux-foo: -F #{pane_id}": "%3\n"}}
	c := &Client{Runner: fr}
	if err := c.NewSession("moomux-foo", "/tmp/wt", "", "foo"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"new-session", "-d", "-s", "moomux-foo", "-c", "/tmp/wt", "-n", "foo"},
		{"set-window-option", "-t", "=moomux-foo:", "automatic-rename", "off"},
		{"set-option", "-t", "=moomux-foo:", "set-titles", "on"},
		{"set-option", "-t", "=moomux-foo:", "set-titles-string", "#{window_name}"},
		{"set-option", "-t", "=moomux-foo:", "mouse", "on"},
		{"list-panes", "-t", "=moomux-foo:", "-F", "#{pane_id}"},
		{"split-window", "-h", "-t", "=moomux-foo:", "-c", "/tmp/wt", "-l", "33%"},
		{"select-pane", "-t", "%3"},
	}
	if !reflect.DeepEqual(fr.calls, want) {
		t.Fatalf("calls = %v", fr.calls)
	}
}

func TestHasSessionPresent(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	ok, err := c.HasSession("moomux-foo")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHasSessionAbsent(t *testing.T) {
	fr := &fakeRunner{failOn: map[string]bool{"has-session -t =moomux-foo": true}}
	c := &Client{Runner: fr}
	ok, err := c.HasSession("moomux-foo")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestPaneCwd(t *testing.T) {
	fr := &fakeRunner{out: map[string]string{"list-panes -t =moomux-foo: -F #{pane_current_path}": "/tmp/wt\n"}}
	c := &Client{Runner: fr}
	got, err := c.PaneCwd("moomux-foo")
	if err != nil || got != "/tmp/wt" {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestKillSession(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.KillSession("moomux-foo"); err != nil {
		t.Fatal(err)
	}
	if got := fr.calls[0]; !reflect.DeepEqual(got, []string{"kill-session", "-t", "=moomux-foo"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSendKeys(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.SendKeys("moomux-foo", "do the thing"); err != nil {
		t.Fatal(err)
	}
	// "=moomux-foo:" pins send-keys to an exact session match; a bare name
	// falls back to prefix matching and could type into moomux-foo-2 once
	// moomux-foo is gone.
	want := []string{"send-keys", "-t", "=moomux-foo:", "do the thing", "Enter"}
	if got := fr.calls[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestSendKeysUsesExactSessionMatch guards against a bare session name in
// -t, which falls back to tmux prefix matching: if "moomux-foo" no longer
// exists but "moomux-foo-2" does, send-keys would silently type into the
// wrong session instead of failing.
func TestSendKeysUsesExactSessionMatch(t *testing.T) {
	fr := &fakeRunner{}
	c := &Client{Runner: fr}
	if err := c.SendKeys("moomux-foo", "hi"); err != nil {
		t.Fatal(err)
	}
	target := fr.calls[0][2]
	if !strings.HasPrefix(target, "=") {
		t.Fatalf("target %q is not pinned to an exact match (missing '=' prefix)", target)
	}
}

func TestSendKeysError(t *testing.T) {
	fr := &fakeRunner{failOn: map[string]bool{"send-keys -t =moomux-foo: hi Enter": true}}
	c := &Client{Runner: fr}
	if err := c.SendKeys("moomux-foo", "hi"); err == nil {
		t.Fatal("expected error")
	}
}
