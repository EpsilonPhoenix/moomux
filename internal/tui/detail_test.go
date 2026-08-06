package tui

import (
	"strings"
	"testing"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/prstatus"
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
	var ticketHit *linkHit
	for i := range hits {
		if hits[i].url == "https://t/1" {
			ticketHit = &hits[i]
		}
	}
	if ticketHit == nil {
		t.Fatalf("no ticket hit found, hits = %+v", hits)
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
	if ticketHit.line != ticketLine {
		t.Fatalf("hit line = %d, rendered ticket row = %d\n%s", ticketHit.line, ticketLine, rendered)
	}
}

// TestPRStatusLabel guards the label mapping: merged/closed wins outright
// (mergeable/CI stop being meaningful once a PR is done), otherwise
// conflicts and CI state are reported together.
func TestPRStatusLabel(t *testing.T) {
	cases := []struct {
		name string
		info prstatus.Info
		want string
	}{
		{"merged", prstatus.Info{State: "MERGED", Mergeable: "CONFLICTING", CI: "FAILING"}, "merged"},
		{"closed", prstatus.Info{State: "CLOSED"}, "closed"},
		{"open, no checks", prstatus.Info{State: "OPEN", Mergeable: "MERGEABLE", CI: "NONE"}, "open"},
		{"open, passing", prstatus.Info{State: "OPEN", Mergeable: "MERGEABLE", CI: "PASSING"}, "open, CI passing"},
		{"open, conflicting", prstatus.Info{State: "OPEN", Mergeable: "CONFLICTING", CI: "NONE"}, "open, conflicts"},
		{"open, conflicting and failing", prstatus.Info{State: "OPEN", Mergeable: "CONFLICTING", CI: "FAILING"}, "open, conflicts, CI failing"},
		{"open, pending", prstatus.Info{State: "OPEN", Mergeable: "MERGEABLE", CI: "PENDING"}, "open, CI running"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := prStatusLabel(tc.info); got != tc.want {
				t.Fatalf("prStatusLabel(%+v) = %q, want %q", tc.info, got, tc.want)
			}
		})
	}
}

// TestDetailShowsPRStatusRowOnlyWhenCached guards the detail panel's wiring:
// the "pr status" row only appears once a status has actually resolved into
// m.prStatus, not merely because the session has a PR attached — otherwise
// a session that just gained a PR (before its first gh pr view resolves)
// would show a misleading or empty status.
func TestDetailShowsPRStatusRowOnlyWhenCached(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{"demo": {Repo: "/tmp/demo"}}}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:one", Project: "demo", Name: "one", PR: "https://github.com/example/repo/pull/1"},
	}}
	statusCh := make(chan watcher.Snapshot)
	m := New(cfg, be, statusCh, func() {})
	m.width, m.height = 80, 24

	frame, _ := m.renderDetail(80-2, 24-2)
	if strings.Contains(frame, "pr status") {
		t.Fatalf("expected no pr status row before a status resolves:\n%s", frame)
	}

	m.prStatus["demo:one"] = prStatusInfo{ok: true, info: prstatus.Info{State: "OPEN", Mergeable: "CONFLICTING", CI: "FAILING"}}
	frame, _ = m.renderDetail(80-2, 24-2)
	if !strings.Contains(frame, "conflicts") || !strings.Contains(frame, "CI failing") {
		t.Fatalf("expected the cached pr status to render:\n%s", frame)
	}
}
