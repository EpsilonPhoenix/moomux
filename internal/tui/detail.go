package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/erickgnclvs/moomux/internal/watcher"
)

func (m *Model) renderDetail(width, height int) (string, []linkHit) {
	var b strings.Builder
	if !m.compactScreen() {
		b.WriteString(titleStyle.Render("DETAIL"))
		b.WriteString("\n\n")
	}
	if len(m.sessions) == 0 {
		b.WriteString(muteStyle.Render("nothing selected"))
		return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(b.String()), nil
	}
	s := m.sessions[m.cursor]
	st := m.effectiveState(s)
	dot := dotParked
	label := "parked"
	switch st {
	case watcher.Working:
		dot, label = dotWorking, "working"
	case watcher.Waiting:
		dot, label = dotWaiting, "waiting"
	}
	var hits []linkHit
	row := func(k, v, url string) {
		// Measure the rendered height, not logical newlines: the final
		// Width(width) render *wraps* long rows rather than clipping, and a
		// wrapped row above would shift every later hitbox down.
		line := lipgloss.Height(lipgloss.NewStyle().Width(width).Render(b.String())) - 1
		key := muteStyle.Render(fmt.Sprintf("%-10s", k+":"))
		if url != "" {
			col0 := lipgloss.Width(key) + 1
			col1 := min(width, col0+lipgloss.Width(v))
			// MaxHeight/MaxWidth below clips the rendered detail. Do not leave
			// invisible link targets behind in footer or border coordinates.
			if line < height && col0 < col1 {
				hits = append(hits, linkHit{
					sessionID: s.ID,
					url:       url,
					line:      line,
					col0:      col0,
					col1:      col1,
				})
			}
			v = detailLinkStyle.Render(v)
		}
		b.WriteString(fmt.Sprintf("%s %s\n", key, v))
	}
	valueWidth := width - 14
	if valueWidth < 8 {
		valueWidth = 8
	}
	row("status", dot+"  "+label, "")
	if s.Archived {
		row("archived", "yes", "")
	}
	row("agent", s.AgentName(), "")
	row("name", truncate(s.Name, valueWidth), "")
	row("worktree", truncateLeft(s.WorktreePath, valueWidth), "")
	row("tmux", truncate(s.TmuxSession, valueWidth), "")
	row("created", humanizeAge(time.Since(s.CreatedAt)), "")
	if s.Ticket != "" {
		row("ticket", truncateLeft(s.Ticket, valueWidth), s.Ticket)
	}
	if s.PR != "" {
		row("pr", truncateLeft(s.PR, valueWidth), s.PR)
	}
	if prompt := m.prompts[s.ID]; prompt != "" {
		oneline := strings.ReplaceAll(strings.ReplaceAll(prompt, "\r\n", " "), "\n", " ")
		row("prompt", truncate(oneline, valueWidth), "")
	}
	b.WriteString("\n")
	var cowMsg string
	switch st {
	case watcher.Working:
		cowMsg = pickQuip(s.ID, quipsWorking)
	case watcher.Waiting:
		cowMsg = pickQuip(s.ID, quipsWaiting)
	default:
		cowMsg = pickQuip(s.ID, quipsParked)
	}
	b.WriteString(cowStyle.Render(cowsay(cowMsg, valueWidth+10, st)))
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(b.String()), hits
}

func cowsay(msg string, maxWidth int, st watcher.State) string {
	const lineMax = 38
	w := lineMax
	if maxWidth > 0 && maxWidth < w {
		w = maxWidth
	}
	lines := wrapLines(msg, w)
	// cap at 4 lines, truncate last with ellipsis
	if len(lines) > 4 {
		lines = lines[:4]
		r := []rune(lines[3])
		if len(r) > w-1 {
			r = r[:w-1]
		}
		lines[3] = string(r) + "…"
	}
	border := strings.Repeat("_", w+2)
	var b strings.Builder
	b.WriteString(" " + border + "\n")
	for i, l := range lines {
		pad := w - len([]rune(l))
		padded := l + strings.Repeat(" ", pad)
		switch {
		case len(lines) == 1:
			b.WriteString("< " + padded + " >\n")
		case i == 0:
			b.WriteString("/ " + padded + " \\\n")
		case i == len(lines)-1:
			b.WriteString("\\ " + padded + " /\n")
		default:
			b.WriteString("| " + padded + " |\n")
		}
	}
	b.WriteString(" " + strings.Repeat("-", w+2) + "\n")
	var eyes string
	switch st {
	case watcher.Working:
		eyes = "**"
	case watcher.Waiting:
		eyes = "oo"
	default:
		eyes = "--"
	}
	b.WriteString(`        \   ^__^` + "\n")
	b.WriteString("         \\  (" + eyes + `)\_______` + "\n")
	b.WriteString(`            (__)\       )\/\` + "\n")
	b.WriteString(`                ||----w |` + "\n")
	b.WriteString(`                ||     ||`)
	return b.String()
}

func wrapLines(s string, width int) []string {
	s = strings.ReplaceAll(s, "\r", "")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		runes := []rune(strings.TrimSpace(line))
		for len(runes) > width {
			cut := width
			for i := width - 1; i > width-15 && i > 0; i-- {
				if runes[i] == ' ' {
					cut = i
					break
				}
			}
			out = append(out, string(runes[:cut]))
			runes = []rune(strings.TrimLeft(string(runes[cut:]), " "))
		}
		if len(runes) > 0 {
			out = append(out, string(runes))
		}
	}
	return out
}

func humanizeAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hr ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
