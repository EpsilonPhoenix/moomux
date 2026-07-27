package tui

import (
	"strings"
	"testing"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

// TestDetailWrapsLongTicketURL asserts a ticket/PR URL too long for the
// detail panel's value column is wrapped across lines rather than
// truncated, so the whole URL stays visible (and tappable, for SSH clients
// that recognize URLs by their on-screen text).
func TestDetailWrapsLongTicketURL(t *testing.T) {
	longURL := "https://tracker.example.com/projects/some-very-long-project-name/issues/TICK-123456"
	cfg := &config.Config{Projects: map[string]config.Project{"demo": {Repo: "/tmp/demo"}}}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:one", Project: "demo", Name: "one", Ticket: longURL},
	}}
	statusCh := make(chan watcher.Snapshot)
	m := New(cfg, be, statusCh, func() {})
	m.width, m.height = 80, 24

	frame := m.View()
	joined := strings.Join(strings.Fields(frame), "")
	if !strings.Contains(joined, strings.ReplaceAll(longURL, " ", "")) {
		t.Errorf("rendered frame doesn't contain the full ticket URL unbroken; frame:\n%s", frame)
	}
}
