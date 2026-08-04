package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/erickgnclvs/moomux/internal/browser"
	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/gitwt"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

// updateTitlesCmd pushes each changed session's new status to its tmux
// window name in the background (rename-window shells out, so this stays
// off the Update goroutine).
func updateTitlesCmd(backend Backend, changed map[string]watcher.State) tea.Cmd {
	return func() tea.Msg {
		for id, st := range changed {
			_ = backend.SetSessionStatusTitle(id, st)
		}
		return nil
	}
}

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
		// MultiWatcher fans out one Snapshot per sub-watcher, each covering
		// only its own agent's paths, so we can't replace m.states wholesale
		// here without wiping every other watcher's entries. Instead prune
		// against the full live session set so paths from deleted sessions
		// don't linger forever.
		live := make(map[string]bool, len(m.backend.Sessions()))
		for _, s := range m.backend.Sessions() {
			live[s.WorktreePath] = true
		}
		for path := range m.states {
			if !live[path] {
				delete(m.states, path)
			}
		}
		m.refreshSessions()
		if msg.Snap.Err != nil {
			// Surface once rather than re-flashing on every subsequent tick
			// while the same failure persists.
			warning := "status scan warning: " + msg.Snap.Err.Error()
			if m.flash != warning {
				m.setFlash("error", warning)
			}
		}
		changedTitles := map[string]watcher.State{}
		for _, s := range m.backend.Sessions() {
			st := m.effectiveState(s)
			if prev, ok := m.titleState[s.ID]; !ok || prev != st {
				m.titleState[s.ID] = st
				changedTitles[s.ID] = st
			}
		}
		cmds := []tea.Cmd{listenStatus(m.statusCh), refreshStatusCmd(m)}
		if len(changedTitles) > 0 {
			cmds = append(cmds, updateTitlesCmd(m.backend, changedTitles))
		}
		if cmd := m.fetchStaleGitStatusCmd(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case StatusRefreshedMsg:
		m.tmuxAlive = msg.TmuxAlive
		for id, p := range msg.Prompts {
			if m.prompts[id] == "" {
				m.prompts[id] = p
			}
		}
		if !m.tmuxCheckedOnce {
			// The startup tmux-alive check (fired immediately by Init(), not
			// the routine 2s one) just resolved. Every session's git status
			// is still unfetched at this point (checkedAt is zero), so this
			// covers all of them without waiting for the first watcher tick,
			// which may be a couple seconds off yet.
			m.tmuxCheckedOnce = true
			return m, m.fetchStaleGitStatusCmd()
		}
		return m, nil

	case GitStatusMsg:
		for id, st := range msg.Status {
			delete(m.gitStatusPending, id)
			// Two overlapping fetches for the same session (a routine
			// refresh racing the delete dialog's on-demand check, say) can
			// resolve out of order — never let an older result clobber a
			// fresher one already recorded.
			if cur, ok := m.gitStatus[id]; ok && !st.checkedAt.After(cur.checkedAt) {
				continue
			}
			m.gitStatus[id] = st
		}
		if m.confirmChecking && len(m.sessions) > 0 {
			if st, ok := msg.Status[m.sessions[m.cursor].ID]; ok {
				m.confirmGit = st
				m.confirmChecking = false
			}
		}
		return m, nil

	case StatusChannelClosedMsg:
		m.setFlash("error", "status watcher stopped")
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
		for i, name := range m.projects {
			if name == msg.Session.Project {
				m.activeProj = i
				break
			}
		}
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
			if m.mode != ModeEditSession {
				// The user closed the form while the update was in flight;
				// don't snap it back open.
				m.setError(msg.Err)
				return m, nil
			}
			m.sessionForm.err = msg.Err.Error()
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
				m.setFlash("info", "added project "+msg.Name)
				m.finishProjectAdded(msg.Name)
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
			m.setFlash("info", "initialized git repo + added "+msg.Name)
			m.finishProjectAdded(msg.Name)
			return m, nil
		case "plain":
			if msg.Err != nil {
				m.mode = ModeNewProject
				m.projForm.err = msg.Err.Error()
				return m, nil
			}
			m.activateProject(msg.Name)
			m.setFlash("info", "added plain (non-git) project "+msg.Name)
			m.finishProjectAdded(msg.Name)
			return m, nil
		}
		return m, nil

	case ProjectMovedMsg:
		if msg.Err != nil {
			m.setError(msg.Err)
			return m, nil
		}
		// Re-anchor by name rather than index on both cursors: the active
		// project (which may not be the one that just moved, when the
		// reorder came from the picker) and, while the picker is open, its
		// own highlight following the project it just reordered.
		var activeName string
		if m.activeProj < len(m.projects) {
			activeName = m.projects[m.activeProj]
		}
		m.refreshProjects()
		if i := indexOfProject(m.projects, activeName); i >= 0 {
			m.activeProj = i
		}
		if m.mode == ModeProjectPicker {
			if i := indexOfProject(m.projects, msg.Name); i >= 0 {
				m.pickerCursor = i
			}
		}
		return m, nil

	case ProjectUpdatedMsg:
		m.busy = false
		if msg.Err != nil {
			if m.mode != ModeEditProject {
				m.setError(msg.Err)
				return m, nil
			}
			m.projForm.err = msg.Err.Error()
			return m, nil
		}
		m.activateProject(msg.Name)
		m.mode = m.projectDialogReturn
		m.setFlash("info", "updated project "+msg.Name)
		return m, nil

	case ProjectRemovedMsg:
		if msg.Err != nil {
			m.mode = m.projectDialogReturn
			m.setFlash("error", msg.Err.Error())
			return m, nil
		}
		m.refreshProjects()
		m.cursor = 0
		m.refreshSessions()
		m.mode = m.projectDialogReturn
		m.setFlash("info", "removed project "+msg.Name)
		return m, nil

	case ThemeSetMsg:
		m.setFlash("info", "theme saved: "+msg.Theme+" / "+appearanceLabel(msg.Appearance))
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
			if url, copyOnly := m.linkAt(msg.X, msg.Y); url != "" {
				if copyOnly || m.isRemote() {
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
			// Not a ticket/PR icon — a tap on the row itself selects and
			// opens that session in one motion, since mobile clients (mosh
			// over Moshi, etc.) have no keyboard focus to move a cursor with
			// first.
			if id, ok := m.sessionRowAt(msg.X, msg.Y); ok {
				for i, s := range m.sessions {
					if s.ID == id {
						m.cursor = i
						break
					}
				}
				return m, m.openSessionCmd(id)
			}
		}
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if len(m.sessions) > 0 {
					m.cursor = (m.cursor - 1 + len(m.sessions)) % len(m.sessions)
				}
			case tea.MouseButtonWheelDown:
				if len(m.sessions) > 0 {
					m.cursor = (m.cursor + 1) % len(m.sessions)
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			// Quit from anywhere — the per-mode handlers ignore unmatched
			// keys, so without this ctrl+c is swallowed inside every form
			// and dialog. ("q" stays mode-specific: it's typeable text in
			// the form inputs.)
			m.cancelPoll()
			return m, tea.Quit
		}
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
		case ModeProjectPicker:
			return m.updateProjectPicker(msg)
		case ModeThemePicker:
			return m.updateThemePicker(msg)
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

// openNewSessionForm opens the new-session dialog for the active project,
// pre-selecting its default agent unless it requires an explicit choice on
// every session.
func (m *Model) openNewSessionForm() {
	m.mode = ModeNewForm
	m.newFormErr = ""
	m.newFormFocus = 1 // start below the project picker, at the name field
	m.newFormProjIdx = m.activeProj
	m.nameInput.SetValue("")
	m.nameInput.Focus()
	m.branchInput.SetValue("")
	m.branchInput.Blur()
	m.ticketInput.SetValue("")
	m.ticketInput.Blur()
	m.prInput.SetValue("")
	m.prInput.Blur()
	m.promptInput.SetValue("")
	m.promptInput.Blur()
	m.resetOverlayViewport()
	m.resizeFormInputs()
	m.newFormApplyProjectDefaults()
}

// newFormApplyProjectDefaults sets the agent selector to the currently
// selected project's default agent, unless that project requires an
// explicit choice on every session.
func (m *Model) newFormApplyProjectDefaults() {
	proj := m.projects[m.newFormProjIdx]
	p := m.cfg.Projects[proj]
	if p.PromptAgent {
		m.newFormAgentIdx = -1
	} else {
		m.newFormAgentIdx = agentChoiceIndex(p.AgentName())
	}
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
		m.switchProject(1)
	case key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.PrevProject):
		m.switchProject(-1)
	case key.Matches(msg, m.keys.NextProjectAll):
		if len(m.projects) > 0 {
			m.activeProj = (m.activeProj + 1) % len(m.projects)
			m.cursor = 0
			m.refreshSessions()
		}
	case key.Matches(msg, m.keys.PrevProjectAll):
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
		if m.busy {
			// A session create is already in flight; opening another form here
			// would let its submit fire a second, concurrent osascript call
			// into iTerm2 before the first tab/window has settled — iTerm2
			// resolves "current window"/"current tab" racily while it's still
			// launching, so the second AppleScript can land its "tmux attach"
			// in the tab the first one just created instead of a new one (see
			// terminal/iterm.go's createTab).
			return m.flashError(fmt.Errorf("still creating the previous session — wait for it to finish"))
		}
		if len(m.projects) == 0 {
			return m.flashError(fmt.Errorf("no projects configured — press / then n to add one"))
		}
		m.openNewSessionForm()
	case key.Matches(msg, m.keys.Delete):
		if len(m.sessions) > 0 {
			id := m.sessions[m.cursor].ID
			// Opens instantly on whatever's cached from the routine
			// background refresh (or nothing yet) so the dialog never
			// visibly pauses, then kicks off a fresh check of its own in the
			// background — confirmChecking drives a small loading note in
			// the dialog until GitStatusMsg lands and updates confirmGit for
			// real. Deliberately not routed through fetchStaleGitStatusCmd's
			// gitStatusPending bookkeeping: this is an explicit user action
			// that wants the freshest answer right now, not deduped against
			// whatever the periodic refresh happens to be doing.
			m.confirmGit = m.gitStatus[id]
			m.confirmAck = false
			m.confirmChecking = true
			m.mode = ModeConfirmDelete
			// Without this the dialog inherits the previous overlay's scroll
			// offset and can open with the "what you're deleting" text
			// scrolled off-screen, leaving only "y to confirm" visible.
			m.resetOverlayViewport()
			return m, fetchGitStatusCmd(m.backend, []string{id})
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
	case key.Matches(msg, m.keys.AllSessions):
		m.allSessions = !m.allSessions
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
	case key.Matches(msg, m.keys.ProjectPicker):
		// No zero-projects guard: the picker is now the only way to add a
		// project (main list's P/E were removed), so it has to stay reachable
		// even with none yet — its own empty-state hint covers that ("press
		// n to add one").
		m.pickerCursor = m.activeProj
		m.mode = ModeProjectPicker
		m.resetOverlayViewport()
		return m, nil
	case key.Matches(msg, m.keys.ThemePicker):
		m.openThemePicker()
		return m, nil
	case key.Matches(msg, m.keys.DelProject):
		if len(m.projects) == 0 {
			return m.flashError(fmt.Errorf("no projects to remove"))
		}
		if n := m.projectSessionCount(); n > 0 {
			return m.flashError(fmt.Errorf("%s has %d session(s) (incl. archived) — delete them first", m.projects[m.activeProj], n))
		}
		m.projectDialogReturn = ModeList
		m.mode = ModeConfirmDeleteProject
		m.resetOverlayViewport()
		return m, nil
	case key.Matches(msg, m.keys.Open):
		if len(m.sessions) > 0 {
			return m, m.openSessionCmd(m.sessions[m.cursor].ID)
		}
	}
	return m, nil
}

// switchProject moves the active project by delta, skipping empty ones —
// shared by Tab/Shift-Tab and a horizontal mouse-wheel swipe.
func (m *Model) switchProject(delta int) {
	if len(m.projects) == 0 {
		return
	}
	m.activeProj = m.nextNonEmptyProject(delta)
	m.cursor = 0
	m.refreshSessions()
}

// openSessionCmd returns a Cmd that opens/attaches the given session,
// shared by the Enter/o key binding and a row click.
func (m *Model) openSessionCmd(id string) tea.Cmd {
	return func() tea.Msg {
		hint, err := m.backend.OpenSession(id)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return SessionOpenedMsg{ID: id, Hint: hint}
	}
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
		// Only steer the project/agent selectors when one of them is the
		// focused field; otherwise the arrows belong to the text input's
		// cursor.
		if m.newFormFocus == newFormProjFocus {
			if len(m.projects) > 0 {
				if key.Matches(msg, m.keys.Left) {
					m.newFormProjIdx = (m.newFormProjIdx - 1 + len(m.projects)) % len(m.projects)
				} else {
					m.newFormProjIdx = (m.newFormProjIdx + 1) % len(m.projects)
				}
				m.newFormApplyProjectDefaults()
			}
			return m, nil
		}
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
		pr := m.prInput.Value()
		firstPrompt := m.promptInput.Value()
		if name == "" && branch == "" {
			m.newFormErr = "enter a session name or an existing branch"
			return m, nil
		}
		if m.newFormAgentIdx < 0 {
			m.newFormErr = "this project requires choosing an agent — tab to the agent row, then ←→"
			return m, nil
		}
		m.newFormErr = ""
		proj := m.projects[m.newFormProjIdx]
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
			// The session (worktree + tmux pane) already exists at this
			// point — a PR-tag or first-prompt failure below must not
			// discard that success and report it as if creation itself
			// failed; it's surfaced as a hint on the same SessionCreatedMsg
			// instead, same as CreateSession's own degraded-but-succeeded
			// terminal-open failures.
			if pr != "" {
				if updated, tagErr := m.backend.SetSessionTags(s.ID, ticket, pr); tagErr != nil {
					hint = joinHint(hint, fmt.Sprintf("couldn't set PR tag: %v", tagErr))
				} else {
					s = updated
				}
			}
			if firstPrompt != "" {
				if extra := newFormPromptExtras(ticket, pr); extra != "" {
					firstPrompt += "\n\n" + extra
				}
				if err := m.backend.StartFirstPrompt(s.TmuxSession, firstPrompt); err != nil {
					hint = joinHint(hint, fmt.Sprintf("couldn't send first prompt: %v", err))
				}
			}
			return SessionCreatedMsg{Session: s, Hint: hint}
		}
	}
	var cmd tea.Cmd
	switch m.newFormFocus {
	case newFormProjFocus:
		// selector row: no text input to type into
	case 2:
		m.branchInput, cmd = m.branchInput.Update(msg)
	case newFormAgentFocus:
		// selector row: no text input to type into
	case 4:
		m.ticketInput, cmd = m.ticketInput.Update(msg)
	case 5:
		m.prInput, cmd = m.prInput.Update(msg)
	case 6:
		m.promptInput, cmd = m.promptInput.Update(msg)
	default:
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, cmd
}

// joinHint combines two non-empty hint strings for display; either may be
// empty.
func joinHint(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + " — " + b
	}
}

// newFormPromptExtras builds a "Ticket: ...\nPR: ..." block from whichever
// of ticket/pr are non-empty, so the agent's first task carries the same
// context the session list shows as clickable icons. Empty if neither is
// set.
func newFormPromptExtras(ticket, pr string) string {
	var lines []string
	if ticket != "" {
		lines = append(lines, "Ticket: "+ticket)
	}
	if pr != "" {
		lines = append(lines, "PR: "+pr)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) newFormBlurAll() {
	m.nameInput.Blur()
	m.branchInput.Blur()
	m.ticketInput.Blur()
	m.prInput.Blur()
	m.promptInput.Blur()
}

func (m *Model) newFormFocusInput() {
	switch m.newFormFocus {
	case newFormProjFocus:
		// selector row: nothing to focus
	case 2:
		m.branchInput.Focus()
	case newFormAgentFocus:
		// selector row: nothing to focus
	case 4:
		m.ticketInput.Focus()
	case 5:
		m.prInput.Focus()
	case 6:
		m.promptInput.Focus()
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
		if m.confirmChecking {
			// The fresh check hasn't resolved yet — acting on whatever's
			// cached (possibly nothing, possibly stale) would defeat the
			// point of checking at all. Ignore y until GitStatusMsg lands.
			return m, nil
		}
		if m.confirmGit.ok && (m.confirmGit.dirty || m.confirmGit.unpushed) && !m.confirmAck {
			// First y on a dirty/unpushed worktree only acknowledges the
			// warning; a second y is required to actually delete.
			m.confirmAck = true
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
	const totalFields = projFormInputCount + 3 // +1 emoji selector, +1 agent selector, +1 worktree toggle
	switch {
	case key.Matches(msg, m.keys.Cancel):
		// projectDialogReturn defaults to ModeList (its zero value), which is
		// also correct for New()'s zero-projects startup case that opens
		// this form directly without going through the picker or main list.
		m.mode = m.projectDialogReturn
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
			m.projForm.emojiIdx = cycleProjectEmojiIdx(m.projForm.emojiChoices, m.projForm.emojiIdx, -1)
			return m, nil
		case projFormInputCount + 1:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, -1)
			return m, nil
		case projFormInputCount + 2:
			m.projForm.noWorktree = !m.projForm.noWorktree
			return m, nil
		}
	case key.Matches(msg, m.keys.Right):
		switch m.projForm.focus {
		case projFormInputCount:
			m.projForm.emojiIdx = cycleProjectEmojiIdx(m.projForm.emojiChoices, m.projForm.emojiIdx, 1)
			return m, nil
		case projFormInputCount + 1:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, 1)
			return m, nil
		case projFormInputCount + 2:
			m.projForm.noWorktree = !m.projForm.noWorktree
			return m, nil
		}
	case key.Matches(msg, m.keys.Enter):
		name := m.projForm.inputs[0].Value()
		repo := m.projForm.inputs[1].Value()
		base := m.projForm.inputs[2].Value()
		prefix := m.projForm.inputs[3].Value()
		emoji := projectEmojiFieldValue(m.projForm.emojiChoices, m.projForm.emojiIdx)
		if base == "" {
			base = "main"
		}
		agent, promptAgent := projectAgentFields(m.projForm.agentIdx)
		p := config.Project{Repo: repo, BaseBranch: base, BranchPrefix: prefix, Emoji: emoji, Agent: agent, PromptAgent: promptAgent, NoWorktree: m.projForm.noWorktree}
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
		// A background op is in flight (possibly a session create started
		// before this overlay opened); still let the user close the form
		// instead of being stuck in an unresponsive modal.
		if key.Matches(msg, m.keys.Cancel) {
			m.mode = ModeList
		}
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
		return []int{1, projFormInputCount, projFormInputCount + 1}
	}
	return []int{1, 2, 3, projFormInputCount, projFormInputCount + 1, projFormInputCount + 2}
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
		if key.Matches(msg, m.keys.Cancel) {
			m.mode = m.projectDialogReturn
		}
		return m, nil
	}
	project := m.cfg.Projects[m.editProjectName]
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = m.projectDialogReturn
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
			m.projForm.emojiIdx = cycleProjectEmojiIdx(m.projForm.emojiChoices, m.projForm.emojiIdx, -1)
		case projFormInputCount + 1:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, -1)
		case projFormInputCount + 2:
			m.projForm.noWorktree = !m.projForm.noWorktree
		}
		return m, nil
	case key.Matches(msg, m.keys.Right):
		switch m.projForm.focus {
		case projFormInputCount:
			m.projForm.emojiIdx = cycleProjectEmojiIdx(m.projForm.emojiChoices, m.projForm.emojiIdx, 1)
		case projFormInputCount + 1:
			m.projForm.agentIdx = cycleProjectAgentIdx(m.projForm.agentIdx, 1)
		case projFormInputCount + 2:
			m.projForm.noWorktree = !m.projForm.noWorktree
		}
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		project.Repo = m.projForm.inputs[1].Value()
		project.Emoji = projectEmojiFieldValue(m.projForm.emojiChoices, m.projForm.emojiIdx)
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

// indexOfProject returns name's index in m.projects, or -1 if it's not
// there (e.g. removed, or never added).
func indexOfProject(projects []string, name string) int {
	for i, n := range projects {
		if n == name {
			return i
		}
	}
	return -1
}

func (m *Model) activateProject(name string) {
	m.refreshProjects()
	if i := indexOfProject(m.projects, name); i >= 0 {
		m.activeProj = i
	}
	m.cursor = 0
	m.refreshSessions()
}

// finishProjectAdded is the shared tail of every successful ProjectAddedMsg
// branch (add/init/plain). Adding a project from the main list still funnels
// into creating its first session, but adding one from the picker returns to
// the picker — with the cursor following the project just added — rather
// than yanking the user into an unrelated dialog they didn't ask for.
func (m *Model) finishProjectAdded(name string) {
	if m.projectDialogReturn == ModeProjectPicker {
		if i := indexOfProject(m.projects, name); i >= 0 {
			m.pickerCursor = i
		}
		m.mode = ModeProjectPicker
		m.resetOverlayViewport()
		return
	}
	m.openNewSessionForm()
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
			m.mode = m.projectDialogReturn
			return m, nil
		}
		name := m.projects[m.activeProj]
		m.mode = m.projectDialogReturn
		return m, func() tea.Msg {
			err := m.backend.RemoveProject(name)
			return ProjectRemovedMsg{Name: name, Err: err}
		}
	case "n", "esc":
		m.mode = m.projectDialogReturn
	}
	return m, nil
}

// updateProjectPicker handles keys while ModeProjectPicker is open: ↑↓/jk
// move the cursor, enter/o jumps to the highlighted project and closes the
// dialog, esc backs out without changing the active project.
func (m *Model) updateProjectPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.keys.Up):
		if len(m.projects) > 0 {
			m.pickerCursor = (m.pickerCursor - 1 + len(m.projects)) % len(m.projects)
		}
	case key.Matches(msg, m.keys.Down):
		if len(m.projects) > 0 {
			m.pickerCursor = (m.pickerCursor + 1) % len(m.projects)
		}
	// Reorder with the same up/down chords the session list uses (shift+↑↓ /
	// KJ) rather than MoveProjLeft/Right — this list is vertical, like the
	// session list, not the header's horizontal tabs.
	case key.Matches(msg, m.keys.MoveUp):
		if len(m.projects) > 0 && m.pickerCursor > 0 {
			name := m.projects[m.pickerCursor]
			return m, func() tea.Msg {
				if err := m.backend.MoveProject(name, -1); err != nil {
					return ProjectMovedMsg{Name: name, Err: err}
				}
				return ProjectMovedMsg{Name: name}
			}
		}
	case key.Matches(msg, m.keys.MoveDown):
		if len(m.projects) > 0 && m.pickerCursor < len(m.projects)-1 {
			name := m.projects[m.pickerCursor]
			return m, func() tea.Msg {
				if err := m.backend.MoveProject(name, 1); err != nil {
					return ProjectMovedMsg{Name: name, Err: err}
				}
				return ProjectMovedMsg{Name: name}
			}
		}
	// d rather than the main list's D (capitalized there to stay distinct
	// from the session-level d shown on the same screen) — the picker only
	// ever lists projects, so there's no session-level d to collide with.
	// e has no main-list equivalent at all anymore; edit only happens here.
	case key.Matches(msg, m.keys.Delete):
		if len(m.projects) > 0 {
			proj := m.projects[m.pickerCursor]
			if n := m.projectSessionCountFor(proj); n > 0 {
				return m.flashError(fmt.Errorf("%s has %d session(s) (incl. archived) — delete them first", proj, n))
			}
			// ModeConfirmDeleteProject acts on m.activeProj, so land on the
			// highlighted project first — the same "jump to it" step Open
			// does, just followed immediately by the confirm dialog instead
			// of returning to the list. Route through activateProject (not a
			// bare m.activeProj assignment) so m.sessions/m.cursor stay in
			// sync too — otherwise canceling back out of the confirm dialog
			// leaves the header pointing at this project while the session
			// list still shows the previous one's sessions.
			m.activateProject(proj)
			m.projectDialogReturn = ModeProjectPicker
			m.mode = ModeConfirmDeleteProject
			m.resetOverlayViewport()
		}
		return m, nil
	case key.Matches(msg, m.keys.EditSession):
		// m.busy stays true (not reset) if a previous edit's save is still
		// in flight when its Cancel is pressed — see updateEditProject's own
		// busy guard — so without this check a second edit opened here could
		// have its state clobbered when that first ProjectUpdatedMsg lands.
		if len(m.projects) > 0 && !m.busy {
			name := m.projects[m.pickerCursor]
			m.editProjectName = name
			m.projForm = editProjectForm(name, m.cfg.Projects[name])
			m.activateProject(name) // keeps m.sessions/m.cursor in sync — see the delete case above
			m.projectDialogReturn = ModeProjectPicker
			m.mode = ModeEditProject
			m.resetOverlayViewport()
			m.resizeFormInputs()
		}
		return m, nil
	case key.Matches(msg, m.keys.New):
		// No len(m.projects)==0 guard: this is now the only way to add a
		// project at all (past the zero-projects startup flow), so it must
		// work from the picker's own empty state too — see its "press n to
		// add one" hint.
		m.projectDialogReturn = ModeProjectPicker
		m.mode = ModeNewProject
		m.projForm = newProjectForm()
		m.resetOverlayViewport()
		m.resizeFormInputs()
		return m, nil
	case key.Matches(msg, m.keys.Open):
		if len(m.projects) > 0 {
			m.activeProj = m.pickerCursor
			m.cursor = 0
			m.refreshSessions()
		}
		m.mode = ModeList
	}
	return m, nil
}

// openThemePicker opens ModeThemePicker, seeding the cursor and live-preview
// appearance from the persisted config so Esc has something to revert to.
func (m *Model) openThemePicker() {
	m.themeCursor = themeIndex(m.cfg.Theme)
	m.previewAppearance = m.cfg.Appearance
	m.mode = ModeThemePicker
	m.resetOverlayViewport()
}

// updateThemePicker handles keys while ModeThemePicker is open. Up/down and
// 'a' apply their choice immediately (live preview); enter persists it, esc
// reverts to the config's last-saved theme/appearance.
func (m *Model) updateThemePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Cancel):
		applyTheme(m.cfg.Theme)
		applyAppearance(m.cfg.Appearance)
		m.mode = ModeList
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.themeCursor = (m.themeCursor - 1 + len(themeNames)) % len(themeNames)
		applyTheme(themeNames[m.themeCursor])
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.themeCursor = (m.themeCursor + 1) % len(themeNames)
		applyTheme(themeNames[m.themeCursor])
		return m, nil
	case msg.String() == "a":
		m.previewAppearance = nextAppearance(m.previewAppearance)
		applyAppearance(m.previewAppearance)
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		theme := themeNames[m.themeCursor]
		appearance := m.previewAppearance
		m.mode = ModeList
		return m, func() tea.Msg {
			if err := m.backend.SetTheme(theme, appearance); err != nil {
				return ErrorMsg{Err: err}
			}
			return ThemeSetMsg{Theme: theme, Appearance: appearance}
		}
	}
	return m, nil
}

func (m *Model) flashError(err error) (tea.Model, tea.Cmd) {
	m.setError(err)
	return m, nil
}
