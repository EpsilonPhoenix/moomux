// Package claudehook writes and clears the "needs input" marker files that
// internal/watcher polls, and installs the Claude Code hooks that keep them
// up to date (see settings.go).
package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SessionsDir is the directory moomux's watcher already polls for Claude
// Code's own session status files.
func SessionsDir(home string) string {
	return filepath.Join(home, ".claude", "sessions")
}

func markerPath(home, sessionID string) string {
	return filepath.Join(SessionsDir(home), sessionID+".notify.json")
}

type marker struct {
	CWD    string `json:"cwd"`
	Status string `json:"status"`
}

// SetNeedsInput records that the session is blocked on the user (a
// permission prompt or an idle-prompt timeout) so the watcher's next poll
// picks it up as watcher.NeedsInput.
func SetNeedsInput(home, sessionID, cwd string) error {
	if err := os.MkdirAll(SessionsDir(home), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(marker{CWD: cwd, Status: "needs-input"})
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath(home, sessionID), data, 0o644)
}

// Clear removes the needs-input marker, e.g. once a tool runs or the user
// sends the next message.
func Clear(home, sessionID string) error {
	err := os.Remove(markerPath(home, sessionID))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
