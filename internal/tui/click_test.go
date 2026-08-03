package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

type fakeBackend struct {
	sessions []session.Session

	moveSessionCalls []moveSessionCall
	moveSessionErr   error

	moveProjectCalls []moveProjectCall
	moveProjectErr   error

	createCalls []createCall
	createErr   error
	createHint  string

	deleteCalls []string
	deleteErr   error

	openCalls []string
	openErr   error
	openHint  string

	killCalls []string

	tagCalls []tagCall
	tagErr   error

	sessionAgentCalls []sessionAgentCall
	sessionAgentErr   error

	archiveCalls []archiveCall
	archiveErr   error

	addProjectCalls  []projectCall
	addProjectErr    error
	initProjectCalls []projectCall
	initProjectErr   error
	plainCalls       []projectCall
	plainErr         error

	updateProjectCalls []projectCall
	updateProjectErr   error

	removeProjectCalls []string
	removeProjectErr   error
}

type createCall struct{ project, name, agent, branch, ticket string }
type tagCall struct{ id, ticket, pr string }
type sessionAgentCall struct{ id, agent string }
type archiveCall struct {
	id       string
	archived bool
}
type projectCall struct {
	name string
	p    config.Project
}

type moveSessionCall struct {
	id    string
	delta int
}

type moveProjectCall struct {
	name  string
	delta int
}

func (f *fakeBackend) CreateSession(project, name, agent, existingBranch, ticket string) (session.Session, string, error) {
	f.createCalls = append(f.createCalls, createCall{project, name, agent, existingBranch, ticket})
	if f.createErr != nil {
		return session.Session{}, "", f.createErr
	}
	label := name
	if label == "" {
		label = existingBranch
	}
	s := session.Session{ID: session.MakeID(project, label), Project: project, Name: label, Agent: agent, Ticket: ticket}
	f.sessions = append(f.sessions, s)
	return s, f.createHint, nil
}
func (f *fakeBackend) OpenSession(id string) (string, error) {
	f.openCalls = append(f.openCalls, id)
	return f.openHint, f.openErr
}
func (f *fakeBackend) DeleteSession(id string) error {
	f.deleteCalls = append(f.deleteCalls, id)
	return f.deleteErr
}
func (f *fakeBackend) KillTmux(id string) error {
	f.killCalls = append(f.killCalls, id)
	return nil
}
func (f *fakeBackend) SetSessionStatusTitle(id string, st watcher.State) error { return nil }
func (f *fakeBackend) SetSessionTags(id, ticket, pr string) (session.Session, error) {
	f.tagCalls = append(f.tagCalls, tagCall{id, ticket, pr})
	if f.tagErr != nil {
		return session.Session{}, f.tagErr
	}
	for i, s := range f.sessions {
		if s.ID == id {
			f.sessions[i].Ticket, f.sessions[i].PR = ticket, pr
			return f.sessions[i], nil
		}
	}
	return session.Session{ID: id, Ticket: ticket, PR: pr}, nil
}
func (f *fakeBackend) SetSessionAgent(id, agent string) (session.Session, error) {
	f.sessionAgentCalls = append(f.sessionAgentCalls, sessionAgentCall{id, agent})
	if f.sessionAgentErr != nil {
		return session.Session{}, f.sessionAgentErr
	}
	for i, s := range f.sessions {
		if s.ID == id {
			f.sessions[i].Agent = agent
			return f.sessions[i], nil
		}
	}
	return session.Session{ID: id, Agent: agent}, nil
}
func (f *fakeBackend) SetSessionArchived(id string, archived bool) (session.Session, error) {
	f.archiveCalls = append(f.archiveCalls, archiveCall{id, archived})
	if f.archiveErr != nil {
		return session.Session{}, f.archiveErr
	}
	for i, s := range f.sessions {
		if s.ID == id {
			f.sessions[i].Archived = archived
			return f.sessions[i], nil
		}
	}
	return session.Session{ID: id, Archived: archived}, nil
}
func (f *fakeBackend) MoveSession(id string, delta int) error {
	f.moveSessionCalls = append(f.moveSessionCalls, moveSessionCall{id: id, delta: delta})
	return f.moveSessionErr
}
func (f *fakeBackend) MoveProject(name string, delta int) error {
	f.moveProjectCalls = append(f.moveProjectCalls, moveProjectCall{name: name, delta: delta})
	return f.moveProjectErr
}
func (f *fakeBackend) TmuxAliveAll() map[string]bool { return map[string]bool{} }
func (f *fakeBackend) Sessions() []session.Session   { return f.sessions }
func (f *fakeBackend) Projects() []string            { return nil }
func (f *fakeBackend) AddProject(name string, p config.Project) error {
	f.addProjectCalls = append(f.addProjectCalls, projectCall{name, p})
	return f.addProjectErr
}
func (f *fakeBackend) InitProjectAndAdd(name string, p config.Project) error {
	f.initProjectCalls = append(f.initProjectCalls, projectCall{name, p})
	return f.initProjectErr
}
func (f *fakeBackend) AddPlainProject(name string, p config.Project) error {
	f.plainCalls = append(f.plainCalls, projectCall{name, p})
	return f.plainErr
}
func (f *fakeBackend) UpdateProject(name string, p config.Project) error {
	f.updateProjectCalls = append(f.updateProjectCalls, projectCall{name, p})
	return f.updateProjectErr
}
func (f *fakeBackend) RemoveProject(name string) error {
	f.removeProjectCalls = append(f.removeProjectCalls, name)
	return f.removeProjectErr
}

// TestLinkHitsResolveClicks renders a full frame and asserts that clicking
// on the printed ticket/PR icon glyphs resolves to the session's URLs, and
// that clicking one column outside the icon range does not.
func TestLinkHitsResolveClicks(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{"demo": {Repo: "/tmp/demo"}}}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:one", Project: "demo", Name: "one", Ticket: "https://ticket.example/1", PR: "https://pr.example/1"},
	}}
	statusCh := make(chan watcher.Snapshot)
	m := New(cfg, be, statusCh, func() {})
	m.width, m.height = 80, 24

	frame := m.View()
	lines := strings.Split(frame, "\n")

	findCol := func(icon string) (line, col int) {
		for li, l := range lines {
			if idx := strings.Index(l, icon); idx >= 0 {
				return li, lipgloss.Width(l[:idx])
			}
		}
		t.Fatalf("icon %q not found in rendered frame:\n%s", icon, frame)
		return -1, -1
	}

	ticketLine, ticketCol := findCol(iconTicket)
	prLine, prCol := findCol(iconPR)

	if got := m.linkAt(ticketCol, ticketLine); got != be.sessions[0].Ticket {
		t.Errorf("click on ticket icon at (%d,%d) = %q, want %q", ticketCol, ticketLine, got, be.sessions[0].Ticket)
	}
	if got := m.linkAt(prCol, prLine); got != be.sessions[0].PR {
		t.Errorf("click on pr icon at (%d,%d) = %q, want %q", prCol, prLine, got, be.sessions[0].PR)
	}
	if got := m.linkAt(ticketCol-1, ticketLine); got != "" {
		t.Errorf("click one column left of ticket icon resolved to %q, want empty", got)
	}
}

func TestTruncatedDetailURLsRemainClickable(t *testing.T) {
	ticketURL := "https://tickets.example.com/org/project/issues/12345"
	prURL := "https://github.com/org/project/pull/67890"
	cfg := &config.Config{Projects: map[string]config.Project{"demo": {Repo: "/tmp/demo"}}}
	be := &fakeBackend{sessions: []session.Session{
		{
			ID:      "demo:one",
			Project: "demo",
			Name:    "one",
			Ticket:  ticketURL,
			PR:      prURL,
		},
	}}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24

	frame := m.View()
	lines := strings.Split(frame, "\n")

	assertLink := func(visibleTail, wantURL string) {
		t.Helper()
		for line, rendered := range lines {
			if idx := strings.Index(rendered, visibleTail); idx >= 0 {
				col := lipgloss.Width(rendered[:idx])
				if got := m.linkAt(col, line); got != wantURL {
					t.Fatalf("click on detail URL tail %q at (%d,%d) = %q, want %q\n%s", visibleTail, col, line, got, wantURL, frame)
				}
				return
			}
		}
		t.Fatalf("detail URL tail %q not found:\n%s", visibleTail, frame)
	}

	assertLink("issues/12345", ticketURL)
	assertLink("pull/67890", prURL)
}

func TestClippedDetailURLsDoNotLeaveClickTargets(t *testing.T) {
	m := newTestModel(&fakeBackend{sessions: []session.Session{
		{
			ID:      "demo:one",
			Project: "demo",
			Name:    "one",
			Ticket:  "https://tickets.example/123",
			PR:      "https://github.com/example/repo/pull/456",
		},
	}})

	_, clippedHits := m.renderDetail(36, 5)
	if len(clippedHits) != 0 {
		t.Fatalf("clipped detail returned link hits: %+v", clippedHits)
	}

	_, visibleHits := m.renderDetail(36, 10)
	if len(visibleHits) != 2 {
		t.Fatalf("visible detail returned %d link hits, want 2: %+v", len(visibleHits), visibleHits)
	}
}

// TestLinkClickOverSSHCopiesInsteadOfOpening asserts that clicking a
// ticket/PR icon while browser.Remote() is true copies the URL to the
// clipboard (via OSC 52) instead of shelling out to `open` — since `open`
// would launch a browser on the remote machine rather than the user's own,
// and moomux's mouse tracking means the terminal never gets a chance to
// handle the tap as a link itself. The copy happens synchronously inside
// Update() rather than via a returned tea.Cmd: a Cmd runs in its own
// goroutine concurrently with bubbletea's render loop, and both writing to
// os.Stdout at once can interleave and corrupt the OSC 52 escape sequence
// before the terminal ever sees a well-formed one.
func TestLinkClickOverSSHCopiesInsteadOfOpening(t *testing.T) {
	t.Setenv("SSH_TTY", "/dev/ttys001")

	cfg := &config.Config{Projects: map[string]config.Project{"demo": {Repo: "/tmp/demo"}}}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:one", Project: "demo", Name: "one", Ticket: "https://ticket.example/1"},
	}}
	statusCh := make(chan watcher.Snapshot)
	m := New(cfg, be, statusCh, func() {})
	m.width, m.height = 80, 24
	m.View() // populate m.linkHits

	hit := m.linkHits[0]
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: hit.x0, Y: hit.y})
	m2 := updated.(*Model)

	if cmd != nil {
		t.Errorf("expected no async command for the copy path, got one")
	}
	if m2.flashKind != "info" || m2.flash != "copied "+be.sessions[0].Ticket {
		t.Errorf("flash = (%q, %q), want (\"info\", %q)", m2.flashKind, m2.flash, "copied "+be.sessions[0].Ticket)
	}
}

// TestRemoteLinksToggleOverridesAutoDetection covers the R toggle: since
// transports like mosh set none of SSH_TTY/SSH_CONNECTION/SSH_CLIENT,
// browser.Remote()'s auto-detection has no signal for them, so a user needs
// to be able to force copy mode from inside the running session.
func TestRemoteLinksToggleOverridesAutoDetection(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{"demo": {Repo: "/tmp/demo"}}}
	be := &fakeBackend{}
	statusCh := make(chan watcher.Snapshot)
	m := New(cfg, be, statusCh, func() {})

	// No SSH env set and not forced: isRemote() says false.
	if m.isRemote() {
		t.Errorf("with no SSH env and forceCopyLinks off: isRemote() = true, want false")
	}

	// Toggle on: forces isRemote() to true even with no SSH env.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = updated.(*Model)
	if !m.forceCopyLinks || !m.isRemote() {
		t.Errorf("after one R press: forceCopyLinks = %v, isRemote() = %v, want true/true", m.forceCopyLinks, m.isRemote())
	}

	// Toggle off again: falls back to auto-detection.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	m = updated.(*Model)
	if m.forceCopyLinks || m.isRemote() {
		t.Errorf("after two R presses: forceCopyLinks = %v, isRemote() = %v, want false/false", m.forceCopyLinks, m.isRemote())
	}
}
