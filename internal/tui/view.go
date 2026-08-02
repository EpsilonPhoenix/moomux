package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// narrowWidthBreak is the terminal width below which the list and detail
// panels stack vertically instead of side by side (phone-sized SSH clients).
const narrowWidthBreak = 72

// minStackedPaneHeight is the minimum usable content height for each panel in
// the narrow stacked layout. When a mobile keyboard leaves fewer rows than
// this for both panes, the list gets the full body instead of being pushed
// above the visible viewport by the detail pane.
const minStackedPaneHeight = 10

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

// compactOverlayContent removes decorative blank rows when a mobile keyboard
// leaves little vertical space. The viewport still provides scrolling when
// the compact content cannot fit.
func (m *Model) compactOverlayContent(content string) string {
	if m.height < 20 {
		content = strings.ReplaceAll(content, "\n\n", "\n")
	}
	return strings.TrimRight(content, "\n")
}

func lineContaining(content, needle string) int {
	if needle == "" {
		return 0
	}
	if i := strings.Index(content, needle); i >= 0 {
		return strings.Count(content[:i], "\n")
	}
	return 0
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) formFooter(hint, controls, errText string) string {
	if m.overlayWidth(formHintWidth) < 28 {
		// Put escape first when there is not enough width for descriptive
		// controls so the essential way out is never the truncated portion.
		controls = "esc  tab/↑↓  enter"
	}
	hint = strings.TrimRight(m.renderFormHint(hint), "\n")
	var rows []string
	if hint != "" {
		rows = append(rows, hint)
	}
	if errText != "" {
		rows = append(rows, dangerStyle.Render(errText))
	}
	rows = append(rows, muteStyle.Render(controls))
	return strings.Join(rows, "\n")
}

func (m *Model) helpFooter() string {
	controls := "↑↓/jk/pg scroll  ?/esc close"
	if m.overlayWidth(formHintWidth) < 28 {
		controls = "?/esc close  ↑↓ scroll"
	}
	return muteStyle.Render(controls)
}

// focusedOverlayLine returns the rendered line containing the active form
// control. It is used to bring that control back into view after focus or
// terminal-size changes without overriding subsequent manual scrolling.
func (m *Model) focusedOverlayLine(content string) int {
	switch m.mode {
	case ModeNewForm:
		switch m.newFormFocus {
		case newFormProjFocus:
			return lineContaining(content, m.renderNewFormProjectSelector())
		case 2:
			return lineContaining(content, m.branchInput.View())
		case newFormAgentFocus:
			return lineContaining(content, m.renderNewFormAgentSelector())
		case 4:
			return lineContaining(content, m.ticketInput.View())
		default:
			return lineContaining(content, m.nameInput.View())
		}
	case ModeNewProject:
		if m.projForm.focus < len(m.projForm.inputs) {
			return lineContaining(content, m.projForm.inputs[m.projForm.focus].View())
		}
		if m.projForm.focus == projFormInputCount {
			return lineContaining(content, m.renderAgentSelector())
		}
		return lineContaining(content, m.renderWorktreeToggle())
	case ModeTagForm:
		if m.tagForm.focus < len(m.tagForm.inputs) {
			return lineContaining(content, m.tagForm.inputs[m.tagForm.focus].View())
		}
	case ModeEditSession:
		agent := agentChoices[m.sessionForm.agentIdx]
		return lineContaining(content, titleStyle.Render("["+agent+"]"))
	case ModeEditProject:
		if m.projForm.focus < len(m.projForm.inputs) {
			return lineContaining(content, m.projForm.inputs[m.projForm.focus].View())
		}
		if m.projForm.focus == projFormInputCount {
			return lineContaining(content, m.renderAgentSelector())
		}
		return lineContaining(content, m.renderWorktreeToggle())
	}
	return -1
}

// renderOverlay constrains dialog content to the current terminal and gives
// it a persistent vertical viewport. footer remains visible while content
// scrolls; focusedLine is automatically revealed only after focus/size/mode
// changes so explicit scrolling is not immediately undone on the next frame.
func (m *Model) renderOverlay(content, footer string, focusedLine int) string {
	content = m.compactOverlayContent(content)

	maxWidth := m.overlayWidth(formHintWidth)
	contentWidth := lipgloss.Width(content)
	if fw := lipgloss.Width(footer); fw > contentWidth {
		contentWidth = fw
	}
	if contentWidth > maxWidth {
		contentWidth = maxWidth
	}
	if contentWidth < 1 {
		contentWidth = 1
	}

	availableHeight := m.height - overlayBox.GetVerticalFrameSize()
	if availableHeight < 1 {
		availableHeight = 1
	}
	footerHeight := 0
	separatorHeight := 0
	if footer != "" && availableHeight >= 2 {
		footerHeight = lipgloss.Height(footer)
		if footerHeight > availableHeight-1 {
			footerHeight = availableHeight - 1
			footer = lastLines(footer, footerHeight)
		}
		if availableHeight-footerHeight >= 2 {
			separatorHeight = 1
		}
	}
	contentHeight := availableHeight - footerHeight - separatorHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	if h := lipgloss.Height(content); h < contentHeight {
		contentHeight = h
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	m.overlayViewport.Width = contentWidth
	m.overlayViewport.Height = contentHeight
	m.overlayViewport.SetContent(content)

	if focusedLine >= 0 && (m.overlayMode != m.mode || m.overlayFocus != focusedLine) {
		// Keep the focused row visible at every height. Reserve one row above
		// it only when the viewport is tall enough; margins must never move
		// the focus itself outside a one- or two-row viewport.
		contextAbove := 0
		if contentHeight >= 3 {
			contextAbove = 1
		}
		m.overlayViewport.SetYOffset(focusedLine - contextAbove)
	}
	m.overlayMode = m.mode
	m.overlayFocus = focusedLine

	body := m.overlayViewport.View()
	if footerHeight > 0 {
		// One newline joins the viewport and footer as adjacent rows; any
		// separator height represents additional blank rows between them.
		body += strings.Repeat("\n", separatorHeight+1)
		body += lipgloss.NewStyle().
			MaxWidth(contentWidth).
			MaxHeight(footerHeight).
			Render(footer)
	}
	box := overlayBox.Render(body)
	placed := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(placed)
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
	var detailHits []linkHit
	var detailX, detailY int
	// Below this width a side-by-side list+detail split leaves too little
	// room for either panel (common on phone-sized SSH clients), so stack
	// them instead.
	if m.width < narrowWidthBreak {
		panelW := m.width - 2
		if panelW < 20 {
			panelW = 20
		}

		// A stacked layout has two pairs of horizontal borders, while
		// bodyHeight reserves room for one. Account for the second pair before
		// splitting the content height between the panes.
		stackedHeight := bodyHeight - 2
		if stackedHeight < 2*minStackedPaneHeight {
			var listContent string
			listContent, hits = m.renderList(panelW-2, bodyHeight)
			body = panelBorder.Width(panelW).Height(bodyHeight).Render(listContent)
		} else {
			listH := stackedHeight / 2
			detailH := stackedHeight - listH
			var listContent string
			listContent, hits = m.renderList(panelW-2, listH)
			top := panelBorder.Width(panelW).Height(listH).Render(listContent)
			var detailContent string
			detailContent, detailHits = m.renderDetail(panelW-2, detailH)
			bottom := panelBorder.Width(panelW).Height(detailH).Render(detailContent)
			detailX = panelBorder.GetBorderLeftSize() + panelBorder.GetPaddingLeft()
			detailY = lipgloss.Height(header) + lipgloss.Height(top) + panelBorder.GetBorderTopSize()
			body = lipgloss.JoinVertical(lipgloss.Left, top, bottom)
		}
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

		// bodyHeight already excludes the panel's two border rows (the -2
		// where it's computed above); subtracting again here left two blank
		// filler rows per panel and clipped link hits near the bottom.
		var listContent string
		listContent, hits = m.renderList(listW-2, bodyHeight)
		left := panelBorder.Width(listW).Height(bodyHeight).Render(listContent)
		var detailContent string
		detailContent, detailHits = m.renderDetail(detailW-2, bodyHeight)
		right := panelBorder.Width(detailW).Height(bodyHeight).Render(detailContent)
		detailX = lipgloss.Width(left) + panelBorder.GetBorderLeftSize() + panelBorder.GetPaddingLeft()
		detailY = lipgloss.Height(header) + panelBorder.GetBorderTopSize()
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}

	m.updateLinkHits(header, hits, detailHits, detailX, detailY)

	base := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	// Very small terminal sizes can be shorter/narrower than the fixed
	// header and footer combined. Never emit more rows or columns than the
	// terminal reported, otherwise terminal viewport panning can hide the
	// top/side of the list.
	base = lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(base)

	switch m.mode {
	case ModeNewForm:
		content := m.compactOverlayContent(m.renderNewForm())
		footer := m.formFooter(newFormFieldHints[m.newFormFocus], "tab/↑↓ fields  ←→ choose  enter  esc cancel", m.newFormErr)
		return m.renderOverlay(content, footer, m.focusedOverlayLine(content))
	case ModeConfirmDelete:
		return m.renderOverlay(m.renderConfirm(), "", -1)
	case ModeNewProject:
		content := m.compactOverlayContent(m.renderNewProject())
		footer := m.formFooter(projFormFieldHints[m.projForm.focus], "tab/↑↓  ←→ choose  enter save  esc cancel", m.projForm.err)
		return m.renderOverlay(content, footer, m.focusedOverlayLine(content))
	case ModeConfirmDeleteProject:
		return m.renderOverlay(m.renderConfirmDeleteProject(), "", -1)
	case ModeProjectInitChoice:
		return m.renderOverlay(m.renderProjectInitChoice(), "", -1)
	case ModeTagForm:
		content := m.compactOverlayContent(m.renderTagForm())
		footer := m.formFooter(tagFormFieldHints[m.tagForm.focus], "tab/↑↓ fields  enter save  esc cancel", "")
		return m.renderOverlay(content, footer, m.focusedOverlayLine(content))
	case ModeHelp:
		return m.renderOverlay(m.renderHelp(), m.helpFooter(), -1)
	case ModeEditSession:
		content := m.compactOverlayContent(m.renderEditSession())
		footer := m.formFooter(
			"agent used the next time this session's tmux process is created",
			"←→ agent  enter save  esc cancel",
			m.sessionForm.err,
		)
		return m.renderOverlay(content, footer, m.focusedOverlayLine(content))
	case ModeEditProject:
		content := m.compactOverlayContent(m.renderEditProject())
		footer := m.formFooter(editProjectFieldHints[m.projForm.focus], "tab/↑↓  ←→ change  enter save  esc cancel", m.projForm.err)
		return m.renderOverlay(content, footer, m.focusedOverlayLine(content))
	}
	return base
}

func (m *Model) renderHeader() string {
	cow := cowStyle.Render("  ^__^\n  (oo)\\_\n  (__)\\ )")
	left := cow
	if m.width >= narrowWidthBreak {
		wordmark := titleStyle.Render("moomux")
		left = lipgloss.JoinHorizontal(lipgloss.Center, cow, "  ", wordmark)
	}

	counts := map[string]int{}
	for _, s := range m.backend.Sessions() {
		if !s.Archived {
			counts[s.Project]++
		}
	}

	projectLabel := func(i int) string {
		p := m.projects[i]
		label := p
		if n := counts[p]; n > 0 {
			label = p + superscript(n)
		}
		return label
	}

	remaining := m.width - 2 - lipgloss.Width(left)
	if remaining < 0 {
		remaining = 0
	}

	var right string
	if m.width < narrowWidthBreak {
		if len(m.projects) > 0 && m.activeProj < len(m.projects) && remaining > 2 {
			// tabActive contributes one padding cell on each side.
			label := truncateToWidth(projectLabel(m.activeProj), remaining-2)
			right = tabActive.Render(label)
		}
	} else {
		tabs := make([]string, 0, len(m.projects))
		for i := range m.projects {
			label := projectLabel(i)
			if i == m.activeProj {
				tabs = append(tabs, tabActive.Render(label))
			} else {
				tabs = append(tabs, tabInactive.Render(label))
			}
		}
		right = strings.Join(tabs, " ")
	}

	// Tabs that don't fit are clipped rather than allowed to widen the row
	// past the terminal's actual columns — expanding remaining to fit them
	// pushed the header past m.width instead.
	rightCol := lipgloss.NewStyle().Width(remaining).MaxWidth(remaining).Align(lipgloss.Right).Render(right)
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
