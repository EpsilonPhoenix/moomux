package userscript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Before the project-specific dir was added, a script placed at
// .config/moomux/userscripts/<project>/worktree-create/ was silently never
// run because only the global worktree-create dir was scanned.
func TestRunWorktreeCreateRunsGlobalAndProjectScripts(t *testing.T) {
	home := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "log")

	writeScript(t, globalDir(home, "worktree-create"), "10-global.sh",
		"#!/bin/sh\necho global >> "+logFile+"\n")
	writeScript(t, projectDir(home, "myproj", "worktree-create"), "20-project.sh",
		"#!/bin/sh\necho project >> "+logFile+"\n")

	wt := t.TempDir()
	warnings := RunWorktreeCreate(home, Env{Project: "myproj", Worktree: wt})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	out, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("script did not run: %v", err)
	}
	if got := string(out); got != "global\nproject\n" {
		t.Fatalf("scripts ran out of order or missing, got %q", got)
	}
}

// A script that succeeds but prints to stdout used to be silently
// discarded — only failing scripts produced a message. Successful output
// should be surfaced too, so callers (e.g. the create-session hint) can show
// it to the user.
func TestRunWorktreeCreateSurfacesSuccessfulOutput(t *testing.T) {
	home := t.TempDir()
	writeScript(t, globalDir(home, "worktree-create"), "10-hello.sh",
		"#!/bin/sh\necho hello from script\n")

	wt := t.TempDir()
	messages := RunWorktreeCreate(home, Env{Project: "myproj", Worktree: wt})
	if len(messages) != 1 || !strings.Contains(messages[0], "hello from script") {
		t.Fatalf("expected script output surfaced, got %v", messages)
	}
}

func TestRunWorktreeDeleteOnlyRunsDeleteScripts(t *testing.T) {
	home := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "log")

	writeScript(t, globalDir(home, "worktree-create"), "create.sh",
		"#!/bin/sh\necho create >> "+logFile+"\n")
	writeScript(t, globalDir(home, "worktree-delete"), "delete.sh",
		"#!/bin/sh\necho delete >> "+logFile+"\n")

	wt := t.TempDir()
	if warnings := RunWorktreeDelete(home, Env{Project: "myproj", Worktree: wt}); len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	out, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("script did not run: %v", err)
	}
	if got := string(out); got != "delete\n" {
		t.Fatalf("expected only delete script to run, got %q", got)
	}
}
