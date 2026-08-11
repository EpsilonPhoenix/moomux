package claudehook

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureKillCommandInstalledCreatesFile(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureKillCommandInstalled(home)
	if err != nil {
		t.Fatalf("EnsureKillCommandInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for a brand-new install")
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "commands", "kill.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != killCommand {
		t.Fatalf("kill.md content = %q, want %q", data, killCommand)
	}
}

func TestEnsureKillCommandInstalledSkipsNoopWrite(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureKillCommandInstalled(home); err != nil {
		t.Fatalf("first EnsureKillCommandInstalled: %v", err)
	}
	path := filepath.Join(home, ".claude", "commands", "kill.md")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	backdated := before.ModTime().Add(-time.Hour)
	if err := os.Chtimes(path, backdated, backdated); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureKillCommandInstalled(home)
	if err != nil {
		t.Fatalf("second EnsureKillCommandInstalled: %v", err)
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

func TestEnsureKillCommandInstalledOverwritesStaleContent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "commands", "kill.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stale content from an older moomux build"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsureKillCommandInstalled(home)
	if err != nil {
		t.Fatalf("EnsureKillCommandInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when existing content differs")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != killCommand {
		t.Fatalf("kill.md content = %q, want %q", data, killCommand)
	}
}

func TestEnsureAllInstalledInstallsHooksAndCommand(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureAllInstalled(home)
	if err != nil {
		t.Fatalf("EnsureAllInstalled: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for a brand-new install")
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Fatalf("expected settings.json to be installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "commands", "kill.md")); err != nil {
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
