package codexhook

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureKillPromptInstalledCreatesFile(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureKillPromptInstalled(home)
	if err != nil {
		t.Fatalf("EnsureKillPromptInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for a brand-new install")
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "prompts", "kill.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != killPrompt {
		t.Fatalf("kill.md content = %q, want %q", data, killPrompt)
	}
}

func TestEnsureKillPromptInstalledSkipsNoopWrite(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureKillPromptInstalled(home); err != nil {
		t.Fatalf("first EnsureKillPromptInstalled: %v", err)
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

	changed, err := EnsureKillPromptInstalled(home)
	if err != nil {
		t.Fatalf("second EnsureKillPromptInstalled: %v", err)
	}
	if changed {
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

func TestEnsureKillPromptInstalledOverwritesStaleContent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "prompts", "kill.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale content from an older moomux build"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureKillPromptInstalled(home)
	if err != nil {
		t.Fatalf("EnsureKillPromptInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when existing content differs")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != killPrompt {
		t.Fatalf("kill.md content = %q, want %q", data, killPrompt)
	}
}

func TestEnsureAllInstalledInstallsHooksAndPrompt(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureAllInstalled(home)
	if err != nil {
		t.Fatalf("EnsureAllInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for a brand-new install")
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "hooks.json")); err != nil {
		t.Fatalf("expected hooks.json to be installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "prompts", "kill.md")); err != nil {
		t.Fatalf("expected kill.md to be installed: %v", err)
	}

	changed, err = EnsureAllInstalled(home)
	if err != nil {
		t.Fatalf("second EnsureAllInstalled: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false once both are already installed")
	}
}
