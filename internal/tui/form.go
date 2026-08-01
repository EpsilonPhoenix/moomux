package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// formHintWidth and formHintLines keep every form's hint row a fixed size —
// wrapped to this width and padded to this many lines — so switching fields
// (and thus hint text) never resizes the overlay box. formHintWidth is a cap;
// on narrow terminals it shrinks to fit so the overlay never exceeds the
// screen width.
const (
	formHintWidth = 72
	formHintLines = 2
)

// overlayWidth returns the content width available to overlay forms: capped
// at max, but shrunk to fit narrow terminals (accounting for overlayBox's
// border + padding).
func (m *Model) overlayWidth(max int) int {
	w := m.width - overlayBox.GetHorizontalFrameSize()
	if w > max {
		w = max
	}
	if w < 1 {
		w = 1
	}
	return w
}

// clampInputWidth shrinks desired to fit avail, never grows it — text inputs
// keep their designed width on normal terminals and only narrow on small ones.
func clampInputWidth(desired, avail int) int {
	if avail < 1 {
		avail = 1
	}
	if desired > avail {
		return avail
	}
	return desired
}

func textInputWidth(ti *textinput.Model, desired, avail int) int {
	// textinput.Width does not include its prompt ("> " by default), so
	// reserve those cells explicitly. Its View also renders a cursor cell
	// after short values, which must fit without clipping the dialog border.
	return clampInputWidth(desired, avail-lipgloss.Width(ti.Prompt)-1)
}

// setInputWidth sets a text input's Width and re-runs its overflow
// calculation — textinput only recomputes its scroll offset on cursor/value
// changes, not when Width is set directly, so a stale offset would otherwise
// leave the old (wider) content un-truncated.
func setInputWidth(ti *textinput.Model, w int) {
	pos := ti.Position()
	ti.Width = w
	ti.SetCursor(pos)
}

// resizeFormInputs re-clamps every form's text-input widths to the current
// terminal size. Called on resize and whenever a form is (re)opened, since
// the forms are built with fixed default widths that only fit on typical
// terminals.
func (m *Model) resizeFormInputs() {
	avail := m.overlayWidth(72)

	setInputWidth(&m.nameInput, textInputWidth(&m.nameInput, 40, avail))
	setInputWidth(&m.branchInput, textInputWidth(&m.branchInput, 40, avail))
	setInputWidth(&m.ticketInput, textInputWidth(&m.ticketInput, 40, avail))

	if len(m.tagForm.inputs) == 2 {
		labels := []string{"ticket url", "pr url"}
		for i := range m.tagForm.inputs {
			w := textInputWidth(&m.tagForm.inputs[i], 48, avail-m.formLabelWidth(labels[i], 12))
			setInputWidth(&m.tagForm.inputs[i], w)
		}
	}

	labels := []string{"name", "repo", "base branch", "branch prefix"}
	projWidths := [4]int{32, 48, 24, 24}
	for i := range m.projForm.inputs {
		if i < len(projWidths) {
			labelWidth := m.formLabelWidth(labels[i], 15)
			w := textInputWidth(&m.projForm.inputs[i], projWidths[i], avail-labelWidth)
			setInputWidth(&m.projForm.inputs[i], w)
		}
	}
}

func (m *Model) formLabelWidth(label string, desired int) int {
	available := m.overlayWidth(formHintWidth)
	if available < 40 {
		// Drop alignment-only padding on narrow screens and give the value or
		// input the reclaimed cells.
		desired = lipgloss.Width(label + ":")
	}
	if desired >= available {
		desired = available - 1
	}
	if desired < 1 {
		desired = 1
	}
	return desired
}

func (m *Model) renderFormLabel(label string, desired int) string {
	width := m.formLabelWidth(label, desired)
	return muteStyle.Width(width).Render(truncateToWidth(label+":", width))
}

func (m *Model) renderFormHint(text string) string {
	// MaxHeight caps a hint long enough to wrap past formHintLines — without
	// it, that one field's hint would grow the overlay box taller than
	// every other field's, breaking the "fixed size hint row" the constants
	// above promise.
	rendered := hintStyle.Width(m.overlayWidth(formHintWidth)).MaxHeight(formHintLines).Render(text)
	for lipgloss.Height(rendered) < formHintLines {
		rendered += "\n"
	}
	return rendered
}

// newFormFieldHints gives a one-line explanation for whichever field of the
// new-session form is currently focused, so the jargon (worktree, base
// branch) doesn't have to be memorized up front.
// newFormFieldCount is the focus cycle length: name, branch, agent
// selector, ticket — matching the rendered order.
const newFormFieldCount = 4

// newFormAgentFocus is the newFormFocus value for the agent selector row.
const newFormAgentFocus = 2

var newFormFieldHints = []string{
	0: "shown in the session list and used for the worktree folder name — leave blank to derive one from the branch",
	1: "an existing branch to resume, or a new one to branch off the project's base branch",
	2: "which agent CLI runs in the session's pane — ←→ to choose",
	3: "optional — shown as a clickable ticket icon next to the session",
}

func (m *Model) renderNewForm() string {
	var b strings.Builder
	proj := ""
	if len(m.projects) > 0 {
		proj = m.projects[m.activeProj]
	}
	b.WriteString(titleStyle.Render("New session"))
	b.WriteString("\n\n")
	b.WriteString(muteStyle.Render(fmt.Sprintf("project: %s", proj)))
	b.WriteString("\n\n")
	b.WriteString(m.nameInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.branchInput.View())
	b.WriteString("\n\n")
	b.WriteString(muteStyle.Render("agent:  "))
	b.WriteString(m.renderNewFormAgentSelector())
	b.WriteString("\n\n")
	b.WriteString(m.ticketInput.View())
	return b.String()
}

func (m *Model) renderNewFormAgentSelector() string {
	if m.newFormAgentIdx < 0 {
		return warnStyle.Render("choose an agent (←→)")
	}
	var b strings.Builder
	for i, a := range agentChoices {
		if i > 0 {
			b.WriteString("  ")
		}
		if i == m.newFormAgentIdx {
			b.WriteString(titleStyle.Render("[" + a + "]"))
		} else {
			b.WriteString(muteStyle.Render(a))
		}
	}
	rendered := b.String()
	available := m.overlayWidth(formHintWidth) - lipgloss.Width("agent:  ")
	if lipgloss.Width(rendered) > available {
		return renderCompactAgentSelector(agentChoices[m.newFormAgentIdx], true, available)
	}
	return rendered
}

func renderCompactAgentSelector(agent string, focused bool, available int) string {
	selected := lipgloss.NewStyle().Bold(true)
	if focused {
		selected = titleStyle
	}
	label := "[" + agent + "]"
	if available < lipgloss.Width(label) {
		return selected.Render(truncateToWidth(label, available))
	}
	if available >= lipgloss.Width(label)+4 {
		return muteStyle.Render("‹ ") + selected.Render(label) + muteStyle.Render(" ›")
	}
	return selected.Render(label)
}

func (m *Model) renderEditSession() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit session"))
	b.WriteString("\n\n")
	b.WriteString(muteStyle.Render("project: "))
	b.WriteString(m.sessionForm.project)
	b.WriteString("\n")
	b.WriteString(muteStyle.Render("session: "))
	b.WriteString(m.sessionForm.name)
	b.WriteString("\n\n")
	b.WriteString(muteStyle.Render("agent:  "))
	b.WriteString(m.renderSessionAgentSelector())
	return b.String()
}

func (m *Model) renderSessionAgentSelector() string {
	var b strings.Builder
	for i, agent := range agentChoices {
		if i > 0 {
			b.WriteString("  ")
		}
		if i == m.sessionForm.agentIdx {
			b.WriteString(titleStyle.Render("[" + agent + "]"))
		} else {
			b.WriteString(muteStyle.Render(agent))
		}
	}
	rendered := b.String()
	available := m.overlayWidth(formHintWidth) - lipgloss.Width("agent:  ")
	if lipgloss.Width(rendered) > available {
		return renderCompactAgentSelector(agentChoices[m.sessionForm.agentIdx], true, available)
	}
	return rendered
}

// projFormFieldHints gives a one-line explanation for whichever field of the
// add-project form is currently focused (index projFormInputCount is the
// agent selector), so terms like "base branch" or "branch prefix" don't need
// to be looked up elsewhere.
var projFormFieldHints = []string{
	0: "internal label for this project — shown in the tabs at the top",
	1: "path to the project's git repo — prefilled from the current directory; edit it, or point elsewhere",
	2: "the branch new session worktrees branch off of (usually main or master)",
	3: "prepended to every new session's branch name, e.g. \"alice/\" → alice/feature-x — leave blank to skip",
	4: "coding agent launched by default for new sessions in this project — \"ask each time\" requires choosing one on every new session",
	5: "off: every session runs directly in the repo folder instead of its own worktree/branch",
}

var editProjectFieldHints = []string{
	0: "project names cannot be changed",
	1: "repository path used by new sessions — existing worktrees stay where they are",
	2: "base branch used when creating new session worktrees",
	3: "prepended to branches created for new sessions — leave blank to skip",
	4: "coding agent copied into new sessions by default — \"ask each time\" requires choosing one on every new session",
	5: "changes worktree behavior for new sessions only",
}

func (m *Model) renderNewProject() string {
	labels := []string{"name", "repo", "base branch", "branch prefix"}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Add project"))
	b.WriteString("\n\n")
	for i, ti := range m.projForm.inputs {
		b.WriteString(m.renderFormLabel(labels[i], 15))
		b.WriteString(ti.View())
		b.WriteString("\n")
	}
	b.WriteString(m.renderFormLabel("agent", 15))
	b.WriteString(m.renderAgentSelector())
	b.WriteString("\n")
	b.WriteString(m.renderFormLabel("worktrees", 15))
	b.WriteString(m.renderWorktreeToggle())
	b.WriteString("\n\n")
	return b.String()
}

func (m *Model) renderEditProject() string {
	project := m.cfg.Projects[m.editProjectName]
	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit project"))
	b.WriteString("\n\n")
	b.WriteString(m.renderFormLabel("name", 15))
	b.WriteString(m.editProjectName)
	b.WriteString(muteStyle.Render(" (fixed)"))
	b.WriteString("\n")
	b.WriteString(m.renderFormLabel("repo", 15))
	b.WriteString(m.projForm.inputs[1].View())
	b.WriteString("\n")
	if !project.IsPlain() {
		b.WriteString(m.renderFormLabel("base branch", 15))
		b.WriteString(m.projForm.inputs[2].View())
		b.WriteString("\n")
		b.WriteString(m.renderFormLabel("branch prefix", 15))
		b.WriteString(m.projForm.inputs[3].View())
		b.WriteString("\n")
	}
	b.WriteString(m.renderFormLabel("agent", 15))
	b.WriteString(m.renderAgentSelector())
	b.WriteString("\n")
	if !project.IsPlain() {
		b.WriteString(m.renderFormLabel("worktrees", 15))
		b.WriteString(m.renderWorktreeToggle())
		b.WriteString("\n")
	}
	return b.String()
}

func (m *Model) renderWorktreeToggle() string {
	focused := m.projForm.focus == projFormInputCount+1
	choice := "on"
	if m.projForm.noWorktree {
		choice = "off"
	}
	label := "[" + choice + "]"
	if focused {
		return titleStyle.Render(label)
	}
	return lipgloss.NewStyle().Bold(true).Render(label)
}

// projectAgentChoices is the project form's agent selector: the real agents,
// plus a trailing "ask each time" entry (index askAgentIdx maps to it) that
// defers the choice to each new-session form instead of fixing one here.
var projectAgentChoices = append(append([]string{}, agentChoices...), "ask each time")

func (m *Model) renderAgentSelector() string {
	focused := m.projForm.focus == projFormInputCount
	selectedIdx := m.projForm.agentIdx
	if selectedIdx == askAgentIdx {
		selectedIdx = len(agentChoices)
	}
	var b strings.Builder
	for i, a := range projectAgentChoices {
		if i > 0 {
			b.WriteString("  ")
		}
		if i == selectedIdx {
			if focused {
				b.WriteString(titleStyle.Render("[" + a + "]"))
			} else {
				b.WriteString(lipgloss.NewStyle().Bold(true).Render("[" + a + "]"))
			}
		} else {
			b.WriteString(muteStyle.Render(a))
		}
	}
	rendered := b.String()
	available := m.overlayWidth(formHintWidth) - m.formLabelWidth("agent", 15)
	if lipgloss.Width(rendered) > available {
		return renderCompactAgentSelector(projectAgentChoices[selectedIdx], focused, available)
	}
	return rendered
}

// tagFormFieldHints gives a one-line explanation for whichever field of the
// tag form is currently focused, matching the other forms' contextual hints.
var tagFormFieldHints = []string{
	0: "shown as a clickable ticket icon next to the session",
	1: "shown as a clickable PR icon next to the session",
}

func (m *Model) renderTagForm() string {
	labels := []string{"ticket url", "pr url"}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Tag session"))
	b.WriteString("\n\n")
	for i, ti := range m.tagForm.inputs {
		b.WriteString(m.renderFormLabel(labels[i], 12))
		b.WriteString(ti.View())
		b.WriteString("\n\n")
	}
	return b.String()
}

func (m *Model) renderProjectInitChoice() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Path is not a git repository"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("path: %s\n", m.pending.p.Repo))
	if w := tccWarning(m.pending.p.Repo); w != "" {
		b.WriteString(warnStyle.Width(64).Render(w))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString("How should moomux set this up?\n\n")
	b.WriteString("  i  ")
	b.WriteString(muteStyle.Render("init a new git repo here (mkdir + git init + empty commit)"))
	b.WriteString("\n")
	b.WriteString("  s  ")
	b.WriteString(muteStyle.Render("skip git — manage as a plain folder, no worktrees/branches"))
	b.WriteString("\n\n")
	b.WriteString(muteStyle.Render("b/esc to go back to the form"))
	return b.String()
}

func (m *Model) renderConfirmDeleteProject() string {
	if len(m.projects) == 0 {
		return ""
	}
	name := m.projects[m.activeProj]
	var b strings.Builder
	b.WriteString(dangerStyle.Render("Remove project?"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("name: %s\n", name))
	b.WriteString("\n")
	b.WriteString(muteStyle.Render("This only removes the entry from config."))
	b.WriteString("\n")
	b.WriteString(muteStyle.Render("Worktrees and the repo itself are untouched."))
	b.WriteString("\n\n")
	b.WriteString("y to confirm   n/esc to cancel")
	return b.String()
}
