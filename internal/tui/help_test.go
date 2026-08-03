package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/erickgnclvs/moomux/internal/session"
)

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestHelpToggle(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{
		// A ticket URL produces a clickable link hit, so we can prove the
		// overlay clears the (now-hidden) clickable list behind it.
		{ID: "demo:a", Project: "demo", Name: "a", Ticket: "https://example/T-1"},
	}}
	m := newTestModel(be)
	// The help overlay lists every command across two side-by-side columns;
	// at newTestModel's default 24 rows it's genuinely too short to show the
	// full list without scrolling (the R row alone wraps to 3 lines). Use a
	// height tall enough to render the whole thing in one View() call, same
	// as most real terminals.
	m.height = 34

	if m.mode != ModeList {
		t.Fatalf("expected ModeList initially, got %v", m.mode)
	}

	// Render the list once so link hits are populated from the visible rows.
	m.View()
	if len(m.linkHits) == 0 {
		t.Fatalf("expected link hits populated in list view, got none")
	}

	// ? opens the help overlay.
	m.Update(runeKey('?'))
	if m.mode != ModeHelp {
		t.Fatalf("expected ModeHelp after '?', got %v", m.mode)
	}

	// The overlay lists commands and clears the clickable list behind it so a
	// click can't resolve to a now-hidden session's URL.
	view := m.View()
	if !strings.Contains(view, "moomux commands") {
		t.Fatalf("help view missing title, got:\n%s", view)
	}
	// "picker" alone rather than a longer phrase: the description column
	// wraps at a width that depends on the overlay's layout, and a longer
	// phrase can end up split across lines by that wrap.
	if !strings.Contains(view, "edit session") || !strings.Contains(view, "picker") {
		t.Fatalf("help view missing edit commands, got:\n%s", view)
	}
	if m.linkHits != nil {
		t.Fatalf("expected link hits cleared behind overlay, got %v", m.linkHits)
	}

	// ? closes it again.
	m.Update(runeKey('?'))
	if m.mode != ModeList {
		t.Fatalf("expected ModeList after second '?', got %v", m.mode)
	}
}

func TestHelpEscCloses(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}}}
	m := newTestModel(be)

	m.Update(runeKey('?'))
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeList {
		t.Fatalf("expected ModeList after esc, got %v", m.mode)
	}
}
