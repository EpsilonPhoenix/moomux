package codexhook

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureKillPromptCreatesFile(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureKillPrompt(home)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on first install")
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "prompts", "kill.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != killPrompt {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestEnsureKillPromptIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureKillPrompt(home); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureKillPrompt(home)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second install should be a no-op")
	}
}

func TestEnsureKillPromptSkipsNoopWrite(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureKillPrompt(home); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".codex", "prompts", "kill.md")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	backdated := before.ModTime().Add(-time.Hour)
	if err := os.Chtimes(path, backdated, backdated); err != nil {
		t.Fatal(err)
	}

	if changed, err := EnsureKillPrompt(home); err != nil {
		t.Fatal(err)
	} else if changed {
		t.Fatal("expected changed=false for a no-op call")
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(backdated) {
		t.Fatalf("expected no-op call to leave the file untouched, mtime changed from %v to %v", backdated, after.ModTime())
	}
}

func TestEnsureKillPromptOverwritesStaleContent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "prompts", "kill.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale content from an older moomux build"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureKillPrompt(home)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true when existing content differs")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != killPrompt {
		t.Fatalf("unexpected content: %s", data)
	}
}
