package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderWhatsNew renders the release notes behind the ModeWhatsNew overlay:
// one block per release, a heading per category, a bullet per change.
func (m *Model) renderWhatsNew() string {
	avail := m.overlayWidth(formHintWidth)
	var b strings.Builder
	title := "what's new"
	if len(m.whatsNew) == 1 {
		title += " in v" + m.whatsNew[0].Version
	}
	b.WriteString(titleStyle.Render(title))
	for _, r := range m.whatsNew {
		b.WriteString("\n\n")
		if len(m.whatsNew) > 1 {
			b.WriteString(helpGroupStyle.Render("v"+r.Version) + "\n")
		}
		for i, s := range r.Sections {
			if i > 0 {
				b.WriteString("\n")
			}
			if s.Title != "" {
				b.WriteString(helpKeyStyle.Render(s.Title) + "\n")
			}
			for _, item := range s.Items {
				const prefix = "  • "
				text := helpDescStyle.Width(max(avail-lipgloss.Width(prefix), 8)).Render(item)
				b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, prefix, text) + "\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
