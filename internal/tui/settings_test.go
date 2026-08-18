package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestSettingsScreenTogglesAutoTmux(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	if m.cfg.AutoTmux {
		t.Fatal("expected AutoTmux to start false")
	}

	m.Update(runeKey('s'))
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // sort mode -> theme
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // theme -> auto-tmux
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.cfg.AutoTmux {
		t.Fatal("expected enter on the auto-tmux row to flip AutoTmux on")
	}
	if len(be.setAutoTmuxCalls) != 1 || !be.setAutoTmuxCalls[0] {
		t.Fatalf("expected backend.SetAutoTmux(true), got %v", be.setAutoTmuxCalls)
	}
	if m.mode != ModeSettings {
		t.Fatalf("expected toggling a row to keep the settings screen open, got %v", m.mode)
	}
}

func TestSettingsScreenTogglesAutoSubmitDefaultViaLeftRight(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(runeKey('s'))
	for i := 0; i < 3; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyDown}) // sort mode -> theme -> auto-tmux -> auto-submit default
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})

	if !m.cfg.AutoSubmitDefault {
		t.Fatal("expected right-arrow on the auto-submit-default row to flip it on")
	}
	if len(be.setAutoSubmitDefaultCalls) != 1 || !be.setAutoSubmitDefaultCalls[0] {
		t.Fatalf("expected backend.SetAutoSubmitDefault(true), got %v", be.setAutoSubmitDefaultCalls)
	}
}

func TestSettingsScreenCursorWraps(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(runeKey('s'))
	if m.settingsCursor != 0 {
		t.Fatalf("expected cursor to start at 0, got %d", m.settingsCursor)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if want := len(settingsRows) - 1; m.settingsCursor != want {
		t.Fatalf("expected up from row 0 to wrap to %d, got %d", want, m.settingsCursor)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.settingsCursor != 0 {
		t.Fatalf("expected down from the last row to wrap to 0, got %d", m.settingsCursor)
	}
}

func TestSettingsScreenEscReturnsToModeList(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(runeKey('s'))
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeList {
		t.Fatalf("expected esc to return to ModeList, got %v", m.mode)
	}
}

// TestSettingsFooterFitsNarrowWidths mirrors
// TestThemePickerFooterFitsNarrowWidths: the footer must never be wider than
// the overlay, or it gets hard-clipped mid-word on mobile widths.
func TestSettingsFooterFitsNarrowWidths(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	for _, width := range []int{200, 100, 72, 60, 50, 40} {
		m.width = width
		footer := m.settingsFooter()
		avail := m.overlayWidth(formHintWidth)
		if w := lipgloss.Width(footer); w > avail {
			t.Errorf("width=%d: footer %q is %d cells wide, want <= %d", width, footer, w, avail)
		}
	}
}
