package tui

import (
	"strings"
	"testing"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

// TestDetailTruncatesLongTicketURLButKeepsItClickable asserts a ticket/PR
// URL too long for the detail panel's value column is truncated on screen,
// but the click target recorded for that row still carries the full URL —
// so tapping the truncated text still opens/copies the real link.
func TestDetailTruncatesLongTicketURLButKeepsItClickable(t *testing.T) {
	longURL := "https://tracker.example.com/projects/some-very-long-project-name/issues/TICK-123456"
	cfg := &config.Config{Projects: map[string]config.Project{"demo": {Repo: "/tmp/demo"}}}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:one", Project: "demo", Name: "one", Ticket: longURL},
	}}
	statusCh := make(chan watcher.Snapshot)
	m := New(cfg, be, statusCh, func() {})
	m.width, m.height = 80, 24

	frame, hits := m.renderDetail(80-2, 24-2)
	joined := strings.Join(strings.Fields(frame), "")
	if strings.Contains(joined, strings.ReplaceAll(longURL, " ", "")) {
		t.Errorf("expected the long ticket URL to be truncated on screen, but it appeared unbroken; frame:\n%s", frame)
	}

	var found bool
	for _, h := range hits {
		if h.url == longURL {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a link hit for the full ticket URL %q, got hits: %+v", longURL, hits)
	}
}

func TestDetailLinkHitsSurviveWrappedRows(t *testing.T) {
	// A long value that wraps in the rendered panel must not shift the
	// hitboxes of the rows below it.
	m := layoutTestModel(1)
	m.sessions[0].WorktreePath = "/tmp/wt"
	m.sessions[0].Agent = "an-agent-name-far-longer-than-any-narrow-detail-pane-can-hold-on-one-line"
	m.sessions[0].Ticket = "https://t/1"

	width := 30
	rendered, hits := m.renderDetail(width, 24)
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	lines := strings.Split(rendered, "\n")
	ticketLine := -1
	for i, l := range lines {
		if strings.Contains(l, "ticket") {
			ticketLine = i
			break
		}
	}
	if ticketLine == -1 {
		t.Fatalf("no ticket row rendered:\n%s", rendered)
	}
	if hits[0].line != ticketLine {
		t.Fatalf("hit line = %d, rendered ticket row = %d\n%s", hits[0].line, ticketLine, rendered)
	}
}
