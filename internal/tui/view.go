package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// narrowWidthBreak is the terminal width below which the list and detail
// panels stack vertically instead of side by side (phone-sized SSH clients).
const narrowWidthBreak = 72

var superDigits = [...]string{"⁰", "¹", "²", "³", "⁴", "⁵", "⁶", "⁷", "⁸", "⁹"}

// superscript renders n using superscript Unicode digits, e.g. 12 -> "¹²".
func superscript(n int) string {
	var b strings.Builder
	for _, r := range strconv.Itoa(n) {
		b.WriteString(superDigits[r-'0'])
	}
	return b.String()
}

// truncateToWidth clips s to at most w cells, appending an ellipsis if it
// had to cut. Used to keep footer rows to a single, fixed-height line so the
// overall layout never shifts between frames.
func truncateToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	r := []rune(s)
	out := make([]rune, 0, len(r))
	width := 0
	for _, c := range r {
		cw := lipgloss.Width(string(c))
		if width+cw > w-1 {
			break
		}
		out = append(out, c)
		width += cw
	}
	return string(out) + "…"
}

func (m *Model) View() string {
	if m.width == 0 {
		return "starting…"
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	// -2 accounts for panelBorder's top/bottom border lines, which sit
	// outside the Height() passed to it below.
	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if bodyHeight < 5 {
		bodyHeight = 5
	}

	var body string
	var hits []linkHit
	// Below this width a side-by-side list+detail split leaves too little
	// room for either panel (common on phone-sized SSH clients), so stack
	// them instead.
	if m.width < narrowWidthBreak {
		panelW := m.width - 2
		if panelW < 20 {
			panelW = 20
		}
		listH := bodyHeight / 2
		if listH < 5 {
			listH = 5
		}
		detailH := bodyHeight - listH
		if detailH < 5 {
			detailH = 5
		}
		var listContent string
		listContent, hits = m.renderList(panelW-2, listH-2)
		top := panelBorder.Width(panelW).Height(listH).Render(listContent)
		bottom := panelBorder.Width(panelW).Height(detailH).Render(m.renderDetail(panelW-2, detailH-2))
		body = lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	} else {
		listW := 42
		if m.width-listW < 30 {
			listW = m.width / 2
		}
		if listW < 20 {
			listW = 20
		}
		detailW := m.width - listW - 2
		if detailW < 20 {
			detailW = 20
		}

		var listContent string
		listContent, hits = m.renderList(listW-2, bodyHeight-2)
		left := panelBorder.Width(listW).Height(bodyHeight).Render(listContent)
		right := panelBorder.Width(detailW).Height(bodyHeight).Render(m.renderDetail(detailW-2, bodyHeight-2))
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	m.updateLinkHits(header, hits)

	base := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)

	switch m.mode {
	case ModeNewForm:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayBox.Render(m.renderNewForm()))
	case ModeConfirmDelete:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayBox.Render(m.renderConfirm()))
	case ModeNewProject:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayBox.Render(m.renderNewProject()))
	case ModeConfirmDeleteProject:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayBox.Render(m.renderConfirmDeleteProject()))
	case ModeProjectInitChoice:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayBox.Render(m.renderProjectInitChoice()))
	case ModeTagForm:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayBox.Render(m.renderTagForm()))
	case ModeHelp:
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlayBox.Render(m.renderHelp()))
	}
	return base
}

func (m *Model) renderHeader() string {
	cow := cowStyle.Render("  ^__^\n  (oo)\\_\n  (__)\\ )")
	wordmark := titleStyle.Render("moomux")
	left := lipgloss.JoinHorizontal(lipgloss.Center, cow, "  ", wordmark)

	counts := map[string]int{}
	for _, s := range m.backend.Sessions() {
		if !s.Archived {
			counts[s.Project]++
		}
	}
	tabs := []string{}
	for i, p := range m.projects {
		label := p
		if n := counts[p]; n > 0 {
			label = p + superscript(n)
		}
		if i == m.activeProj {
			tabs = append(tabs, tabActive.Render(label))
		} else {
			tabs = append(tabs, tabInactive.Render(label))
		}
	}
	right := strings.Join(tabs, " ")

	remaining := m.width - 2 - lipgloss.Width(left)
	if remaining < lipgloss.Width(right) {
		remaining = lipgloss.Width(right)
	}
	rightCol := lipgloss.NewStyle().Width(remaining).Align(lipgloss.Right).Render(right)
	row := lipgloss.JoinHorizontal(lipgloss.Center, left, rightCol)
	return lipgloss.NewStyle().Padding(0, 1).Render(row)
}

// renderFooter always returns exactly two lines — a message row (blank when
// there's no flash) and a hints row — so the overall layout height never
// changes between frames. Both rows are truncated rather than word-wrapped;
// letting them grow to variable heights previously caused the body/footer
// split to jitter across renders, which could leave stale content on screen
// or push the hints row out of view.
func (m *Model) renderFooter() string {
	// The footer advertises a single entry point — the full command reference
	// lives behind ?:help. Left-aligned to line up with the header and panels.
	// subtract 2 for the footer's horizontal padding (Padding(0,1) = 1 each side)
	inner := m.width - 2
	hint := helpKeyStyle.Foreground(colAccent).Render("?") + helpDescStyle.Render(" help")
	hintRow := lipgloss.NewStyle().Width(inner).Render(hint)

	messageLine := ""
	if m.flash != "" {
		flashStyle := infoFlashStyle
		prefix := "✓ "
		if m.flashKind == "error" {
			flashStyle = errorFlashStyle
			prefix = "✖ "
		}
		messageLine = flashStyle.Render(truncateToWidth(prefix+m.flash, inner))
	}
	messageRow := lipgloss.NewStyle().Width(inner).Render(messageLine)

	rows := lipgloss.JoinVertical(lipgloss.Left, messageRow, hintRow)
	return footerStyle.Width(m.width).Render(rows)
}
