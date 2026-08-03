// Command uishot renders a moomux TUI screen to stdout as raw ANSI, using a
// fake backend and canned sample data — no real projects, git repos, or tmux
// sessions required. Pair it with scripts/screenshot.sh, which wraps the
// ANSI capture in a pty (so lipgloss emits color), converts it to HTML, and
// renders that HTML to a PNG with a headless browser.
//
// See CLAUDE.md ("UI changes") for when to run this.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/gitwt"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/tui"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

// screens maps a scenario name to the key sequence that drives a freshly
// created Model from the list screen into that scenario. "list" needs no
// keys. Each entry is sent as a tea.KeyMsg: the named keys below map to
// their special tea.KeyType, anything else is typed as literal runes (so a
// whole word like "demo/Documents/foo" types into the focused input in one
// step).
var screens = map[string][]string{
	"list":        {},
	"new-session": {"n"},
	// Adding/editing a project only happens inside the picker now (P/E were
	// removed from the main list), so these open it first.
	"new-project":    {"/", "n"},
	"tag":            {"t"},
	"project-picker": {"/"},
	"edit-session":   {"e"},
	"edit-project":   {"/", "e"},
	// 3 tabs walks focus from repo (1) through base branch (2) and branch
	// prefix (3) to land on the emoji selector, showing its focused/[glyph]
	// state rather than the unfocused default the plain "edit-project" and
	// "new-project" scenarios capture.
	"edit-project-emoji": {"/", "e", "tab", "tab", "tab"},
	"confirm-delete":     {"d"},
	// "demo" has sample sessions, so D there flashes the blocked error; the
	// confirm screen is only reachable on the sessionless "spare" project
	// (tab switches to it).
	"confirm-delete-project": {"tab", "D"},
	"delete-project-blocked": {"D"},
	"archived":               {"A"},
	"all-sessions":           {"G"},
	"all-archived":           {"G", "A"},
	"help":                   {"?"},
	// needs-input has no keys of its own; renderScreen feeds it a
	// StatusTickMsg marking the first sample session watcher.NeedsInput.
	"needs-input": {},
	// Submits the new-project form with a path under ~/Documents that isn't
	// a git repo, landing on the "skip git" choice screen with its macOS
	// Files-and-Folders warning (see internal/tui/tcc.go). "$HOME" is
	// expanded to the real home dir at runtime so the warning actually
	// triggers regardless of machine. "ctrl+u" clears each field's cwd
	// prefill (see newProjectForm) before typing over it.
	"project-init-choice": {"/", "n", "ctrl+u", "demo2", "tab", "ctrl+u", "$HOME/Documents/projects", "enter"},
	// no-projects-startup is the actual first screen a zero-projects config
	// renders (tui.New's zero-projects branch auto-opens the add-project
	// form before any key is pressed) — no keys, since that's the point.
	"no-projects-startup": {},
	// no-projects starts from the same zero-projects config; esc backs out
	// of that auto-opened form to the empty list, to show its own
	// empty-state hint (for whoever backs out without adding one yet).
	"no-projects": {"esc"},
	// project-picker-emptied deletes the only (sessionless) project from
	// inside the picker itself, landing back on the picker's own "no
	// projects yet" render — a path the shared demo/spare sample data can't
	// reach (demo always has sessions, so it can never pass the delete
	// guard), hence the dedicated single-project config below.
	"project-picker-emptied": {"/", "d", "y"},
}

var namedKeys = map[string]tea.KeyType{
	"tab":       tea.KeyTab,
	"shift+tab": tea.KeyShiftTab,
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEsc,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"ctrl+u":    tea.KeyCtrlU,
}

func keyMsgFor(s string) tea.KeyMsg {
	if kt, ok := namedKeys[s]; ok {
		return tea.KeyMsg{Type: kt}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// drive sends msg through Update, then synchronously runs any returned
// tea.Cmd and feeds its resulting message back in — needed for scenarios
// like project-init-choice where the form submission dispatches an async
// backend call (AddProject) whose result message drives the mode switch.
func drive(m *tui.Model, msg tea.Msg) {
	_, cmd := m.Update(msg)
	runCmd(m, cmd)
}

func runCmd(m *tui.Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmd(m, c)
		}
		return
	}
	_, next := m.Update(msg)
	runCmd(m, next)
}

type fakeBackend struct {
	sessions []session.Session
	// cfg, when set, is mutated by RemoveProject so scenarios that need to
	// screenshot the state *after* a removal (e.g. project-picker-emptied)
	// see it reflected in m.projects on the next refresh — every other
	// backend method here is a no-op since no other scenario needs its
	// project/session data to actually change.
	cfg *config.Config
}

func (f *fakeBackend) CreateSession(project, name, agent, existingBranch, ticket string) (session.Session, string, error) {
	return session.Session{}, "", nil
}
func (f *fakeBackend) OpenSession(id string) (string, error)                   { return "", nil }
func (f *fakeBackend) DeleteSession(id string) error                           { return nil }
func (f *fakeBackend) KillTmux(id string) error                                { return nil }
func (f *fakeBackend) SetSessionStatusTitle(id string, st watcher.State) error { return nil }
func (f *fakeBackend) MoveSession(id string, delta int) error                  { return nil }
func (f *fakeBackend) MoveProject(name string, delta int) error                { return nil }
func (f *fakeBackend) SetSessionTags(id, ticket, pr string) (session.Session, error) {
	return session.Session{}, nil
}
func (f *fakeBackend) SetSessionAgent(id, agent string) (session.Session, error) {
	return session.Session{}, nil
}
func (f *fakeBackend) SetSessionArchived(id string, archived bool) (session.Session, error) {
	return session.Session{}, nil
}

// TmuxAliveAll reports every sample session as alive so effectiveState
// doesn't force them all to "parked" — that would hide whatever State a
// scenario sets via a StatusTickMsg (see the "needs-input" scenario).
func (f *fakeBackend) TmuxAliveAll() map[string]bool {
	alive := make(map[string]bool, len(f.sessions))
	for _, s := range f.sessions {
		alive[s.ID] = true
	}
	return alive
}
func (f *fakeBackend) Sessions() []session.Session                           { return f.sessions }
func (f *fakeBackend) Projects() []string                                    { return nil }
func (f *fakeBackend) AddProject(name string, p config.Project) error        { return gitwt.ErrNotGitRepo }
func (f *fakeBackend) InitProjectAndAdd(name string, p config.Project) error { return nil }
func (f *fakeBackend) AddPlainProject(name string, p config.Project) error   { return nil }
func (f *fakeBackend) UpdateProject(name string, p config.Project) error     { return nil }
func (f *fakeBackend) RemoveProject(name string) error {
	if f.cfg != nil {
		delete(f.cfg.Projects, name)
	}
	return nil
}

func sampleSessions() []session.Session {
	now := time.Now().UTC()
	return []session.Session{
		{
			ID:           "demo:feature-auth",
			Project:      "demo",
			Name:         "feature-auth",
			Branch:       "feature/auth",
			WorktreePath: "/tmp/demo/feature-auth",
			TmuxSession:  "moomux-feature-auth",
			CreatedAt:    now,
			Agent:        "claude",
			Ticket:       "https://tracker.example/TICK-123",
		},
		{
			ID:           "demo:bugfix-timeout",
			Project:      "demo",
			Name:         "bugfix-timeout",
			Branch:       "bugfix/timeout",
			WorktreePath: "/tmp/demo/bugfix-timeout",
			TmuxSession:  "moomux-bugfix-timeout",
			CreatedAt:    now,
			Agent:        "codex",
			PR:           "https://github.com/example/repo/pull/42",
		},
		{
			ID:           "demo:old-spike",
			Project:      "demo",
			Name:         "old-spike",
			Branch:       "spike/old-idea",
			WorktreePath: "/tmp/demo/old-spike",
			TmuxSession:  "moomux-old-spike",
			CreatedAt:    now,
			Agent:        "claude",
			Archived:     true,
		},
	}
}

// renderScreen drives a freshly created Model through the key sequence
// registered for screenName against canned sample data, returning its final
// rendered view. It's the piece scripts/screenshot.sh's pty/HTML/Chromium
// pipeline wraps, and the piece that's practical to cover with a Go test.
func renderScreen(screenName string, width, height int) (string, error) {
	keys, ok := screens[screenName]
	if !ok {
		return "", fmt.Errorf("unknown screen %q (want one of: %s)", screenName, screenNames())
	}

	cfg := &config.Config{Projects: map[string]config.Project{
		"demo": {
			Kind: "git", Repo: "/tmp/demo", BaseBranch: "main",
			BranchPrefix: "feature", Agent: "codex",
		},
		"spare": {
			Kind: "git", Repo: "/tmp/spare", BaseBranch: "main",
			BranchPrefix: "feature", Agent: "claude",
		},
	}}
	sessions := sampleSessions()
	switch screenName {
	case "no-projects-startup", "no-projects":
		cfg = &config.Config{Projects: map[string]config.Project{}}
		sessions = nil
	case "project-picker-emptied":
		cfg = &config.Config{Projects: map[string]config.Project{
			"solo": {Kind: "git", Repo: "/tmp/solo", BaseBranch: "main"},
		}}
		sessions = nil
	}
	be := &fakeBackend{sessions: sessions, cfg: cfg}
	statusCh := make(chan watcher.Snapshot)
	m := tui.New(cfg, be, statusCh, func() {})

	home, _ := os.UserHomeDir()

	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if screenName == "needs-input" {
		// Update, not drive(): the returned cmd re-arms listenStatus(statusCh),
		// which blocks forever reading a channel nothing here ever sends on.
		m.Update(tui.StatusTickMsg{Snap: watcher.Snapshot{
			States: map[string]watcher.State{sessions[0].WorktreePath: watcher.NeedsInput},
		}})
	}
	for _, k := range keys {
		drive(m, keyMsgFor(strings.ReplaceAll(k, "$HOME", home)))
	}

	return m.View(), nil
}

func main() {
	screen := flag.String("screen", "list", fmt.Sprintf("screen to render: %s", screenNames()))
	width := flag.Int("width", 100, "terminal width")
	height := flag.Int("height", 32, "terminal height")
	flag.Parse()

	out, err := renderScreen(*screen, *width, *height)
	if err != nil {
		fmt.Fprintf(os.Stderr, "uishot: %s\n", err)
		os.Exit(1)
	}

	fmt.Print(out)
}

func screenNames() string {
	names := make([]string, 0, len(screens))
	for name := range screens {
		names = append(names, name)
	}
	sort.Strings(names)
	out := ""
	for i, name := range names {
		if i > 0 {
			out += ", "
		}
		out += name
	}
	return out
}
