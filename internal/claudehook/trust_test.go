package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readClaudeJSON(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude.json"))
	if err != nil {
		t.Fatalf("read .claude.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal .claude.json: %v", err)
	}
	return m
}

func TestTrustDirectoryCreatesEntry(t *testing.T) {
	home := t.TempDir()
	dir := "/repo/worktrees/feature-x"

	if err := TrustDirectory(home, dir); err != nil {
		t.Fatal(err)
	}

	root := readClaudeJSON(t, home)
	entry, ok := root[dir].(map[string]any)
	if !ok {
		t.Fatalf("no entry for %q: %v", dir, root)
	}
	if entry["hasTrustDialogAccepted"] != true {
		t.Fatalf("hasTrustDialogAccepted = %v, want true", entry["hasTrustDialogAccepted"])
	}
	if entry["hasCompletedProjectOnboarding"] != true {
		t.Fatalf("hasCompletedProjectOnboarding = %v, want true", entry["hasCompletedProjectOnboarding"])
	}
}

func TestTrustDirectoryPreservesOtherProjectsAndFields(t *testing.T) {
	home := t.TempDir()
	existing := `{
		"/other/project": {"hasTrustDialogAccepted": false, "allowedTools": ["Bash"]},
		"/repo/worktrees/feature-x": {"hasTrustDialogAccepted": false, "projectOnboardingSeenCount": 3}
	}`
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := TrustDirectory(home, "/repo/worktrees/feature-x"); err != nil {
		t.Fatal(err)
	}

	root := readClaudeJSON(t, home)
	other, ok := root["/other/project"].(map[string]any)
	if !ok || other["hasTrustDialogAccepted"] != false {
		t.Fatalf("unrelated project entry was touched: %v", root["/other/project"])
	}
	entry := root["/repo/worktrees/feature-x"].(map[string]any)
	if entry["hasTrustDialogAccepted"] != true {
		t.Fatalf("hasTrustDialogAccepted = %v, want true", entry["hasTrustDialogAccepted"])
	}
	if entry["projectOnboardingSeenCount"] != float64(3) {
		t.Fatalf("existing field projectOnboardingSeenCount was dropped: %v", entry)
	}
}
