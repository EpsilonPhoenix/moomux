package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erickgnclvs/moomux/internal/session"
)

func layoutTestModel(sessionCount int) *Model {
	sessions := make([]session.Session, sessionCount)
	for i := range sessions {
		name := fmt.Sprintf("session-%02d", i)
		sessions[i] = session.Session{
			ID:           "demo:" + name,
			Project:      "demo",
			Name:         name,
			WorktreePath: "/tmp/" + name,
			TmuxSession:  name,
		}
	}
	return newTestModel(&fakeBackend{sessions: sessions})
}

func TestNarrowShortLayoutShowsOnlySessionList(t *testing.T) {
	m := layoutTestModel(3)
	m.width, m.height = 50, 24

	view := m.View()

	if !strings.Contains(view, "SESSIONS") {
		t.Fatalf("short narrow view does not contain session list:\n%s", view)
	}
	if strings.Contains(view, "DETAIL") {
		t.Fatalf("short narrow view unexpectedly contains detail pane:\n%s", view)
	}
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("rendered height = %d, terminal height = %d", got, m.height)
	}
}

func TestNarrowTallLayoutShowsStackedDetail(t *testing.T) {
	m := layoutTestModel(3)
	m.width, m.height = 50, 40

	view := m.View()

	if !strings.Contains(view, "SESSIONS") || !strings.Contains(view, "DETAIL") {
		t.Fatalf("tall narrow view should contain both panes:\n%s", view)
	}
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("rendered height = %d, terminal height = %d", got, m.height)
	}
}

func TestNarrowLayoutRestoresDetailAfterResize(t *testing.T) {
	m := layoutTestModel(3)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
	if view := m.View(); strings.Contains(view, "DETAIL") {
		t.Fatalf("detail visible before resize:\n%s", view)
	}

	m.Update(tea.WindowSizeMsg{Width: 50, Height: 40})
	if view := m.View(); !strings.Contains(view, "DETAIL") {
		t.Fatalf("detail not restored after resize:\n%s", view)
	}
}

func TestNarrowShortLayoutKeepsSelectedSessionVisible(t *testing.T) {
	m := layoutTestModel(20)
	m.width, m.height = 50, 16
	m.cursor = len(m.sessions) - 1

	view := m.View()

	if !strings.Contains(view, m.sessions[m.cursor].Name) {
		t.Fatalf("selected session %q not visible:\n%s", m.sessions[m.cursor].Name, view)
	}
}

func TestViewNeverExceedsTerminalHeight(t *testing.T) {
	for _, tc := range []struct {
		width  int
		height int
	}{
		{width: 50, height: 8},
		{width: 50, height: 16},
		{width: 50, height: 24},
		{width: 50, height: 40},
		{width: 80, height: 12},
		{width: 80, height: 24},
	} {
		t.Run(fmt.Sprintf("%dx%d", tc.width, tc.height), func(t *testing.T) {
			m := layoutTestModel(20)
			m.width, m.height = tc.width, tc.height

			if got := lipgloss.Height(m.View()); got > tc.height {
				t.Fatalf("rendered height = %d, terminal height = %d", got, tc.height)
			}
		})
	}
}
