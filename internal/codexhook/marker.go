// Package codexhook writes and clears the "needs input" marker files that
// internal/watcher polls, and installs the Codex CLI hooks that keep them up
// to date (see settings.go).
package codexhook

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// MarkerDir is the directory moomux's watcher polls for Codex needs-input
// markers — Codex itself has no per-session status file here (unlike
// Claude's ~/.claude/sessions/), so this directory exists purely for moomux.
func MarkerDir(home string) string {
	return filepath.Join(home, ".codex", "moomux-notify")
}

// markerPath is keyed by cwd, not a session/thread id: Codex's own activity
// table (see internal/watcher's SQLiteWatcher usage in main.go) is keyed by
// cwd too, and moomux runs at most one Codex session per worktree, so cwd is
// already a stable, collision-free key without needing Codex's thread id.
func markerPath(home, cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return filepath.Join(MarkerDir(home), hex.EncodeToString(sum[:])+".json")
}

type marker struct {
	CWD    string `json:"cwd"`
	Status string `json:"status"`
}

// SetNeedsInput records that the session is blocked on the user (an
// approval/permission request) so the watcher's next poll picks it up as
// watcher.NeedsInput.
func SetNeedsInput(home, cwd string) error {
	if err := os.MkdirAll(MarkerDir(home), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(marker{CWD: cwd, Status: "needs-input"})
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath(home, cwd), data, 0o644)
}

// Clear removes the needs-input marker, e.g. once a tool runs or the user
// sends the next message.
func Clear(home, cwd string) error {
	err := os.Remove(markerPath(home, cwd))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
