package claudehook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureTagCommandCreatesFile(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureTagCommand(home)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on first install")
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "commands", "tag.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tagCommand {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestEnsureTagCommandIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureTagCommand(home); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureTagCommand(home)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second install should be a no-op")
	}
}
