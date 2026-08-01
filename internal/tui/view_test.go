package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/erickgnclvs/moomux/internal/session"
)

func layoutTestModel(sessionCount int) *Model {
	sessions := make([]session.Session, sessionCount)
	for i := range sessions {
		name := fmt.Sprintf("session-%02d", i)
		sessions[i] = session.Session{
			ID:           "demo:" + name,
			Project:      "demo",
			Name:         name,
			WorktreePath: "/tmp/" + name,
			TmuxSession:  name,
		}
	}
	return newTestModel(&fakeBackend{sessions: sessions})
}

func TestNarrowShortLayoutShowsOnlySessionList(t *testing.T) {
	m := layoutTestModel(3)
	m.width, m.height = 50, 24

	view := m.View()

	if !strings.Contains(view, "SESSIONS") {
		t.Fatalf("short narrow view does not contain session list:\n%s", view)
	}
	if strings.Contains(view, "DETAIL") {
		t.Fatalf("short narrow view unexpectedly contains detail pane:\n%s", view)
	}
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("rendered height = %d, terminal height = %d", got, m.height)
	}
}

func TestNarrowHeaderHidesWordmark(t *testing.T) {
	m := layoutTestModel(1)

	m.width = narrowWidthBreak - 1
	if header := m.renderHeader(); strings.Contains(header, "moomux") {
		t.Fatalf("narrow header unexpectedly contains wordmark:\n%s", header)
	}

	m.width = narrowWidthBreak
	if header := m.renderHeader(); !strings.Contains(header, "moomux") {
		t.Fatalf("wide header does not contain wordmark:\n%s", header)
	}
}

func TestNarrowHeaderShowsOnlyCurrentProject(t *testing.T) {
	m := layoutTestModel(1)
	m.width = 50
	m.projects = []string{"alpha", "beta", "gamma"}
	m.activeProj = 1

	header := m.renderHeader()

	if !strings.Contains(header, "beta") {
		t.Fatalf("narrow header does not contain current project:\n%s", header)
	}
	if strings.Contains(header, "alpha") || strings.Contains(header, "gamma") {
		t.Fatalf("narrow header contains inactive projects:\n%s", header)
	}
	if got := lipgloss.Width(header); got > m.width {
		t.Fatalf("header width = %d, terminal width = %d:\n%s", got, m.width, header)
	}
}

func TestNarrowHeaderTruncatesLongCurrentProject(t *testing.T) {
	m := layoutTestModel(1)
	m.width = 24
	m.projects = []string{"a-very-long-current-project-name"}
	m.activeProj = 0

	header := m.renderHeader()

	if got := lipgloss.Width(header); got > m.width {
		t.Fatalf("header width = %d, terminal width = %d:\n%s", got, m.width, header)
	}
	if !strings.Contains(header, "…") {
		t.Fatalf("long current project was not truncated:\n%s", header)
	}
}

func TestNarrowDetailKeepsEndOfWorktreePath(t *testing.T) {
	m := layoutTestModel(1)
	m.sessions[0].WorktreePath = "/Users/example/Development/moomux/feature-right-end"

	detail, _ := m.renderDetail(30, 20)

	if !strings.Contains(detail, "right-end") {
		t.Fatalf("detail does not show the useful end of the worktree path:\n%s", detail)
	}
	if !strings.Contains(detail, "…") {
		t.Fatalf("truncated worktree path has no leading ellipsis:\n%s", detail)
	}
}

func TestNarrowProjectEditKeepsEndOfRepoInput(t *testing.T) {
	m := layoutTestModel(1)
	m.width, m.height = 24, 16
	project := m.cfg.Projects["demo"]
	project.Repo = "/Users/example/Development/moomux/repo-XYZ"
	m.cfg.Projects["demo"] = project
	m.editProjectName = "demo"
	m.projForm = editProjectForm("demo", project)
	m.mode = ModeEditProject
	m.resizeFormInputs()

	view := m.View()

	if !strings.Contains(view, "XYZ") {
		t.Fatalf("project editor does not show the end of the repo input:\n%s", view)
	}
	if !strings.Contains(view, "repo:") {
		t.Fatalf("project editor scrolled past the focused repo row:\n%s", view)
	}
	if !strings.Contains(view, "╮") || !strings.Contains(view, "╯") {
		t.Fatalf("narrow project editor clipped its right border:\n%s", view)
	}
}

func TestNarrowProjectEditShortRepoPreservesFrame(t *testing.T) {
	m := layoutTestModel(1)
	m.width, m.height = 24, 12
	project := m.cfg.Projects["demo"]
	project.Repo = "/tmp/demo"
	m.editProjectName = "demo"
	m.projForm = editProjectForm("demo", project)
	m.mode = ModeEditProject
	m.resizeFormInputs()

	view := m.View()

	if !strings.Contains(view, "╮") || !strings.Contains(view, "╯") {
		t.Fatalf("short repo input clipped the narrow dialog frame:\n%s", view)
	}
}

func TestNarrowEditSessionShowsCompactSelectedAgent(t *testing.T) {
	m := layoutTestModel(1)
	m.width, m.height = 24, 12
	m.mode = ModeEditSession
	m.sessionForm = sessionForm{
		project:  "demo",
		name:     "session",
		agentIdx: 2,
	}

	view := m.View()

	if !strings.Contains(view, "[opencode]") {
		t.Fatalf("narrow session editor does not show selected agent:\n%s", view)
	}
	if !strings.Contains(view, "╮") || !strings.Contains(view, "╯") {
		t.Fatalf("narrow session editor clipped its right border:\n%s", view)
	}
}

func TestNarrowTallLayoutShowsStackedDetail(t *testing.T) {
	m := layoutTestModel(3)
	m.width, m.height = 50, 40

	view := m.View()

	if !strings.Contains(view, "SESSIONS") || !strings.Contains(view, "DETAIL") {
		t.Fatalf("tall narrow view should contain both panes:\n%s", view)
	}
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("rendered height = %d, terminal height = %d", got, m.height)
	}
}

func TestNarrowLayoutRestoresDetailAfterResize(t *testing.T) {
	m := layoutTestModel(3)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
	if view := m.View(); strings.Contains(view, "DETAIL") {
		t.Fatalf("detail visible before resize:\n%s", view)
	}

	m.Update(tea.WindowSizeMsg{Width: 50, Height: 40})
	if view := m.View(); !strings.Contains(view, "DETAIL") {
		t.Fatalf("detail not restored after resize:\n%s", view)
	}
}

func TestNarrowShortLayoutKeepsSelectedSessionVisible(t *testing.T) {
	m := layoutTestModel(20)
	m.width, m.height = 50, 16
	m.cursor = len(m.sessions) - 1

	view := m.View()

	if !strings.Contains(view, m.sessions[m.cursor].Name) {
		t.Fatalf("selected session %q not visible:\n%s", m.sessions[m.cursor].Name, view)
	}
}

func TestViewNeverExceedsTerminalHeight(t *testing.T) {
	for _, tc := range []struct {
		width  int
		height int
	}{
		{width: 50, height: 8},
		{width: 50, height: 16},
		{width: 50, height: 24},
		{width: 50, height: 40},
		{width: 80, height: 12},
		{width: 80, height: 24},
	} {
		t.Run(fmt.Sprintf("%dx%d", tc.width, tc.height), func(t *testing.T) {
			m := layoutTestModel(20)
			m.width, m.height = tc.width, tc.height

			if got := lipgloss.Height(m.View()); got > tc.height {
				t.Fatalf("rendered height = %d, terminal height = %d", got, tc.height)
			}
		})
	}
}

func TestOverlaysStayWithinKeyboardSizedViewport(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Model)
	}{
		{
			name: "new session",
			setup: func(m *Model) {
				m.mode = ModeNewForm
			},
		},
		{
			name: "new project",
			setup: func(m *Model) {
				m.mode = ModeNewProject
				m.projForm = newProjectForm()
			},
		},
		{
			name: "tag session",
			setup: func(m *Model) {
				m.mode = ModeTagForm
				m.tagForm = newTagForm("", "")
			},
		},
		{
			name: "help",
			setup: func(m *Model) {
				m.mode = ModeHelp
			},
		},
		{
			name: "edit session",
			setup: func(m *Model) {
				m.mode = ModeEditSession
				m.sessionForm = sessionForm{
					project:  "demo",
					name:     "session",
					agentIdx: 1,
				}
			},
		},
		{
			name: "edit project",
			setup: func(m *Model) {
				m.mode = ModeEditProject
				m.editProjectName = "demo"
				m.projForm = editProjectForm("demo", m.cfg.Projects["demo"])
			},
		},
	} {
		for _, size := range []struct {
			width  int
			height int
		}{
			{width: 24, height: 12},
			{width: 50, height: 12},
			{width: 50, height: 16},
		} {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size.width, size.height), func(t *testing.T) {
				m := layoutTestModel(1)
				m.width, m.height = size.width, size.height
				tc.setup(m)
				m.resizeFormInputs()

				view := m.View()
				if got := lipgloss.Width(view); got > m.width {
					t.Fatalf("rendered width = %d, terminal width = %d:\n%s", got, m.width, view)
				}
				if got := lipgloss.Height(view); got > m.height {
					t.Fatalf("rendered height = %d, terminal height = %d:\n%s", got, m.height, view)
				}
				if !strings.Contains(view, "esc") {
					t.Fatalf("essential escape control is not visible:\n%s", view)
				}
			})
		}
	}
}

func TestShortFormViewportKeepsFocusedInputVisible(t *testing.T) {
	m := layoutTestModel(1)
	m.width, m.height = 50, 12
	m.mode = ModeNewForm
	m.nameInput.SetValue("unique-name")
	m.branchInput.SetValue("unique-branch")
	m.ticketInput.SetValue("unique-ticket")
	m.resizeFormInputs()

	values := []string{"unique-name", "unique-branch", "unique-ticket"}
	hints := []string{"worktree folder", "existing branch", "clickable ticket"}
	for focus, value := range values {
		m.newFormBlurAll()
		m.newFormFocus = focus
		m.newFormFocusInput()

		view := m.View()
		if !strings.Contains(view, value) {
			t.Fatalf("focused input %d value %q is not visible:\n%s", focus, value, view)
		}
		if !strings.Contains(view, "esc cancel") {
			t.Fatalf("sticky form actions are not visible:\n%s", view)
		}
		if !strings.Contains(view, hints[focus]) {
			t.Fatalf("contextual hint %q is not visible:\n%s", hints[focus], view)
		}
	}
}

func TestShortProjectFormKeepsBottomControlVisible(t *testing.T) {
	m := layoutTestModel(1)
	m.width, m.height = 50, 12
	m.mode = ModeNewProject
	m.projForm = newProjectForm()
	m.projForm.focus = projFormInputCount + 1
	m.resizeFormInputs()

	view := m.View()

	if !strings.Contains(view, "[on]") {
		t.Fatalf("focused bottom control is not visible:\n%s", view)
	}
	if !strings.Contains(view, "esc cancel") {
		t.Fatalf("sticky project actions are not visible:\n%s", view)
	}
}

func TestManualFormScrollPersistsUntilFocusChanges(t *testing.T) {
	m := layoutTestModel(1)
	m.width, m.height = 50, 12
	m.mode = ModeNewForm

	m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	scrolled := m.overlayViewport.YOffset
	m.View()

	if scrolled == 0 {
		t.Fatal("form viewport did not scroll")
	}
	if got := m.overlayViewport.YOffset; got != scrolled {
		t.Fatalf("manual scroll was reset: before render=%d after=%d", scrolled, got)
	}
}

func TestConfirmDeleteOpensUnscrolled(t *testing.T) {
	// Scroll inside the help overlay, close it, then open the delete
	// confirmation: it must start at the top, showing what's being
	// deleted — not inherit the help overlay's scroll offset.
	m := layoutTestModel(1)
	m.width, m.height = 50, 12
	m.mode = ModeHelp

	m.View()
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.overlayViewport.YOffset == 0 {
		t.Fatal("help viewport did not scroll; scenario not set up")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if m.mode != ModeConfirmDelete {
		t.Fatalf("mode = %v, want ModeConfirmDelete", m.mode)
	}
	if got := m.overlayViewport.YOffset; got != 0 {
		t.Fatalf("confirm dialog opened pre-scrolled: YOffset = %d", got)
	}
}

func TestHelpOverlayScrollsWhileControlsRemainVisible(t *testing.T) {
	m := layoutTestModel(1)
	m.width, m.height = 50, 12
	m.mode = ModeHelp

	m.View() // populate the viewport content before sending scroll input
	before := m.overlayViewport.YOffset
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	view := m.View()

	if m.overlayViewport.YOffset <= before {
		t.Fatalf("help viewport did not scroll: before=%d after=%d", before, m.overlayViewport.YOffset)
	}
	if !strings.Contains(view, "?/esc close") {
		t.Fatalf("sticky help controls are not visible after scrolling:\n%s", view)
	}
}
