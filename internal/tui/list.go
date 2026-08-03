package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

// linkHit records where a clickable ticket/PR icon landed within the
// rendered list, in the list panel's own local coordinates (line index and
// column range on that line). The TUI translates these to absolute terminal
// coordinates once the surrounding layout (header height, panel border) is
// known, so a click can be matched back to the URL to open.
type linkHit struct {
	sessionID  string
	url        string
	copyOnly   bool // force clipboard copy instead of browser.Open (e.g. a tmux command, not a URL)
	line       int
	col0, col1 int // half-open column range
}

func (m *Model) renderList(width, height int) (string, []linkHit) {
	var b strings.Builder
	title := "SESSIONS"
	empty := "  no sessions — press n to create"
	if m.allSessions {
		title = "ALL SESSIONS"
		empty = "  no sessions — press n to create"
	}
	if len(m.projects) == 0 {
		empty = "  no projects yet — press / then n to add one"
	} else if m.showArchived {
		title = "ARCHIVED"
		if m.allSessions {
			title = "ALL ARCHIVED"
		}
		empty = "  no archived sessions"
	} else if !m.allSessions {
		if n := m.archivedCount(); n > 0 {
			title += superscript(n)
		}
	}
	// On small screens the title (plus its blank line beneath) costs two
	// rows that are worth more as extra visible sessions than as a label.
	// ARCHIVED is exempt: it's the only signal telling the archived view
	// apart from the normal one, so hiding it would leave archived sessions
	// looking indistinguishable from active ones.
	compact := m.compactScreen() && !m.showArchived
	titleRows := 0
	if !compact {
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n\n")
		titleRows = 2
	}
	if len(m.sessions) == 0 {
		b.WriteString(muteStyle.Render(empty))
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(b.String()), nil
	}
	visible := height - titleRows
	if visible < 1 {
		visible = 1
	}
	start := 0
	if len(m.sessions) > visible {
		start = m.cursor - visible/2
		if start < 0 {
			start = 0
		}
		if max := len(m.sessions) - visible; start > max {
			start = max
		}
	}
	end := start + visible
	if end > len(m.sessions) {
		end = len(m.sessions)
	}
	var hits []linkHit
	for i := start; i < end; i++ {
		s := m.sessions[i]
		selected := i == m.cursor
		projectLabel := ""
		if m.allSessions {
			projectLabel = m.projectEmoji(s.Project)
		}
		row, rowHits := renderRow(s, m.effectiveState(s), width-4, selected, projectLabel)
		for _, h := range rowHits {
			h.sessionID = s.ID
			// titleRows lines for the "SESSIONS" title and blank line above
			// (0 on short terminals, where it's hidden); +1 column for the
			// row style's own left padding.
			h.line = titleRows + (i - start)
			h.col0++
			h.col1++
			hits = append(hits, h)
		}
		if selected {
			row = listRowSelected.Render(row)
		} else {
			row = listRow.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(b.String()), hits
}

func renderRow(s session.Session, st watcher.State, width int, selected bool, projectLabel string) (string, []linkHit) {
	dotStyle := dotParkedStyle
	switch st {
	case watcher.Working:
		dotStyle = dotWorkingStyle
	case watcher.Done:
		dotStyle = dotDoneStyle
	case watcher.NeedsInput:
		dotStyle = dotNeedsInputStyle
	}
	iconTicketStyle, iconPRStyle := iconTicketStyle, iconPRStyle
	if selected {
		dotStyle = dotStyle.Background(colSelBg)
		iconTicketStyle = iconTicketStyle.Background(colSelBg)
		iconPRStyle = iconPRStyle.Background(colSelBg)
	}
	dot := dotStyle.Render("⬤")
	var icons string
	var hits []linkHit
	col := 0
	addIcon := func(style lipgloss.Style, glyph, url string) {
		w := lipgloss.Width(glyph)
		hits = append(hits, linkHit{url: url, col0: col, col1: col + w})
		icons += style.Render(glyph) + style.Render(" ")
		col += w + 1
	}
	if s.Ticket != "" {
		addIcon(iconTicketStyle, "🎫", s.Ticket)
	}
	if s.PR != "" {
		addIcon(iconPRStyle, "🔀", s.PR)
	}
	suffix := icons + dot
	nameWidth := width - 1 - lipgloss.Width(suffix)
	if nameWidth < 4 {
		nameWidth = 4
	}
	var prefix string
	if projectLabel != "" {
		// Budget at most half the name column for the project tag, so a long
		// project name can't crowd the session name down to nothing.
		labelWidth := nameWidth / 2
		if labelWidth > lipgloss.Width(projectLabel)+1 {
			labelWidth = lipgloss.Width(projectLabel) + 1
		}
		if labelWidth > 0 {
			label := truncate(projectLabel, labelWidth-1)
			// Width() (not fmt's %-*s, which pads by rune count) pads to the
			// true terminal column budget — emoji tags are 1 rune but 2
			// display columns, so rune-count padding under-budgeted by a
			// column and threw off every position after it, including the
			// selected-row background.
			style := muteStyle.Width(labelWidth)
			if selected {
				style = style.Background(colSelBg)
			}
			prefix = style.Render(label)
			nameWidth -= labelWidth
			if nameWidth < 4 {
				nameWidth = 4
			}
		}
	}
	// The name text needs its own explicit style (not just reliance on the
	// caller's outer Background wrap): when a prefix is present, its Render()
	// call emits a closing ANSI reset that would otherwise wipe out any
	// color set before it, leaving this plain text with no background at
	// all — the exact bug behind the selected-row highlight vanishing once
	// a project-emoji prefix was introduced.
	nameStyle := lipgloss.NewStyle()
	sepStyle := lipgloss.NewStyle()
	if selected {
		nameStyle = nameStyle.Background(colSelBg).Foreground(colFg).Bold(true)
		sepStyle = sepStyle.Background(colSelBg)
	}
	name := prefix + nameStyle.Render(fmt.Sprintf("%-*s", nameWidth, truncate(s.Name, nameWidth)))
	offset := nameWidth + lipgloss.Width(prefix) + 1
	for i := range hits {
		hits[i].col0 += offset
		hits[i].col1 += offset
	}
	return name + sepStyle.Render(" ") + suffix, hits
}

// projectEmojiPalette is the fallback set for projects that haven't chosen
// their own emoji (config.Project.Emoji) — picked deterministically per
// project name so the same project always gets the same glyph.
var projectEmojiPalette = []string{"🐙", "🦊", "🚀", "🔥", "🌊", "🍀", "⚡", "🎯", "🐝", "🦉"}

// projectEmojiChoices is the project-form emoji selector's cycle order:
// "auto" (index 0, the deterministic-pick sentinel) followed by the palette.
var projectEmojiChoices = append([]string{"auto"}, projectEmojiPalette...)

// projectEmojiFieldValue converts a projectForm.emojiIdx into the
// config.Project.Emoji value to store: "" for auto (idx 0), else the picked
// glyph — from choices (normally projectEmojiChoices, but editProjectForm
// may have inserted the project's existing out-of-palette emoji into it).
func projectEmojiFieldValue(choices []string, idx int) string {
	if idx <= 0 || idx >= len(choices) {
		return ""
	}
	return choices[idx]
}

func cycleProjectEmojiIdx(choices []string, idx, delta int) int {
	n := len(choices)
	return (idx + delta + n) % n
}

// projectEmoji returns the project's configured emoji, falling back to a
// deterministic pick from projectEmojiPalette if none is set.
func (m *Model) projectEmoji(project string) string {
	if e := m.cfg.Projects[project].Emoji; e != "" {
		return e
	}
	var h int
	for _, r := range project {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return projectEmojiPalette[h%len(projectEmojiPalette)]
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	if n < 2 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

// truncateLeft keeps the end of a value visible. It is useful for read-only
// paths, where the directory or branch name at the right is usually more
// informative than a common leading prefix.
func truncateLeft(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 2 {
		return string(r[len(r)-n:])
	}
	return "…" + string(r[len(r)-(n-1):])
}
