package tui

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/erickgnclvs/moomux/internal/config"
)

// newSessionDraftPath is where the new-session form keeps the first prompt
// until a create succeeds. A prompt can be paragraphs of work, and every
// way off the form other than success — Esc, ctrl+c, a failed create
// followed by Esc, the TUI dying mid-create, an SSH drop — used to throw it
// away. It sits beside client.toml: it is this front end's state, not the
// core's. A var so TestMain can turn it off for the package (tests share one
// XDG dir, and a draft left by one test would prefill the next one's form).
var newSessionDraftPath = func() string {
	return filepath.Join(filepath.Dir(config.ClientPath()), "new-session-draft.txt")
}

// saveNewSessionDraft writes the form's prompt to the draft file, or
// removes the file when the prompt is blank.
func (m *Model) saveNewSessionDraft() {
	if m.draftPath == "" {
		return
	}
	prompt := m.promptInput.Value()
	if strings.TrimSpace(prompt) == "" {
		m.clearNewSessionDraft()
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.draftPath), 0o700); err != nil {
		slog.Warn("save new-session draft", "path", m.draftPath, "err", err)
		return
	}
	if err := os.WriteFile(m.draftPath, []byte(prompt), 0o600); err != nil {
		slog.Warn("save new-session draft", "path", m.draftPath, "err", err)
	}
}

func (m *Model) clearNewSessionDraft() {
	if m.draftPath == "" {
		return
	}
	if err := os.Remove(m.draftPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("clear new-session draft", "path", m.draftPath, "err", err)
	}
}

// loadNewSessionDraft returns the saved prompt, or "" when there is none.
func (m *Model) loadNewSessionDraft() string {
	if m.draftPath == "" {
		return ""
	}
	b, err := os.ReadFile(m.draftPath)
	if err != nil {
		return ""
	}
	return string(b)
}
