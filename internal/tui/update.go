package tui

import (
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/erickgnclvs/moomux/internal/browser"
	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/gitwt"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeFormInputs()
		// Re-reveal the focused control after the usable viewport changes,
		// including when a mobile keyboard opens or closes.
		m.overlayFocus = -1
		return m, nil

	case StatusTickMsg:
		for path, st := range msg.Snap.States {
			m.states[path] = st
		}
		m.refreshSessions()
		if msg.Snap.Err != nil {
			// Surface once rather than re-flashing on every subsequent tick
			// while the same failure persists.
			warning := "status scan warning: " + msg.Snap.Err.Error()
			if m.flash != warning {
				m.flash = warning
				m.flashTime = time.Now()
			}
		}
		return m, tea.Batch(listenStatus(m.statusCh), refreshStatusCmd(m))

	case StatusRefreshedMsg:
		m.tmuxAlive = msg.TmuxAlive
		for id, p := range msg.Prompts {
			if m.prompts[id] == "" {
				m.prompts[id] = p
			}
		}
		return m, nil

	case StatusChannelClosedMsg:
		m.flash = "status watcher stopped"
		m.flashTime = time.Now()
		return m, nil

	case TmuxKilledMsg:
		m.setFlash("info", "parked")
		return m, refreshStatusCmd(m)

	case InfoMsg:
		if m.busy {
			return m, tickFlash()
		}
		dur := infoFlashDuration
		if m.flashKind == "error" {
			dur = errorFlashDuration
		}
		if !m.flashTime.IsZero() && time.Since(m.flashTime) > dur {
			m.flash = ""
			m.flashKind = ""
		}
		return m, tickFlash()

	case ErrorMsg:
		m.busy = false
		m.setError(msg.Err)
		return m, nil

	case SessionCreatedMsg:
		m.busy = false
		text := "created " + msg.Session.Name
		if msg.Hint != "" {
			text += " — " + msg.Hint
		}
		m.setFlash("info", text)
		// Remove from prompt cache so the next tick scans the new session.
		delete(m.prompts, msg.Session.ID)
		m.refreshSessions()
		return m, refreshStatusCmd(m)

	case SessionDeletedMsg:
		m.setFlash("info", "deleted")
		m.refreshSessions()
		return m, refreshStatusCmd(m)

	case SessionArchivedMsg:
		if msg.Err != nil {
			m.setError(msg.Err)
			return m, nil
		}
		if msg.Archived {
			m.setFlash("info", "archived")
		} else {
			m.setFlash("info", "restored")
		}
		m.refreshSessions()
		return m, nil

	case SessionTaggedMsg:
		m.setFlash("info", "tagged "+msg.Session.Name)
		m.refreshSessions()
		return m, nil

	case SessionAgentUpdatedMsg:
		m.busy = false
		if msg.Err != nil {
			m.sessionForm.err = msg.Err.Error()
			m.mode = ModeEditSession
			return m, nil
		}
		m.refreshSessions()
		for i, s := range m.sessions {
			if s.ID == msg.Session.ID {
				m.cursor = i
				break
			}
		}
		delete(m.prompts, msg.Session.ID)
		m.mode = ModeList
		m.setFlash("info", "updated session "+msg.Session.Name)
		return m, refreshStatusCmd(m)

	case SessionMovedMsg:
		if msg.Err != nil {
			m.setError(msg.Err)
			return m, nil
		}
		m.refreshSessions()
		for i, s := range m.sessions {
			if s.ID == msg.ID {
				m.cursor = i
				break
			}
		}
		return m, nil

	case SessionOpenedMsg:
		text := "opened " + msg.ID
		if msg.Hint != "" {
			text += " — " + msg.Hint
		}
		m.setFlash("info", text)
		return m, nil

	case ProjectAddedMsg:
		switch msg.Kind {
		case "add":
			if msg.Err == nil {
				m.activateProject(msg.Name)
				m.mode = ModeList
				m.setFlash("info", "added project "+msg.Name)
				return m, nil
			}
			if errors.Is(msg.Err, gitwt.ErrNotGitRepo) {
				m.pending = pendingProject{name: msg.Name, p: msg.Project}
				m.mode = ModeProjectInitChoice
				m.resetOverlayViewport()
				return m, nil
			}
			m.projForm.err = msg.Err.Error()
			return m, nil
		case "init":
			if msg.Err != nil {
				m.mode = ModeNewProject
				m.projForm.err = msg.Err.Error()
				return m, nil
			}
			m.activateProject(msg.Name)
			m.mode = ModeList
			m.setFlash("info", "initialized git repo + added "+msg.Name)
			return m, nil
		case "plain":
			if msg.Err != nil {
				m.mode = ModeNewProject
				m.projForm.err = msg.Err.Error()
				return m, nil
			}
			m.activateProject(msg.Name)
			m.mode = ModeList
			m.setFlash("info", "added plain (non-git) project "+msg.Name)
			return m, nil
		}
		return m, nil

	case ProjectMovedMsg:
		if msg.Err != nil {
			m.setError(msg.Err)
			return m, nil
		}
		m.refreshProjects()
		for i, n := range m.projects {
			if n == msg.Name {
				m.activeProj = i
				break
			}
		}
		return m, nil

	case ProjectUpdatedMsg:
		m.busy = false
		if msg.Err != nil {
			m.projForm.err = msg.Err.Error()
			m.mode = ModeEditProject
			return m, nil
		}
		m.activateProject(msg.Name)
		m.mode = ModeList
		m.setFlash("info", "updated project "+msg.Name)
		return m, nil

	case ProjectRemovedMsg:
		if msg.Err != nil {
			m.mode = ModeList
			m.setFlash("error", msg.Err.Error())
			return m, nil
		}
		m.refreshProjects()
		m.cursor = 0
		m.refreshSessions()
		m.mode = ModeList
		m.setFlash("info", "removed project "+msg.Name)
		return m, nil

	case LinkOpenedMsg:
		m.setFlash("info", "opened "+msg.URL)
		return m, nil

	case tea.MouseMsg:
		if m.mode != ModeList {
			var cmd tea.Cmd
			m.overlayViewport, cmd = m.overlayViewport.Update(msg)
			return m, cmd
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if url := m.linkAt(msg.X, msg.Y); url != "" {
				if m.isRemote() {
					// Over SSH, `open` launches a browser on the remote
					// machine, not the phone/laptop the user is actually
					// looking at — and moomux's mouse tracking means the
					// terminal never gets a chance to handle the tap as a
					// link itself. Copy the URL instead: OSC 52 isn't tied
					// to mouse mode, so it reaches the client's clipboard
					// regardless.
					//
					// This runs synchronously (not as a tea.Cmd) because a
					// Cmd executes in its own goroutine, concurrently with
					// bubbletea's render loop — both writing to os.Stdout at
					// once can interleave and corrupt the escape sequence
					// before the terminal ever sees a well-formed one.
					if err := browser.Copy(url); err != nil {
						m.setError(err)
						return m, nil
					}
					m.setFlash("info", "copied "+url)
					return m, nil
				}
				return m, func() tea.Msg {
					if err := browser.Open(url); err != nil {
						return ErrorMsg{Err: err}
					}
					return LinkOpenedMsg{URL: url}
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.mode != ModeList && m.mode != ModeHelp && isOverlayScrollKey(msg) {
			var cmd tea.Cmd
			m.overlayViewport, cmd = m.overlayViewport.Update(msg)
			return m, cmd
		}
		switch m.mode {
		case ModeNewForm:
			return m.updateNewForm(msg)
		case ModeConfirmDelete:
			return m.updateConfirm(msg)
		case ModeNewProject:
			return m.updateNewProject(msg)
		case ModeConfirmDeleteProject:
			return m.updateConfirmDeleteProject(msg)
		case ModeProjectInitChoice:
			return m.updateProjectInitChoice(msg)
		case ModeTagForm:
			return m.updateTagForm(msg)
		case ModeHelp:
			return m.updateHelp(msg)
		case ModeEditSession:
			return m.updateEditSession(msg)
		case ModeEditProject:
			return m.updateEditProject(msg)
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func isOverlayScrollKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	default:
		return false
	}
}

func (m *Model) resetOverlayViewport() {
	m.overlayViewport.GotoTop()
	m.overlayMode = ModeList
	m.overlayFocus = -1
}

func (m *Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.cancelPoll()
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.mode = ModeHelp
		m.resetOverlayViewport()
		return m, nil
	case key.Matches(msg, m.keys.Up):
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor - 1 + len(m.sessions)) % len(m.sessions)
		}
	case key.Matches(msg, m.keys.Down):
		if len(m.sessions) > 0 {
			m.cursor = (m.cursor + 1) % len(m.sessions)
		}
	case key.Matches(msg, m.keys.MoveUp):
		if len(m.sessions) > 0 && m.cursor > 0 {
			id := m.sessions[m.cursor].ID
			return m, func() tea.Msg {
				if err := m.backend.MoveSession(id, -1); err != nil {
					return SessionMovedMsg{ID: id, Err: err}
				}
				return SessionMovedMsg{ID: id}
			}
		}
	case key.Matches(msg, m.keys.MoveDown):
		if len(m.sessions) > 0 && m.cursor < len(m.sessions)-1 {
			id := m.sessions[m.cursor].ID
			return m, func() tea.Msg {
				if err := m.backend.MoveSession(id, 1); err != nil {
					return SessionMovedMsg{ID: id, Err: err}
				}
				return SessionMovedMsg{ID: id}
			}
		}
	case key.Matches(msg, m.keys.MoveProjLeft):
		if len(m.projects) > 0 && m.activeProj > 0 {
			name := m.projects[m.activeProj]
			return m, func() tea.Msg {
				if err := m.backend.MoveProject(name, -1); err != nil {
					return ProjectMovedMsg{Name: name, Err: err}
				}
				return ProjectMovedMsg{Name: name}
			}
		}
	case key.Matches(msg, m.keys.MoveProjRight):
		if len(m.projects) > 0 && m.activeProj < len(m.projects)-1 {
			name := m.projects[m.activeProj]
			return m, func() tea.Msg {
				if err := m.backend.MoveProject(name, 1); err != nil {
					return ProjectMovedMsg{Name: name, Err: err}
				}
				return ProjectMovedMsg{Name: name}
			}
		}
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.NextProject):
		if len(m.projects) > 0 {
			m.activeProj = (m.activeProj + 1) % len(m.projects)
			m.cursor = 0
			m.refreshSessions()
		}
	case key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.PrevProject):
		if len(m.projects) > 0 {
			m.activeProj = (m.activeProj - 1 + len(m.projects)) % len(m.projects)
			m.cursor = 0
			m.refreshSessions()
		}
	case key.Matches(msg, m.keys.Refresh):
		m.refreshSessions()
		return m, refreshStatusCmd(m)
	case key.Matches(msg, m.keys.RemoteLinks):
		m.forceCopyLinks = !m.forceCopyLinks
		state := "auto"
		if m.forceCopyLinks {
			state = "forced on"
		}
		m.setFlash("info", "ticket/PR links copy instead of open: "+state)
		return m, nil
	case key.Matches(msg, m.keys.Kill):
		if len(m.sessions) > 0 {
			id := m.sessions[m.cursor].ID
			return m, func() tea.Msg {
				if err := m.backend.KillTmux(id); err != nil {
					return ErrorMsg{Err: err}
				}
				return TmuxKilledMsg{ID: id}
			}
		}
	case key.Matches(msg, m.keys.New):
		if len(m.projects) == 0 {
			return m.flashError(fmt.Errorf("no projects configured — press P to add one"))
		}
		m.mode = ModeNewForm
		m.newFormErr = ""
		m.newFormFocus = 0
		m.nameInput.SetValue("")
		m.nameInput.Focus()
		m.branchInput.SetValue("")
		m.branchInput.Blur()
		m.ticketInput.SetValue("")
		m.ticketInput.Blur()
		m.newFormFocus = 0
		m.resetOverlayViewport()
		m.resizeFormInputs()
		// pre-select the project's default agent, unless it requires an
		// explicit choice on every session
		proj := m.projects[m.activeProj]
		p := m.cfg.Projects[proj]
		if p.PromptAgent {
			m.newFormAgentIdx = -1
		} else {
			m.newFormAgentIdx = agentChoiceIndex(p.AgentName())
		}
	case key.Matches(msg, m.keys.Delete):
		if len(m.sessions) > 0 {
			m.mode = ModeConfirmDelete
			// Without this the dialog inherits the previous overlay's scroll
			// offset and can open with the "what you're deleting" text
			// scrolled off-screen, leaving only "y to confirm" visible.
			m.resetOverlayViewport()
		}
	case key.Matches(msg, m.keys.Archive):
		if len(m.sessions) > 0 {
			id := m.sessions[m.cursor].ID
			archive := !m.showArchived
			return m, func() tea.Msg {
				if _, err := m.backend.SetSessionArchived(id, archive); err != nil {
					return SessionArchivedMsg{ID: id, Archived: archive, Err: err}
				}
				return SessionArchivedMsg{ID: id, Archived: archive}
			}
		}
	case key.Matches(msg, m.keys.ShowArchived):
		m.showArchived = !m.showArchived
		m.cursor = 0
		m.refreshSessions()
	case key.Matches(msg, m.keys.Tag):
		if len(m.sessions) > 0 {
			s := m.sessions[m.cursor]
			m.mode = ModeTagForm
			m.tagForm = newTagForm(s.Ticket, s.PR)
			m.resetOverlayViewport()
			m.resizeFormInputs()
		}
	case key.Matches(msg, m.keys.EditSession):
		if len(m.sessions) > 0 {
			s := m.sessions[m.cursor]
			m.mode = ModeEditSession
			m.resetOverlayViewport()
			m.sessionForm = sessionForm{
				id:       s.ID,
				project:  s.Project,
				name:     s.Name,
				agentIdx: agentChoiceIndex(s.AgentName()),
			}
		}
	case key.Matches(msg, m.keys.NewProject):
		m.mode = ModeNewProject
		m.projForm = newProjectForm()
		m.resetOverlayViewport()
		m.resizeFormInputs()
		return m, nil
	case key.Matches(msg, m.keys.EditProject):
		if len(m.projects) == 0 {
			return m.flashError(fmt.Errorf("no projects to edit"))
		}
		name := m.projects[m.activeProj]
		m.editProjectName = name
		m.projForm = editProjectForm(name, m.cfg.Projects[name])
		m.mode = ModeEditProject
		m.resetOverlayViewport()
		m.resizeFormInputs()
		return m, nil
	case key.Matches(msg, m.keys.DelProject):
		if len(m.projects) == 0 {
			return m.flashError(fmt.Errorf("no projects to remove"))
		}
		if n := m.projectSessionCount(); n > 0 {
			return m.flashError(fmt.Errorf("%s has %d session(s) (incl. archived) — delete them first", m.projects[m.activeProj], n))
		}
		m.mode = ModeConfirmDeleteProject
		m.resetOverlayViewport()
		return m, nil
	case key.Matches(msg, m.keys.Open):
		if len(m.sessions) > 0 {
			id := m.sessions[m.cursor].ID
			return m, func() tea.Msg {
				hint, err := m.backend.OpenSession(id)
				if err != nil {
					return ErrorMsg{Err: err}
				}
				return SessionOpenedMsg{ID: id, Hint: hint}
			}
		}
	}
	return m, nil
}

// updateHelp handles keys while the help overlay is open. Any of ?, esc, or q
// dismisses it; ctrl+c still quits the app outright.
func (m *Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		m.cancelPoll()
		return m, tea.Quit
	}
	switch {
	case key.Matches(msg, m.keys.Help), key.Matches(msg, m.keys.Cancel), key.Matches(msg, m.keys.Quit):
		m.mode = ModeList
		return m, nil
	}
	var cmd tea.Cmd
	m.overlayViewport, cmd = m.overlayViewport.Update(msg)
	return m, cmd
}

func (m *Model) updateNewForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.FormDown), key.Matches(msg, m.keys.FormUp):
		m.newFormBlurAll()
		if key.Matches(msg, m.keys.ShiftTab) || key.Matches(msg, m.keys.FormUp) {
			m.newFormFocus = (m.newFormFocus - 1 + newFormFieldCount) % newFormFieldCount
		} else {
			m.newFormFocus = (m.newFormFocus + 1) % newFormFieldCount
		}
		m.newFormFocusInput()
		return m, nil
	case key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Right):
		// Only steer the agent selector when it's the focused field;
		// otherwise the arrows belong to the text input's cursor.
		if m.newFormFocus == newFormAgentFocus {
			if m.newFormAgentIdx < 0 {
				m.newFormAgentIdx = 0
			} else if key.Matches(msg, m.keys.Left) {
				m.newFormAgentIdx = (m.newFormAgentIdx - 1 + len(agentChoices)) % len(agentChoices)
			} else {
				m.newFormAgentIdx = (m.newFormAgentIdx + 1) % len(agentChoices)
			}
			return m, nil
		}
	case key.Matches(msg, m.keys.Enter):
		name := m.nameInput.Value()
		branch := m.branchInput.Value()
		ticket := m.ticketInput.Value()
		if name == "" && branch == "" {
			m.newFormErr = "enter a session name or an existing branch"
			return m, nil
		}
		if m.newFormAgentIdx < 0 {
			m.newFormErr = "this project requires choosing an agent — tab to the agent row, then ←→"
			return m, nil
		}
		m.newFormErr = ""
		proj := m.projects[m.activeProj]
		agent := agentChoices[m.newFormAgentIdx]
		m.mode = ModeList
		label := name
		if label == "" {
			label = branch
		}
		m.setFlash("info", "creating "+label+"…")
		m.busy = true
		return m, func() tea.Msg {
			s, hint, err := m.backend.CreateSession(proj, name, agent, branch, ticket)
			if err != nil {
				return ErrorMsg{Err: err}
			}
			return SessionCreatedMsg{Session: s, Hint: hint}
		}
	}
	var cmd tea.Cmd
	switch m.newFormFocus {
	case 1:
		m.branchInput, cmd = m.branchInput.Update(msg)
	case newFormAgentFocus:
		// selector row: no text input to type into
	case 3:
		m.ticketInput, cmd = m.ticketInput.Update(msg)
	default:
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) newFormBlurAll() {
	m.nameInput.Blur()
	m.branchInput.Blur()
	m.ticketInput.Blur()
}

func (m *Model) newFormFocusInput() {
	switch m.newFormFocus {
	case 1:
		m.branchInput.Focus()
	case newFormAgentFocus:
		// selector row: nothing to focus
	case 3:
		m.ticketInput.Focus()
	default:
		m.nameInput.Focus()
	}
}

func (m *Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Confirm):
		if len(m.sessions) == 0 {
			m.mode = ModeList
			return m, nil
		}
		id := m.sessions[m.cursor].ID
		m.mode = ModeList
		return m, func() tea.Msg {
			if err := m.backend.DeleteSession(id); err != nil {
				return ErrorMsg{Err: err}
			}
			return SessionDeletedMsg{ID: id}
		}
	case key.Matches(msg, m.keys.No), key.Matches(msg, m.keys.Cancel):
		m.mode = ModeList
	}
	return m, nil
}

// cycleFormFocus blurs the currently focused textinput (if any), advances
// *focus by one step (forward or backward) with wraparound over total
// fields, then focuses the newly selected textinput (if any). *focus may
// land on an index >= len(inputs) to represent a non-textinput field (e.g.
// an agent selector); such indices are simply skipped for Blur/Focus.
func cycleFormFocus(inputs []textinput.Model, focus *int, total int, forward bool) {
	if *focus < len(inputs) {
		inputs[*focus].Blur()
	}
	if forward {
		*focus = (*focus + 1) % total
	} else {
		*focus = (*focus - 1 + total) % total
	}
	if *focus < len(inputs) {
		inputs[*focus].Focus()
	}
}

// cycleProjectAgentIdx moves idx (a projectForm.agentIdx, possibly
// askAgentIdx) by delta across agentChoices plus the trailing "ask each
// time" entry, wrapping in both directions.
func cycleProjectAgentIdx(idx, delta int) int {
	pos := idx
	if pos == askAgentIdx {
		pos = len(agentChoices)
	}
	n := len(agentChoices) + 1
	pos = (pos + delta + n) % n
	if pos == len(agentChoices) {
		return askAgentIdx
	}
	return pos
}

// projectAgentFields returns the Agent and PromptAgent values a submitted
// project form should store, given its agentIdx (possibly askAgentIdx).
func projectAgentFields(agentIdx int) (agent string, promptAgent bool) {
	if agentIdx == askAgentIdx {
		return "", true
	}
	return agentChoices[agentIdx], false
}

func (m *Model) updateNewProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	const totalFields = projFormInputCount + 2 // +1 agent selector, +1 worktree toggle
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.FormDown):
		cycleFormFocus(m.projForm.inputs, &m.projForm.focus, totalFields, true)
		return m, nil
	case key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.FormUp):
		cycleFormFocus(m.projForm.inputs, &m.projForm.focus, totalFields, false)
		return m, nil
	case key.Matches(msg, m.keys.Left):
		switch m.projForm.focus {
		case projFormInputCount:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, -1)
			return m, nil
		case projFormInputCount + 1:
			m.projForm.noWorktree = !m.projForm.noWorktree
			return m, nil
		}
	case key.Matches(msg, m.keys.Right):
		switch m.projForm.focus {
		case projFormInputCount:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, 1)
			return m, nil
		case projFormInputCount + 1:
			m.projForm.noWorktree = !m.projForm.noWorktree
			return m, nil
		}
	case key.Matches(msg, m.keys.Enter):
		name := m.projForm.inputs[0].Value()
		repo := m.projForm.inputs[1].Value()
		base := m.projForm.inputs[2].Value()
		prefix := m.projForm.inputs[3].Value()
		if base == "" {
			base = "main"
		}
		agent, promptAgent := projectAgentFields(m.projForm.agentIdx)
		p := config.Project{Repo: repo, BaseBranch: base, BranchPrefix: prefix, Agent: agent, PromptAgent: promptAgent, NoWorktree: m.projForm.noWorktree}
		return m, func() tea.Msg {
			err := m.backend.AddProject(name, p)
			return ProjectAddedMsg{Kind: "add", Name: name, Project: p, Err: err}
		}
	}
	if m.projForm.focus < projFormInputCount {
		var cmd tea.Cmd
		m.projForm.inputs[m.projForm.focus], cmd = m.projForm.inputs[m.projForm.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateEditSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = ModeList
	case key.Matches(msg, m.keys.Left):
		m.sessionForm.agentIdx = (m.sessionForm.agentIdx - 1 + len(agentChoices)) % len(agentChoices)
	case key.Matches(msg, m.keys.Right):
		m.sessionForm.agentIdx = (m.sessionForm.agentIdx + 1) % len(agentChoices)
	case key.Matches(msg, m.keys.Enter):
		id := m.sessionForm.id
		agent := agentChoices[m.sessionForm.agentIdx]
		m.busy = true
		m.sessionForm.err = ""
		return m, func() tea.Msg {
			s, err := m.backend.SetSessionAgent(id, agent)
			return SessionAgentUpdatedMsg{Session: s, Err: err}
		}
	}
	return m, nil
}

func editProjectFocuses(p config.Project) []int {
	if p.IsPlain() {
		return []int{1, projFormInputCount}
	}
	return []int{1, 2, 3, projFormInputCount, projFormInputCount + 1}
}

func (m *Model) cycleEditProjectFocus(forward bool) {
	focuses := editProjectFocuses(m.cfg.Projects[m.editProjectName])
	current := 0
	for i, focus := range focuses {
		if focus == m.projForm.focus {
			current = i
			break
		}
	}
	if m.projForm.focus < len(m.projForm.inputs) {
		m.projForm.inputs[m.projForm.focus].Blur()
	}
	if forward {
		current = (current + 1) % len(focuses)
	} else {
		current = (current - 1 + len(focuses)) % len(focuses)
	}
	m.projForm.focus = focuses[current]
	if m.projForm.focus < len(m.projForm.inputs) {
		m.projForm.inputs[m.projForm.focus].Focus()
	}
}

func (m *Model) updateEditProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	project := m.cfg.Projects[m.editProjectName]
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.FormDown):
		m.cycleEditProjectFocus(true)
		return m, nil
	case key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.FormUp):
		m.cycleEditProjectFocus(false)
		return m, nil
	case key.Matches(msg, m.keys.Left):
		switch m.projForm.focus {
		case projFormInputCount:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, -1)
		case projFormInputCount + 1:
			m.projForm.noWorktree = !m.projForm.noWorktree
		}
		return m, nil
	case key.Matches(msg, m.keys.Right):
		switch m.projForm.focus {
		case projFormInputCount:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, 1)
		case projFormInputCount + 1:
			m.projForm.noWorktree = !m.projForm.noWorktree
		}
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		project.Repo = m.projForm.inputs[1].Value()
		project.Agent, project.PromptAgent = projectAgentFields(m.projForm.agentIdx)
		if !project.IsPlain() {
			project.BaseBranch = m.projForm.inputs[2].Value()
			project.BranchPrefix = m.projForm.inputs[3].Value()
			project.NoWorktree = m.projForm.noWorktree
		}
		name := m.editProjectName
		m.busy = true
		m.projForm.err = ""
		return m, func() tea.Msg {
			err := m.backend.UpdateProject(name, project)
			return ProjectUpdatedMsg{Name: name, Err: err}
		}
	}
	if m.projForm.focus < len(m.projForm.inputs) {
		var cmd tea.Cmd
		m.projForm.inputs[m.projForm.focus], cmd = m.projForm.inputs[m.projForm.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateTagForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.FormDown), key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.FormUp):
		forward := !(key.Matches(msg, m.keys.ShiftTab) || key.Matches(msg, m.keys.FormUp))
		cycleFormFocus(m.tagForm.inputs, &m.tagForm.focus, len(m.tagForm.inputs), forward)
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		if len(m.sessions) == 0 {
			m.mode = ModeList
			return m, nil
		}
		id := m.sessions[m.cursor].ID
		ticket := m.tagForm.inputs[0].Value()
		pr := m.tagForm.inputs[1].Value()
		m.mode = ModeList
		return m, func() tea.Msg {
			s, err := m.backend.SetSessionTags(id, ticket, pr)
			if err != nil {
				return ErrorMsg{Err: err}
			}
			return SessionTaggedMsg{Session: s}
		}
	}
	var cmd tea.Cmd
	m.tagForm.inputs[m.tagForm.focus], cmd = m.tagForm.inputs[m.tagForm.focus].Update(msg)
	return m, cmd
}

func (m *Model) activateProject(name string) {
	m.refreshProjects()
	for i, n := range m.projects {
		if n == name {
			m.activeProj = i
			break
		}
	}
	m.cursor = 0
	m.refreshSessions()
}

func (m *Model) updateProjectInitChoice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.mode = ModeNewProject
		return m, nil
	}
	switch msg.String() {
	case "i":
		name := m.pending.name
		p := m.pending.p
		return m, func() tea.Msg {
			err := m.backend.InitProjectAndAdd(name, p)
			return ProjectAddedMsg{Kind: "init", Name: name, Project: p, Err: err}
		}
	case "s":
		name := m.pending.name
		p := m.pending.p
		return m, func() tea.Msg {
			err := m.backend.AddPlainProject(name, p)
			return ProjectAddedMsg{Kind: "plain", Name: name, Project: p, Err: err}
		}
	case "esc", "b":
		m.mode = ModeNewProject
		return m, nil
	}
	return m, nil
}

func (m *Model) updateConfirmDeleteProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		if len(m.projects) == 0 {
			m.mode = ModeList
			return m, nil
		}
		name := m.projects[m.activeProj]
		m.mode = ModeList
		return m, func() tea.Msg {
			err := m.backend.RemoveProject(name)
			return ProjectRemovedMsg{Name: name, Err: err}
		}
	case "n", "esc":
		m.mode = ModeList
	}
	return m, nil
}

func (m *Model) flashError(err error) (tea.Model, tea.Cmd) {
	m.setError(err)
	return m, nil
}
