package tui

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erickgnclvs/moomux/internal/browser"
	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/prompt"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

// Backend is everything the TUI calls into. main wires the real impl;
// tests can supply fakes.
type Backend interface {
	// CreateSession's hint, when non-empty, is a user-facing instruction
	// (e.g. "run: tmux attach -t ...") to show alongside success — it is
	// not an error.
	CreateSession(project, name, agent, existingBranch, ticket string) (s session.Session, hint string, err error)
	OpenSession(id string) (hint string, err error)
	DeleteSession(id string) error
	KillTmux(id string) error
	SetSessionTags(id, ticket, pr string) (session.Session, error)
	SetSessionAgent(id, agent string) (session.Session, error)
	// SetSessionArchived hides (or restores) a session from the default
	// list without touching its tmux session or worktree.
	SetSessionArchived(id string, archived bool) (session.Session, error)
	MoveSession(id string, delta int) error
	MoveProject(name string, delta int) error
	// TmuxAliveAll returns id→alive for every stored session using a single
	// tmux list-sessions call instead of N has-session calls.
	TmuxAliveAll() map[string]bool
	Sessions() []session.Session
	Projects() []string
	AddProject(name string, p config.Project) error
	InitProjectAndAdd(name string, p config.Project) error
	AddPlainProject(name string, p config.Project) error
	UpdateProject(name string, p config.Project) error
	RemoveProject(name string) error
}

type Mode int

const (
	ModeList Mode = iota
	ModeNewForm
	ModeConfirmDelete
	ModeNewProject
	ModeConfirmDeleteProject
	ModeProjectInitChoice
	ModeTagForm
	ModeHelp
	ModeEditSession
	ModeEditProject
)

var agentChoices = []string{"claude", "codex", "opencode"}

// askAgentIdx is a sentinel projectForm.agentIdx value, one past the real
// agentChoices, selected as an extra "ask each time" entry in the project
// agent selector — it maps to config.Project.PromptAgent instead of Agent.
const askAgentIdx = -1

// projFormInputCount is the number of plain text inputs in the project form
// (name, repo, base branch, branch prefix). Non-text controls follow at
// fixed offsets from it: focus==projFormInputCount is the emoji selector,
// +1 the agent selector, +2 the worktree toggle.
const projFormInputCount = 4

type projectForm struct {
	inputs   []textinput.Model
	focus    int
	emojiIdx int // index into emojiChoices; 0 is the "auto" (deterministic pick) sentinel
	// emojiChoices is normally projectEmojiChoices, but editProjectForm
	// inserts the project's existing Emoji as its own entry when it isn't
	// one of projectEmojiPalette's glyphs (e.g. hand-edited into the TOML
	// config) — otherwise it would be indistinguishable from "auto" and
	// saving any unrelated field would silently discard it.
	emojiChoices []string
	agentIdx     int // index into agentChoices, or askAgentIdx for "ask each time"
	noWorktree   bool
	err          string
}

type pendingProject struct {
	name string
	p    config.Project
}

type tagForm struct {
	inputs []textinput.Model // [0]=ticket, [1]=PR
	focus  int
}

type sessionForm struct {
	id       string
	project  string
	name     string
	agentIdx int
	err      string
}

func newTagForm(ticket, pr string) tagForm {
	mk := func(placeholder, value string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Width = 48
		ti.CharLimit = 256
		ti.SetValue(value)
		return ti
	}
	tf := tagForm{
		inputs: []textinput.Model{
			mk("ticket url", ticket),
			mk("pr url", pr),
		},
	}
	tf.inputs[0].Focus()
	return tf
}

type Model struct {
	cfg     *config.Config
	backend Backend
	keys    KeyMap

	projects     []string
	activeProj   int
	sessions     []session.Session
	showArchived bool // when true, the list shows archived sessions instead of active ones
	allSessions  bool // when true, the list shows every project's sessions, grouped by project, instead of just the active one
	cursor       int
	states       map[string]watcher.State
	tmuxAlive    map[string]bool
	prompts      map[string]string
	statusCh     <-chan watcher.Snapshot
	cancelPoll   context.CancelFunc

	mode            Mode
	nameInput       textinput.Model
	branchInput     textinput.Model
	ticketInput     textinput.Model
	newFormFocus    int // 0=project selector, 1=nameInput, 2=branchInput, 3=agent selector, 4=ticketInput
	newFormErr      string
	newFormAgentIdx int // agent selector in the new-session form; -1 means "not chosen yet"
	newFormProjIdx  int // project selector in the new-session form; index into m.projects
	projForm        projectForm
	sessionForm     sessionForm
	editProjectName string
	tagForm         tagForm
	pending         pendingProject
	flash           string
	flashKind       string // "info" or "error"
	flashTime       time.Time
	busy            bool // true while a background op (e.g. session create) is in flight; suppresses flash expiry
	// forceCopyLinks overrides browser.Remote()'s auto-detection, forcing
	// ticket/PR clicks to copy instead of open. Auto-detection has no
	// signal at all for transports like mosh that don't set SSH_TTY/
	// SSH_CONNECTION/SSH_CLIENT, so this lets a user force the behavior
	// from inside the running session instead of needing shell/env access
	// on the host. Toggled with R; there's no "force open" counterpart —
	// if auto-detection isn't already saying remote, links just open.
	forceCopyLinks bool

	width, height int

	linkHits        []resolvedLinkHit
	overlayViewport viewport.Model
	overlayMode     Mode
	overlayFocus    int
}

func agentChoiceIndex(agent string) int {
	for i, choice := range agentChoices {
		if choice == agent {
			return i
		}
	}
	return 0
}

// resolvedLinkHit is a linkHit translated into absolute terminal
// coordinates, computed fresh on every View() call and consulted by the
// mouse handler in Update() to resolve a click to a session's ticket/PR URL.
type resolvedLinkHit struct {
	sessionID string
	url       string
	y         int
	x0, x1    int // half-open column range
}

// updateLinkHits recomputes m.linkHits in absolute terminal coordinates from
// the list- and detail-local hits produced during rendering. It's a no-op
// (clearing hits) outside ModeList, since panels aren't clickable behind an
// overlay.
func (m *Model) updateLinkHits(header string, listHits, detailHits []linkHit, detailX, detailY int) {
	if m.mode != ModeList {
		m.linkHits = nil
		return
	}
	m.linkHits = m.linkHits[:0]
	appendHits := func(hits []linkHit, originX, originY int) {
		for _, h := range hits {
			m.linkHits = append(m.linkHits, resolvedLinkHit{
				sessionID: h.sessionID,
				url:       h.url,
				y:         originY + h.line,
				x0:        originX + h.col0,
				x1:        originX + h.col1,
			})
		}
	}
	listX := panelBorder.GetBorderLeftSize() + panelBorder.GetPaddingLeft()
	listY := lipgloss.Height(header) + panelBorder.GetBorderTopSize()
	appendHits(listHits, listX, listY)
	appendHits(detailHits, detailX, detailY)
}

// isRemote decides whether a ticket/PR icon click should copy the URL
// (true) or open it in a browser (false), honoring the user's R toggle
// before falling back to browser.Remote()'s SSH auto-detection.
func (m *Model) isRemote() bool {
	return m.forceCopyLinks || browser.Remote()
}

// linkAt returns the URL of the ticket/PR icon at absolute terminal
// coordinates (x, y), if any.
func (m *Model) linkAt(x, y int) string {
	for _, h := range m.linkHits {
		if y == h.y && x >= h.x0 && x < h.x1 {
			return h.url
		}
	}
	return ""
}

func New(cfg *config.Config, backend Backend, statusCh <-chan watcher.Snapshot, cancel context.CancelFunc) *Model {
	ti := textinput.New()
	ti.Placeholder = "session name (optional if branch set)"
	ti.CharLimit = 64
	ti.Width = 40

	bi := textinput.New()
	bi.Placeholder = "existing branch (optional)"
	bi.CharLimit = 128
	bi.Width = 40

	tki := textinput.New()
	tki.Placeholder = "ticket url (optional)"
	tki.CharLimit = 256
	tki.Width = 40

	m := &Model{
		cfg:             cfg,
		backend:         backend,
		keys:            DefaultKeyMap(),
		states:          map[string]watcher.State{},
		tmuxAlive:       map[string]bool{},
		prompts:         map[string]string{},
		statusCh:        statusCh,
		cancelPoll:      cancel,
		nameInput:       ti,
		branchInput:     bi,
		ticketInput:     tki,
		overlayViewport: viewport.New(1, 1),
		overlayMode:     ModeList,
		overlayFocus:    -1,
	}
	m.projects = cfg.OrderedProjectNames()
	m.refreshSessions()
	m.refreshTmuxAlive()
	m.refreshPrompts()
	if len(m.projects) == 0 {
		m.mode = ModeNewProject
		m.projForm = newProjectForm()
	}
	return m
}

func (m *Model) refreshPrompts() {
	home, _ := os.UserHomeDir()
	for _, s := range m.backend.Sessions() {
		if p := m.prompts[s.ID]; p != "" {
			continue
		}
		m.prompts[s.ID] = prompt.ForAgent(home, s.AgentName(), s.WorktreePath)
	}
}

func (m *Model) refreshTmuxAlive() {
	m.tmuxAlive = m.backend.TmuxAliveAll()
}

// refreshStatusCmd returns a tea.Cmd that computes the tmux-alive map and
// missing prompts off the Bubble Tea event-loop goroutine. It must not
// mutate m — only Update may mutate model state. m.prompts is also written
// concurrently by Update, so we snapshot the keys we already know about here
// (on the caller's goroutine) rather than reading m.prompts from the
// returned closure, which would race.
func refreshStatusCmd(m *Model) tea.Cmd {
	backend := m.backend
	known := make(map[string]string, len(m.prompts))
	for id, p := range m.prompts {
		known[id] = p
	}

	return func() tea.Msg {
		tmuxAlive := backend.TmuxAliveAll()

		home, _ := os.UserHomeDir()
		prompts := make(map[string]string)
		for _, s := range backend.Sessions() {
			if p := known[s.ID]; p != "" {
				continue
			}
			prompts[s.ID] = prompt.ForAgent(home, s.AgentName(), s.WorktreePath)
		}

		return StatusRefreshedMsg{TmuxAlive: tmuxAlive, Prompts: prompts}
	}
}

// effectiveState returns the state to display: if tmux is dead the
// Claude-session JSON is stale and the session is effectively parked.
func (m *Model) effectiveState(s session.Session) watcher.State {
	if !m.tmuxAlive[s.ID] {
		return watcher.Parked
	}
	return m.states[s.WorktreePath]
}

func (m *Model) refreshProjects() {
	m.projects = m.cfg.OrderedProjectNames()
	if m.activeProj >= len(m.projects) {
		m.activeProj = 0
	}
}

// newProjectForm builds the add-project form, pre-filling name/repo from the
// current working directory when it looks usable — the common case is
// running moomux from inside the repo you want to add, and most users don't
// want to type or remember its absolute path.
func newProjectForm() projectForm {
	mk := func(placeholder string, width int) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Width = width
		ti.CharLimit = 256
		return ti
	}
	pf := projectForm{
		inputs: []textinput.Model{
			mk("name (e.g. eg_system)", 32),
			mk("repo path (e.g. ~/Development/eg_system)", 48),
			mk("base branch (default: main)", 24),
			mk("branch prefix (optional)", 24),
		},
		emojiChoices: projectEmojiChoices,
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "/" {
		pf.inputs[0].SetValue(filepath.Base(cwd))
		pf.inputs[1].SetValue(cwd)
	}
	pf.inputs[0].Focus()
	return pf
}

func editProjectForm(name string, p config.Project) projectForm {
	pf := newProjectForm()
	pf.inputs[0].SetValue(name)
	pf.inputs[1].SetValue(p.Repo)
	pf.inputs[2].SetValue(p.BaseBranch)
	pf.inputs[3].SetValue(p.BranchPrefix)
	pf.emojiIdx = 0
	for i, e := range projectEmojiPalette {
		if e == p.Emoji {
			pf.emojiIdx = i + 1
			break
		}
	}
	if p.Emoji != "" && pf.emojiIdx == 0 {
		// p.Emoji isn't one of the palette's glyphs — keep it as its own
		// selectable entry (right after "auto") instead of collapsing it
		// into the auto sentinel, which would silently discard it the next
		// time this project is saved without touching the emoji field.
		pf.emojiChoices = append([]string{"auto", p.Emoji}, projectEmojiPalette...)
		pf.emojiIdx = 1
	}
	pf.inputs[0].Blur()
	pf.inputs[1].Focus()
	pf.focus = 1
	if p.PromptAgent {
		pf.agentIdx = askAgentIdx
	} else {
		pf.agentIdx = agentChoiceIndex(p.AgentName())
	}
	pf.noWorktree = p.NoWorktree
	return pf
}

// projectSessionCount returns how many sessions the active project has,
// archived or not — a project can only be removed once it has none.
func (m *Model) projectSessionCount() int {
	if len(m.projects) == 0 {
		return 0
	}
	proj := m.projects[m.activeProj]
	n := 0
	for _, s := range m.backend.Sessions() {
		if s.Project == proj {
			n++
		}
	}
	return n
}

// projectHasSessions reports whether the named project has any session at
// all (archived or not) — used to skip empty projects when cycling.
func (m *Model) projectHasSessions(name string) bool {
	for _, s := range m.backend.Sessions() {
		if s.Project == name {
			return true
		}
	}
	return false
}

// nextNonEmptyProject returns the index to land on when cycling projects in
// the given direction (+1/-1), skipping projects with no sessions. If every
// other project is empty and the current one has sessions, it stays put. If
// every project including the current one is empty, it takes one step, so
// cycling never gets stuck — that's how you still reach an empty project on
// purpose.
func (m *Model) nextNonEmptyProject(dir int) int {
	n := len(m.projects)
	if n == 0 {
		return 0
	}
	i := m.activeProj
	for step := 0; step < n-1; step++ {
		i = (i + dir + n) % n
		if m.projectHasSessions(m.projects[i]) {
			return i
		}
	}
	if m.projectHasSessions(m.projects[m.activeProj]) {
		return m.activeProj
	}
	return (m.activeProj + dir + n) % n
}

// archivedCount returns how many archived sessions the active project has.
func (m *Model) archivedCount() int {
	if len(m.projects) == 0 {
		return 0
	}
	proj := m.projects[m.activeProj]
	n := 0
	for _, s := range m.backend.Sessions() {
		if s.Project == proj && s.Archived {
			n++
		}
	}
	return n
}

func (m *Model) refreshSessions() {
	if len(m.projects) == 0 {
		m.sessions = nil
		return
	}
	var selectedID string
	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		selectedID = m.sessions[m.cursor].ID
	}

	// In the all-sessions view, projs is every project; otherwise it's just
	// the active one. Sessions with a live tmux window float to the top
	// across the whole list regardless of project, with project order as
	// the tiebreaker among sessions sharing the same status — that
	// tiebreaker is also what keeps a single project's view (projs has one
	// entry, so it never affects ordering) in its existing (Order-based)
	// place.
	projs := m.projects
	if !m.allSessions {
		projs = m.projects[m.activeProj : m.activeProj+1]
	}
	projIndex := make(map[string]int, len(projs))
	for i, p := range projs {
		projIndex[p] = i
	}
	all := m.backend.Sessions()
	out := make([]session.Session, 0, len(all))
	for _, proj := range projs {
		for _, s := range all {
			if s.Project == proj && s.Archived == m.showArchived {
				out = append(out, s)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ai, aj := m.tmuxAlive[out[i].ID], m.tmuxAlive[out[j].ID]; ai != aj {
			return ai && !aj
		}
		return projIndex[out[i].Project] < projIndex[out[j].Project]
	})
	m.sessions = out

	if selectedID != "" {
		for i, s := range m.sessions {
			if s.ID == selectedID {
				m.cursor = i
				break
			}
		}
	}
	if m.cursor >= len(m.sessions) {
		if len(m.sessions) == 0 {
			m.cursor = 0
		} else {
			m.cursor = len(m.sessions) - 1
		}
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(listenStatus(m.statusCh), tickFlash())
}

func listenStatus(ch <-chan watcher.Snapshot) tea.Cmd {
	return func() tea.Msg {
		snap, ok := <-ch
		if !ok {
			return StatusChannelClosedMsg{}
		}
		return StatusTickMsg{Snap: snap}
	}
}

func tickFlash() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return InfoMsg{When: t} })
}

const (
	infoFlashDuration  = 3 * time.Second
	errorFlashDuration = 8 * time.Second
)

func (m *Model) setFlash(kind, text string) {
	m.flash = text
	m.flashKind = kind
	m.flashTime = time.Now()
}

func (m *Model) setError(err error) {
	m.setFlash("error", err.Error())
}
