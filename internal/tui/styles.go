package tui

import "github.com/charmbracelet/lipgloss"

var (
	colFg         = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#e6e6e6"}
	colMute       = lipgloss.AdaptiveColor{Light: "#5b5b66", Dark: "#7a7a85"}
	colAccent     = lipgloss.AdaptiveColor{Light: "#2952cc", Dark: "#7aa2f7"}
	colWorking    = lipgloss.AdaptiveColor{Light: "#4b7a1f", Dark: "#9ece6a"}
	colDone       = lipgloss.AdaptiveColor{Light: "#946f1a", Dark: "#e0af68"}
	colNeedsInput = lipgloss.AdaptiveColor{Light: "#9c3ba1", Dark: "#bb70d2"}
	colParked     = lipgloss.AdaptiveColor{Light: "#7d7d85", Dark: "#565a6e"}
	colDanger     = lipgloss.AdaptiveColor{Light: "#c0293f", Dark: "#f7768e"}
	colBorder     = lipgloss.AdaptiveColor{Light: "#9a9aa5", Dark: "#2d2f3a"}
	colSelBg      = lipgloss.AdaptiveColor{Light: "#a9bdf0", Dark: "#2f395e"}

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	cowStyle   = lipgloss.NewStyle().Foreground(colMute)
	muteStyle  = lipgloss.NewStyle().Foreground(colMute)
	tabActive  = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Padding(0, 1)

	listRow         = lipgloss.NewStyle().Padding(0, 1)
	listRowSelected = lipgloss.NewStyle().Padding(0, 1).Background(colSelBg).Foreground(colFg).Bold(true)

	panelBorder = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colBorder).
			Padding(0, 1)

	footerStyle = lipgloss.NewStyle().Foreground(colMute).Padding(0, 1)

	infoFlashStyle  = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	errorFlashStyle = lipgloss.NewStyle().Foreground(colDanger).Bold(true)

	dotWorkingStyle    = lipgloss.NewStyle().Foreground(colWorking)
	dotDoneStyle       = lipgloss.NewStyle().Foreground(colDone)
	dotNeedsInputStyle = lipgloss.NewStyle().Foreground(colNeedsInput)
	dotParkedStyle     = lipgloss.NewStyle().Foreground(colParked)
	dotWorking         = dotWorkingStyle.Render("⬤")
	dotDone            = dotDoneStyle.Render("⬤")
	dotNeedsInput      = dotNeedsInputStyle.Render("⬤")
	dotParked          = dotParkedStyle.Render("⬤")

	iconTicketStyle = lipgloss.NewStyle().Foreground(colMute)
	iconPRStyle     = lipgloss.NewStyle().Foreground(colMute)
	iconTicket      = iconTicketStyle.Render("🎫")
	iconPR          = iconPRStyle.Render("🔀")
	detailLinkStyle = lipgloss.NewStyle().Foreground(colAccent).Underline(true)

	overlayBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(1, 2)

	dangerStyle = lipgloss.NewStyle().Foreground(colDanger).Bold(true)
	warnStyle   = lipgloss.NewStyle().Foreground(colDone).Bold(true)

	// hintStyle is the contextual, per-field explainer shown in forms —
	// italic to read as a transient tip rather than a persistent label.
	hintStyle = lipgloss.NewStyle().Foreground(colMute).Italic(true)

	// help overlay
	helpGroupStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	helpKeyStyle   = lipgloss.NewStyle().Bold(true).Foreground(colFg)
	helpDescStyle  = lipgloss.NewStyle().Foreground(colMute)
)
