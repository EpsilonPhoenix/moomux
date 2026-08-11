package codexhook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureTagPromptCreatesFile(t *testing.T) {
	home := t.TempDir()
	changed, err := EnsureTagPrompt(home)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected changed=true on first install")
	}
	data, err := os.ReadFile(filepath.Join(home, ".codex", "prompts", "tag.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tagPrompt {
		t.Fatalf("unexpected content: %s", data)
	}
}

func TestEnsureTagPromptIsIdempotent(t *testing.T) {
	home := t.TempDir()
	if _, err := EnsureTagPrompt(home); err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureTagPrompt(home)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second install should be a no-op")
	}
}
