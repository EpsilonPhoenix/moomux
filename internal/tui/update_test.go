package tui

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/gitwt"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

func keyRune(r string) tea.KeyMsg                        { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)} }
func typeText(m *Model, s string)                        { m.Update(keyRune(s)) }
func press(m *Model, k tea.KeyType) (tea.Model, tea.Cmd) { return m.Update(tea.KeyMsg{Type: k}) }

// run executes a key press and, if it produced a command, feeds the resulting
// message back into Update — the synchronous equivalent of the Bubble Tea loop.
func run(m *Model, msg tea.Msg) {
	_, cmd := m.Update(msg)
	drainCmd(m, cmd)
}

func TestNewSessionFormFlow(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	if m.mode != ModeNewForm {
		t.Fatalf("mode = %v", m.mode)
	}
	if v := m.View(); !strings.Contains(v, "New session") && !strings.Contains(v, "session name") {
		t.Fatalf("new form view missing form copy:\n%s", v)
	}

	typeText(m, "myfeat")
	press(m, tea.KeyLeft) // cursor movement in the name input, NOT the agent selector
	typeText(m, "X")      // lands before the final rune, proving the cursor moved
	if got := m.nameInput.Value(); got != "myfeaXt" {
		t.Fatalf("left arrow did not move the text cursor: name = %q", got)
	}
	m.nameInput.SetValue("myfeat")
	m.nameInput.CursorEnd()
	press(m, tea.KeyTab) // -> branch
	for i := 0; i < 4; i++ {
		press(m, tea.KeyTab) // branch -> prompt -> ticket -> PR -> agent selector
	}
	press(m, tea.KeyRight)
	if agentChoices[m.newFormAgentIdx] != "codex" {
		t.Fatalf("agent = %q", agentChoices[m.newFormAgentIdx])
	}
	press(m, tea.KeyLeft)     // back to claude
	press(m, tea.KeyShiftTab) // agent -> PR
	press(m, tea.KeyShiftTab) // PR -> ticket
	typeText(m, "https://t/1")
	press(m, tea.KeyShiftTab)
	press(m, tea.KeyShiftTab) // -> branch again
	if m.newFormFocus != 2 {
		t.Fatalf("focus = %d", m.newFormFocus)
	}

	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.createCalls) != 1 {
		t.Fatalf("createCalls = %v", be.createCalls)
	}
	got := be.createCalls[0]
	if got.project != "demo" || got.name != "myfeat" || got.agent != "claude" || got.ticket != "https://t/1" {
		t.Fatalf("createCall = %+v", got)
	}
	if m.mode != ModeList || !strings.Contains(m.flash, "created myfeat") {
		t.Fatalf("mode=%v flash=%q", m.mode, m.flash)
	}
}

func TestNewSessionFormSendsFirstPrompt(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	typeText(m, "myfeat")
	for i := 0; i < 2; i++ {
		press(m, tea.KeyTab) // name -> branch -> prompt
	}
	typeText(m, "do the thing")

	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.createCalls) != 1 {
		t.Fatalf("createCalls = %v", be.createCalls)
	}
	if len(be.firstPromptCalls) != 1 || be.firstPromptCalls[0].prompt != "do the thing" {
		t.Fatalf("firstPromptCalls = %v", be.firstPromptCalls)
	}
}

// TestNewSessionFormPromptSupportsMultilineNavigation guards the prompt
// field's textarea behavior: ctrl+j inserts a newline (Enter is reserved for
// form submit) and, once a second line exists, up/down move the cursor
// between lines instead of leaving the field the way they do for every other
// row in the form.
func TestNewSessionFormPromptSupportsMultilineNavigation(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	typeText(m, "myfeat")
	for i := 0; i < 2; i++ {
		press(m, tea.KeyTab) // name -> branch -> prompt
	}
	if m.newFormFocus != 3 {
		t.Fatalf("focus = %d, want prompt field", m.newFormFocus)
	}

	typeText(m, "line one")
	press(m, tea.KeyCtrlJ)
	typeText(m, "line two")
	if got := m.promptInput.Value(); got != "line one\nline two" {
		t.Fatalf("promptInput value = %q", got)
	}

	press(m, tea.KeyUp)
	if m.newFormFocus != 3 {
		t.Fatalf("up arrow left the prompt field: focus = %d", m.newFormFocus)
	}
	press(m, tea.KeyDown)
	if m.newFormFocus != 3 {
		t.Fatalf("down arrow left the prompt field: focus = %d", m.newFormFocus)
	}

	// Every other field still cycles focus on up/down.
	press(m, tea.KeyTab) // prompt -> ticket
	if m.newFormFocus != 4 {
		t.Fatalf("focus = %d, want ticket field", m.newFormFocus)
	}
	press(m, tea.KeyDown)
	if m.newFormFocus != 5 {
		t.Fatalf("down arrow did not advance focus off the ticket field: focus = %d", m.newFormFocus)
	}
}

// TestNewSessionFormSurvivesPostCreatePRTagFailure guards against a
// SetSessionTags failure (PR field) discarding the fact that CreateSession
// already succeeded — the worktree and tmux session exist regardless, so
// the UI must still show the new session (via SessionCreatedMsg), just with
// a hint about the tag failure, not report the whole creation as failed.
func TestNewSessionFormSurvivesPostCreatePRTagFailure(t *testing.T) {
	be := &fakeBackend{tagErr: errors.New("tag boom")}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	typeText(m, "myfeat")
	for i := 0; i < 4; i++ {
		press(m, tea.KeyTab) // name -> branch -> prompt -> ticket -> PR
	}
	typeText(m, "https://github.com/x/y/pull/2")

	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.createCalls) != 1 {
		t.Fatalf("createCalls = %v", be.createCalls)
	}
	if m.mode != ModeList || !strings.Contains(m.flash, "created myfeat") {
		t.Fatalf("post-create tag failure must not be reported as a failed creation: mode=%v flash=%q", m.mode, m.flash)
	}
	if !strings.Contains(m.flash, "tag boom") {
		t.Fatalf("tag failure should still be surfaced as a hint: flash=%q", m.flash)
	}
	if len(m.sessions) != 1 {
		t.Fatalf("session list was not refreshed with the created session: %v", m.sessions)
	}
}

// TestNewSessionFormSurvivesPostCreateFirstPromptFailure is the same guard
// for StartFirstPrompt.
func TestNewSessionFormSurvivesPostCreateFirstPromptFailure(t *testing.T) {
	be := &fakeBackend{firstPromptErr: errors.New("prompt boom")}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	typeText(m, "myfeat")
	for i := 0; i < 2; i++ {
		press(m, tea.KeyTab) // name -> branch -> prompt
	}
	typeText(m, "do the thing")

	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ModeList || !strings.Contains(m.flash, "created myfeat") {
		t.Fatalf("post-create prompt failure must not be reported as a failed creation: mode=%v flash=%q", m.mode, m.flash)
	}
	if !strings.Contains(m.flash, "prompt boom") {
		t.Fatalf("prompt failure should still be surfaced as a hint: flash=%q", m.flash)
	}
}

// TestNewSessionFormClearsPRAndPromptOnReopen guards against a PR or first-
// prompt value typed into one session's form silently carrying over and
// getting submitted the next time the form is opened.
func TestNewSessionFormClearsPRAndPromptOnReopen(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	press(m, tea.KeyTab) // -> branch
	press(m, tea.KeyTab) // -> prompt
	typeText(m, "leftover prompt")
	for i := 0; i < 2; i++ {
		press(m, tea.KeyTab) // prompt -> ticket -> PR
	}
	typeText(m, "https://github.com/x/y/pull/2")
	press(m, tea.KeyEsc) // cancel without submitting

	m.Update(keyRune("n")) // reopen
	if v := m.prInput.Value(); v != "" {
		t.Fatalf("prInput carried over stale value %q into the reopened form", v)
	}
	if v := m.promptInput.Value(); v != "" {
		t.Fatalf("promptInput carried over stale value %q into the reopened form", v)
	}
}

func TestNewSessionFormAppendsTicketAndPRToFirstPrompt(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	typeText(m, "myfeat")
	press(m, tea.KeyTab) // -> branch
	press(m, tea.KeyTab) // -> prompt
	typeText(m, "do the thing")
	press(m, tea.KeyTab) // -> ticket
	typeText(m, "https://ticket.example/1")
	press(m, tea.KeyTab) // -> PR
	typeText(m, "https://github.com/x/y/pull/2")

	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.tagCalls) != 1 || be.tagCalls[0].ticket != "https://ticket.example/1" || be.tagCalls[0].pr != "https://github.com/x/y/pull/2" {
		t.Fatalf("tagCalls = %v", be.tagCalls)
	}
	want := "do the thing\n\nTicket: https://ticket.example/1\nPR: https://github.com/x/y/pull/2"
	if len(be.firstPromptCalls) != 1 || be.firstPromptCalls[0].prompt != want {
		t.Fatalf("firstPromptCalls = %v, want prompt %q", be.firstPromptCalls, want)
	}
}

// TestNewSessionCreateInFlightBlocksSecondForm guards against the race where
// submitting a second "new session" form while the first CreateSession call
// is still in flight (e.g. still creating the tmux session/terminal) lets
// both calls run concurrently before the first is registered in the store —
// producing two sessions that collide on the same tmux session name.
func TestNewSessionCreateInFlightBlocksSecondForm(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	typeText(m, "myfeat")
	_, cmd := press(m, tea.KeyEnter)
	if cmd == nil {
		t.Fatalf("expected a create command")
	}
	cmd() // runs backend.CreateSession, but the result message isn't fed back — call is still "in flight"
	if len(be.createCalls) != 1 || m.mode != ModeList || !m.busy {
		t.Fatalf("after first submit: calls=%v mode=%v busy=%v", be.createCalls, m.mode, m.busy)
	}

	m.Update(keyRune("n"))
	if m.mode != ModeList {
		t.Fatalf("second 'n' opened the form while a create was still in flight: mode=%v", m.mode)
	}
	if m.flashKind != "error" || !strings.Contains(m.flash, "still creating") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
	press(m, tea.KeyEnter)
	if len(be.createCalls) != 1 {
		t.Fatalf("createCalls = %v, want still 1 (second submit must not have fired)", be.createCalls)
	}
}

func TestNewSessionFormEmptySubmitIsNoop(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	m.Update(keyRune("n"))
	press(m, tea.KeyEnter)
	if len(be.createCalls) != 0 || m.mode != ModeNewForm {
		t.Fatalf("calls=%v mode=%v", be.createCalls, m.mode)
	}
	// The rejection must be visible in the form itself, not a discarded flash.
	if v := m.View(); !strings.Contains(v, "session name or an existing branch") {
		t.Fatalf("empty-submit error not rendered:\n%s", v)
	}
	press(m, tea.KeyEsc)
	if m.mode != ModeList {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestNewSessionFormAgentRequiredErrorRendered(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	m.cfg.Projects["demo"] = config.Project{Repo: "/repo", PromptAgent: true}
	m.Update(keyRune("n"))
	if m.newFormAgentIdx != -1 {
		t.Fatalf("agentIdx = %d, want -1 for prompt_agent project", m.newFormAgentIdx)
	}
	typeText(m, "myfeat")
	press(m, tea.KeyEnter)
	if len(be.createCalls) != 0 {
		t.Fatalf("createCalls = %v", be.createCalls)
	}
	if v := m.View(); !strings.Contains(v, "requires choosing an agent") {
		t.Fatalf("agent-required error not rendered:\n%s", v)
	}
}

func TestNewSessionFormForcesProjectChoiceWithMultipleProjects(t *testing.T) {
	be := &fakeBackend{}
	m := newMultiProjectTestModel(be) // projects: alpha, beta
	m.Update(keyRune("n"))
	if m.newFormProjIdx != -1 {
		t.Fatalf("projIdx = %d, want -1 with more than one project", m.newFormProjIdx)
	}
	if m.newFormFocus != newFormProjFocus {
		t.Fatalf("focus = %d, want to start on the project selector", m.newFormFocus)
	}
	press(m, tea.KeyEnter)
	if len(be.createCalls) != 0 {
		t.Fatalf("createCalls = %v", be.createCalls)
	}
	if v := m.View(); !strings.Contains(v, "choose a project") {
		t.Fatalf("project-required error not rendered:\n%s", v)
	}

	press(m, tea.KeyRight)
	if m.projects[m.newFormProjIdx] != "alpha" {
		t.Fatalf("projIdx = %d, want alpha", m.newFormProjIdx)
	}
	press(m, tea.KeyTab) // -> name
	typeText(m, "myfeat")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.createCalls) != 1 || be.createCalls[0].project != "alpha" {
		t.Fatalf("createCalls = %v", be.createCalls)
	}
}

func TestNewSessionFormDefaultsProjectWithSingleProject(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be) // single project: demo
	m.Update(keyRune("n"))
	if m.newFormProjIdx != 0 {
		t.Fatalf("projIdx = %d, want 0 with a single project", m.newFormProjIdx)
	}
}

func TestNewSessionCreateErrorFlashes(t *testing.T) {
	be := &fakeBackend{createErr: errors.New("boom")}
	m := newTestModel(be)
	m.Update(keyRune("n"))
	typeText(m, "x")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.flashKind != "error" || !strings.Contains(m.flash, "boom") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
}

func TestNewSessionNoProjectsFlashesError(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{}}
	be := &fakeBackend{}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24
	m.mode = ModeList // New() opens the project form when no projects exist
	m.Update(keyRune("n"))
	if m.flashKind != "error" || !strings.Contains(m.flash, "no projects") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
}

func TestConfirmDeleteFlow(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}}}
	m := newTestModel(be)

	run(m, keyRune("d"))
	if m.mode != ModeConfirmDelete {
		t.Fatalf("mode = %v", m.mode)
	}
	if v := m.View(); !strings.Contains(strings.ToLower(v), "delete") {
		t.Fatalf("confirm view:\n%s", v)
	}

	// 'n' backs out without deleting
	m.Update(keyRune("n"))
	if m.mode != ModeList || len(be.deleteCalls) != 0 {
		t.Fatalf("mode=%v deletes=%v", m.mode, be.deleteCalls)
	}

	run(m, keyRune("d"))
	run(m, keyRune("y"))
	if len(be.deleteCalls) != 1 || be.deleteCalls[0] != "demo:a" {
		t.Fatalf("deleteCalls = %v", be.deleteCalls)
	}
	if !strings.Contains(m.flash, "deleted") {
		t.Fatalf("flash = %q", m.flash)
	}
}

// TestConfirmDeleteOpensInstantlyThenUpdatesFromLiveCheck guards the whole
// point of caching: the dialog must render immediately on whatever's already
// in m.gitStatus (even nothing), not block on a fresh WorktreeStatus call —
// but it also kicks off that fresh call in the background, shows a "checking"
// note while it's in flight, and updates the warning for real once it
// resolves. y is ignored until then, since acting on stale/missing info would
// defeat the point of checking at all.
func TestConfirmDeleteOpensInstantlyThenUpdatesFromLiveCheck(t *testing.T) {
	be := &fakeBackend{
		sessions:       []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}},
		worktreeStatus: map[string]gitStatusInfo{"demo:a": {dirty: true, unpushed: true, ok: true}},
	}
	m := newTestModel(be)
	// Stale/absent cache on open: the dialog must not show a warning yet
	// (nothing cached), only the "checking" note, and must not have called
	// the backend synchronously.
	_, cmd := m.Update(keyRune("d"))
	if len(be.worktreeStatusCalls) != 0 {
		t.Fatalf("opening the delete dialog must not call WorktreeStatus synchronously (it would pause the dialog), got calls = %v", be.worktreeStatusCalls)
	}
	if v := m.View(); strings.Contains(v, "uncommitted changes") || !strings.Contains(v, "checking") {
		t.Fatalf("confirm view before the check resolves:\n%s", v)
	}
	// y must not act on the not-yet-resolved check.
	m.Update(keyRune("y"))
	if len(be.deleteCalls) != 0 || m.mode != ModeConfirmDelete {
		t.Fatalf("y while checking: deleteCalls=%v mode=%v, want no-op", be.deleteCalls, m.mode)
	}

	// The background check resolves.
	drainCmd(m, cmd)
	if len(be.worktreeStatusCalls) != 1 || be.worktreeStatusCalls[0] != "demo:a" {
		t.Fatalf("expected one WorktreeStatus call for demo:a, got %v", be.worktreeStatusCalls)
	}
	if v := m.View(); !strings.Contains(v, "uncommitted changes") || !strings.Contains(v, "unpushed commits") || strings.Contains(v, "checking") {
		t.Fatalf("confirm view after the check resolves:\n%s", v)
	}

	// First y only acknowledges the now-resolved warning — no delete yet.
	m.Update(keyRune("y"))
	if len(be.deleteCalls) != 0 || m.mode != ModeConfirmDelete {
		t.Fatalf("first y: deleteCalls=%v mode=%v, want no delete yet", be.deleteCalls, m.mode)
	}

	// Second y actually deletes.
	run(m, keyRune("y"))
	if len(be.deleteCalls) != 1 || be.deleteCalls[0] != "demo:a" {
		t.Fatalf("deleteCalls = %v", be.deleteCalls)
	}
}

// drainAll recursively runs cmd, feeding each resulting message back into
// Update — including unwrapping tea.BatchMsg, which plain drainCmd/run leave
// unexecuted (tea.Batch's Cmd yields a BatchMsg of further Cmds, not a final
// message). Only safe when the model's statusCh is already closed: otherwise
// the listenStatus cmd bundled into StatusTickMsg's batch blocks forever on
// an empty, open channel.
func drainAll(m *Model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			drainAll(m, c)
		}
		return
	}
	_, next := m.Update(msg)
	drainAll(m, next)
}

// TestGitStatusTrackedRegardlessOfState guards the core design point: git
// status is no longer gated on being parked. A session that's actively
// Working with no cached status yet still gets fetched on the next tick,
// same as a parked one would — staleGitStatusIDs treats "never checked" as
// maximally stale for every session, not just parked ones.
func TestGitStatusTrackedRegardlessOfState(t *testing.T) {
	be := &fakeBackend{
		sessions:       []session.Session{{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"}},
		worktreeStatus: map[string]gitStatusInfo{"demo:a": {dirty: true, ok: true}},
	}
	m := newTestModel(be)
	m.tmuxAlive["demo:a"] = true
	m.states["/wt/a"] = watcher.Working

	drainCmd(m, m.fetchStaleGitStatusCmd())

	if len(be.worktreeStatusCalls) != 1 || be.worktreeStatusCalls[0] != "demo:a" {
		t.Fatalf("expected one WorktreeStatus call for a Working session with no cached status, got %v", be.worktreeStatusCalls)
	}
	if got := m.gitStatus["demo:a"]; !got.dirty {
		t.Fatalf("gitStatus[demo:a] = %+v, want dirty=true", got)
	}
}

// TestStaleGitStatusIsRefetched guards the staleness half of
// staleGitStatusIDs: a cached status older than its jittered threshold is
// worth a fresh fetch even though it was already checked once, since the
// worktree can change while the session sits untouched (edits from another
// terminal, a push from elsewhere).
func TestStaleGitStatusIsRefetched(t *testing.T) {
	be := &fakeBackend{
		sessions:       []session.Session{{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"}},
		worktreeStatus: map[string]gitStatusInfo{"demo:a": {dirty: true, ok: true}},
	}
	m := newTestModel(be)
	// Well past even the top of the jitter range (gitStatusStaleAfter * (1 +
	// gitStatusStaleJitter)), so this is unambiguously stale regardless of
	// demo:a's particular jitter.
	m.gitStatus["demo:a"] = gitStatusInfo{ok: true, checkedAt: time.Now().Add(-2 * gitStatusStaleAfter)}

	drainCmd(m, m.fetchStaleGitStatusCmd())

	if len(be.worktreeStatusCalls) != 1 || be.worktreeStatusCalls[0] != "demo:a" {
		t.Fatalf("stale cached entry should trigger exactly one refetch, calls = %v", be.worktreeStatusCalls)
	}
	if got := m.gitStatus["demo:a"]; !got.dirty {
		t.Fatalf("gitStatus[demo:a] not refreshed from the stale cache: %+v", got)
	}
}

// TestFreshGitStatusIsNotRefetched is the other half: a cached entry well
// within the staleness threshold must not trigger another
// `git status`/`rev-list` call — that's the whole point of caching by
// staleness instead of re-checking every session on every tick.
func TestFreshGitStatusIsNotRefetched(t *testing.T) {
	be := &fakeBackend{
		sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"}},
	}
	m := newTestModel(be)
	m.gitStatus["demo:a"] = gitStatusInfo{ok: true, checkedAt: time.Now()}

	if cmd := m.fetchStaleGitStatusCmd(); cmd != nil {
		t.Fatal("a fresh cached entry should not produce a fetch cmd")
	}
	if len(be.worktreeStatusCalls) != 0 {
		t.Fatalf("no WorktreeStatus call expected, got %v", be.worktreeStatusCalls)
	}
}

// TestGitStatusPendingSkipsDuplicateFetch guards against piling up
// concurrent `git status` calls for the same session: once a fetch is in
// flight (gitStatusPending), staleGitStatusIDs must not select that id again
// even though its cache is still missing/stale — otherwise a session whose
// git command is just running long would get re-issued a fresh fetch on
// every single tick until the first one finally returns.
func TestGitStatusPendingSkipsDuplicateFetch(t *testing.T) {
	be := &fakeBackend{
		sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"}},
	}
	m := newTestModel(be)

	cmd := m.fetchStaleGitStatusCmd()
	if cmd == nil {
		t.Fatal("expected a fetch cmd for a never-checked session")
	}
	if !m.gitStatusPending["demo:a"] {
		t.Fatal("demo:a should be marked pending once its fetch is dispatched")
	}

	// A second tick, before the first fetch resolves, must not re-select it.
	if again := m.fetchStaleGitStatusCmd(); again != nil {
		t.Fatal("a session with a fetch already in flight should not be re-selected")
	}

	// Resolving the first fetch clears pending and allows a future refetch.
	drainCmd(m, cmd)
	if m.gitStatusPending["demo:a"] {
		t.Fatal("demo:a should no longer be pending once its fetch resolves")
	}
}

// TestGitStatusMsgKeepsFresherResult guards against two overlapping fetches
// for the same session (a routine refresh racing the delete dialog's
// on-demand check, say) resolving out of order: whichever has the later
// checkedAt wins, regardless of arrival order.
func TestGitStatusMsgKeepsFresherResult(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}}}
	m := newTestModel(be)

	newer := time.Now()
	older := newer.Add(-time.Hour)

	m.Update(GitStatusMsg{Status: map[string]gitStatusInfo{"demo:a": {dirty: true, ok: true, checkedAt: newer}}})
	if got := m.gitStatus["demo:a"]; !got.dirty || !got.checkedAt.Equal(newer) {
		t.Fatalf("gitStatus[demo:a] = %+v after the first (newer) result", got)
	}

	// A stale, late-arriving result with an older checkedAt must not
	// overwrite the fresher one already recorded.
	m.Update(GitStatusMsg{Status: map[string]gitStatusInfo{"demo:a": {dirty: false, ok: true, checkedAt: older}}})
	if got := m.gitStatus["demo:a"]; !got.dirty || !got.checkedAt.Equal(newer) {
		t.Fatalf("gitStatus[demo:a] = %+v, want the newer result to survive", got)
	}
}

// TestGitStatusStaleThresholdIsJittered guards against a thundering herd:
// sessions all first fetched around the same moment (notably, every session
// at startup) must not all come due for refresh in the same tick forever
// after. gitStatusStaleThreshold must vary by session id (not return the
// same duration for every id) and must be stable across repeated calls for
// the same id (so staleness doesn't flap from one check to the next).
func TestGitStatusStaleThresholdIsJittered(t *testing.T) {
	a := gitStatusStaleThreshold("demo:a")
	b := gitStatusStaleThreshold("demo:b")
	if a == b {
		t.Fatalf("expected different sessions to get different jittered thresholds, both = %v", a)
	}
	for _, got := range []time.Duration{a, b} {
		lo := time.Duration(float64(gitStatusStaleAfter) * (1 - gitStatusStaleJitter))
		hi := time.Duration(float64(gitStatusStaleAfter) * (1 + gitStatusStaleJitter))
		if got < lo || got > hi {
			t.Fatalf("threshold %v outside jitter range [%v, %v]", got, lo, hi)
		}
	}
	if again := gitStatusStaleThreshold("demo:a"); again != a {
		t.Fatalf("threshold for the same id changed across calls: %v then %v", a, again)
	}
}

// TestInitFetchesGitStatusForEverySession guards the startup path: since
// every session's git status is unfetched (checkedAt is zero) the moment the
// app starts, the first tmux-alive resolution must kick off a fetch for all
// of them, not just whichever happen to be parked — regardless of agent
// state, without waiting for the first watcher tick.
func TestInitFetchesGitStatusForEverySession(t *testing.T) {
	be := &fakeBackend{
		sessions: []session.Session{
			{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"},
			{ID: "demo:b", Project: "demo", Name: "b", WorktreePath: "/wt/b"},
		},
		worktreeStatus: map[string]gitStatusInfo{"demo:a": {dirty: true, ok: true}},
	}
	m := newTestModel(be)
	// "b" is alive and Working — still expected to be checked, since git
	// status tracking is no longer gated on parked state.
	m.tmuxAlive["demo:b"] = true
	m.states["/wt/b"] = watcher.Working

	drainCmd(m, m.fetchStaleGitStatusCmd())

	gotIDs := append([]string(nil), be.worktreeStatusCalls...)
	sort.Strings(gotIDs)
	if !reflect.DeepEqual(gotIDs, []string{"demo:a", "demo:b"}) {
		t.Fatalf("expected a WorktreeStatus call for every session, got %v", gotIDs)
	}
	got, ok := m.gitStatus["demo:a"]
	if !ok || !got.dirty {
		t.Fatalf("gitStatus[demo:a] = %+v, ok=%v; want dirty=true", got, ok)
	}
	if _, ok := m.gitStatus["demo:b"]; !ok {
		t.Fatal("gitStatus[demo:b] should have been fetched too — it's not gated on parked state")
	}
}

func TestConfirmDeleteErrorFlashes(t *testing.T) {
	be := &fakeBackend{
		sessions:  []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}},
		deleteErr: errors.New("worktree busy"),
	}
	m := newTestModel(be)
	run(m, keyRune("d"))
	run(m, keyRune("y"))
	if m.flashKind != "error" || !strings.Contains(m.flash, "worktree busy") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
}

func TestTagFormFlow(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:a", Project: "demo", Name: "a", Ticket: "old-ticket", PR: "old-pr"},
	}}
	m := newTestModel(be)
	m.prompts["demo:a"] = "old agent prompt"

	m.Update(keyRune("t"))
	if m.mode != ModeTagForm {
		t.Fatalf("mode = %v", m.mode)
	}
	if m.tagForm.inputs[0].Value() != "old-ticket" || m.tagForm.inputs[1].Value() != "old-pr" {
		t.Fatalf("tag form not prefilled: %q %q", m.tagForm.inputs[0].Value(), m.tagForm.inputs[1].Value())
	}
	if v := m.View(); !strings.Contains(strings.ToLower(v), "ticket") {
		t.Fatalf("tag view:\n%s", v)
	}

	m.tagForm.inputs[0].SetValue("https://t/9")
	press(m, tea.KeyTab) // -> PR field
	if m.tagForm.focus != 1 {
		t.Fatalf("focus = %d", m.tagForm.focus)
	}
	m.tagForm.inputs[1].SetValue("https://pr/9")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.tagCalls) != 1 || be.tagCalls[0] != (tagCall{"demo:a", "https://t/9", "https://pr/9"}) {
		t.Fatalf("tagCalls = %v", be.tagCalls)
	}
	if m.mode != ModeList || !strings.Contains(m.flash, "tagged") {
		t.Fatalf("mode=%v flash=%q", m.mode, m.flash)
	}

	// esc cancels
	m.Update(keyRune("t"))
	press(m, tea.KeyEsc)
	if m.mode != ModeList {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestNewProjectFlow(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(slashKey())
	m.Update(keyRune("n"))
	if m.mode != ModeNewProject {
		t.Fatalf("mode = %v", m.mode)
	}
	if v := m.View(); !strings.Contains(strings.ToLower(v), "repo") {
		t.Fatalf("project form view:\n%s", v)
	}

	m.projForm.inputs[0].SetValue("newproj")
	m.projForm.inputs[1].SetValue("/tmp/newproj")
	m.projForm.inputs[2].SetValue("") // base defaults to main
	m.projForm.inputs[3].SetValue("me")

	// walk focus to the agent selector and worktree toggle
	m.projForm.focus = projFormInputCount + 1
	press(m, tea.KeyRight) // agent claude -> codex
	if m.projForm.agentIdx != 1 {
		t.Fatalf("agentIdx = %d", m.projForm.agentIdx)
	}
	m.projForm.focus = projFormInputCount + 2
	press(m, tea.KeyLeft) // toggle no-worktree on
	if !m.projForm.noWorktree {
		t.Fatal("noWorktree not toggled")
	}
	if v := m.View(); v == "" {
		t.Fatal("empty view with selector focused")
	}

	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.addProjectCalls) != 1 {
		t.Fatalf("addProjectCalls = %v", be.addProjectCalls)
	}
	got := be.addProjectCalls[0]
	want := config.Project{Repo: "/tmp/newproj", BaseBranch: "main", BranchPrefix: "me", Agent: "codex", NoWorktree: true}
	if got.name != "newproj" || got.p != want {
		t.Fatalf("call = %+v", got)
	}
	if m.mode != ModeProjectPicker || !strings.Contains(m.flash, "added project newproj") {
		t.Fatalf("mode=%v flash=%q", m.mode, m.flash)
	}
}

// TestNewProjectEmojiSelectorDefaultsToAutoAndCycles guards the emoji
// picker's index<->value mapping: index 0 ("auto") must store an empty
// Emoji so callers fall back to the deterministic pick, and cycling right
// must select the next palette glyph.
func TestNewProjectEmojiSelectorDefaultsToAutoAndCycles(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	m.Update(slashKey())
	m.Update(keyRune("n"))
	m.projForm.inputs[0].SetValue("newproj")
	m.projForm.inputs[1].SetValue("/tmp/newproj")

	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := be.addProjectCalls[0].p.Emoji; got != "" {
		t.Fatalf("default emoji = %q, want empty (auto)", got)
	}

	be.addProjectCalls = nil
	m.mode = ModeList
	m.Update(slashKey())
	m.Update(keyRune("n"))
	m.projForm.inputs[0].SetValue("newproj2")
	m.projForm.inputs[1].SetValue("/tmp/newproj2")
	m.projForm.focus = projFormInputCount
	press(m, tea.KeyRight) // cycle off "auto" to the first palette glyph
	if m.projForm.emojiIdx != 1 {
		t.Fatalf("emojiIdx = %d, want 1", m.projForm.emojiIdx)
	}
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	want := projectEmojiChoices[1]
	if got := be.addProjectCalls[0].p.Emoji; got != want {
		t.Fatalf("emoji = %q, want %q", got, want)
	}
}

func TestEditSessionFlow(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:a", Project: "demo", Name: "a", Agent: "codex"},
	}}
	m := newTestModel(be)

	m.Update(keyRune("e"))
	if m.mode != ModeEditSession {
		t.Fatalf("mode = %v", m.mode)
	}
	if got := agentChoices[m.sessionForm.agentIdx]; got != "codex" {
		t.Fatalf("prefilled agent = %q", got)
	}
	if view := m.View(); !strings.Contains(view, "Edit session") ||
		!strings.Contains(view, "demo") || !strings.Contains(view, "a") {
		t.Fatalf("edit session view:\n%s", view)
	}

	press(m, tea.KeyRight)
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.sessionAgentCalls) != 1 ||
		be.sessionAgentCalls[0] != (sessionAgentCall{"demo:a", "opencode"}) {
		t.Fatalf("sessionAgentCalls = %v", be.sessionAgentCalls)
	}
	if m.mode != ModeList || !strings.Contains(m.flash, "updated session a") {
		t.Fatalf("mode=%v flash=%q", m.mode, m.flash)
	}
	if _, ok := m.prompts["demo:a"]; ok {
		t.Fatal("agent edit must invalidate the cached prompt")
	}
}

func TestEditSessionCancelAndError(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:a", Project: "demo", Name: "a", Agent: "claude"},
	}}
	m := newTestModel(be)
	m.Update(keyRune("e"))
	press(m, tea.KeyEsc)
	if m.mode != ModeList || len(be.sessionAgentCalls) != 0 {
		t.Fatalf("mode=%v calls=%v", m.mode, be.sessionAgentCalls)
	}

	be.sessionAgentErr = errors.New("disk full")
	m.Update(keyRune("e"))
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ModeEditSession || !strings.Contains(m.sessionForm.err, "disk full") {
		t.Fatalf("mode=%v err=%q", m.mode, m.sessionForm.err)
	}
}

func TestEditProjectFlow(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{
		"demo": {
			Kind: "git", Repo: "/tmp/demo", BaseBranch: "main",
			BranchPrefix: "old", Agent: "codex",
		},
	}}
	be := &fakeBackend{}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 100, 32

	m.Update(slashKey())
	m.Update(keyRune("e"))
	if m.mode != ModeEditProject {
		t.Fatalf("mode = %v", m.mode)
	}
	if m.projForm.inputs[1].Value() != "/tmp/demo" ||
		m.projForm.inputs[2].Value() != "main" ||
		m.projForm.inputs[3].Value() != "old" ||
		agentChoices[m.projForm.agentIdx] != "codex" {
		t.Fatalf("project form = %+v", m.projForm)
	}
	if view := m.View(); !strings.Contains(view, "Edit project") ||
		!strings.Contains(view, "demo") {
		t.Fatalf("edit project view:\n%s", view)
	}

	m.projForm.inputs[1].SetValue("/tmp/renamed")
	m.projForm.inputs[2].SetValue("trunk")
	m.projForm.inputs[3].SetValue("alan")
	m.projForm.agentIdx = 2
	m.projForm.noWorktree = true
	run(m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(be.updateProjectCalls) != 1 {
		t.Fatalf("updateProjectCalls = %v", be.updateProjectCalls)
	}
	got := be.updateProjectCalls[0]
	if got.name != "demo" || got.p.Kind != "git" || got.p.Repo != "/tmp/renamed" ||
		got.p.BaseBranch != "trunk" || got.p.BranchPrefix != "alan" ||
		got.p.Agent != "opencode" || !got.p.NoWorktree {
		t.Fatalf("call = %+v", got)
	}
	if m.mode != ModeProjectPicker || !strings.Contains(m.flash, "updated project demo") {
		t.Fatalf("mode=%v flash=%q", m.mode, m.flash)
	}
}

func TestEditProjectCancelAndError(t *testing.T) {
	be := &fakeBackend{updateProjectErr: errors.New("not a git repo")}
	m := newTestModel(be)
	m.Update(slashKey())
	m.Update(keyRune("e"))
	press(m, tea.KeyEsc)
	if m.mode != ModeProjectPicker || len(be.updateProjectCalls) != 0 {
		t.Fatalf("mode=%v calls=%v", m.mode, be.updateProjectCalls)
	}

	m.Update(keyRune("e")) // still in the picker after the cancel above
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ModeEditProject || !strings.Contains(m.projForm.err, "not a git repo") {
		t.Fatalf("mode=%v err=%q", m.mode, m.projForm.err)
	}
	if view := m.View(); !strings.Contains(view, "not a git repo") {
		t.Fatalf("keyboard-sized project editor hides save error:\n%s", view)
	}
}

func TestEditPlainProjectShowsOnlyRepoAndAgent(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{
		"notes": {Kind: "plain", Repo: "/tmp/notes", Agent: "claude"},
	}}
	be := &fakeBackend{}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24

	m.Update(slashKey())
	m.Update(keyRune("e"))
	view := m.View()
	for _, hidden := range []string{"base branch:", "branch prefix:", "worktrees:"} {
		if strings.Contains(view, hidden) {
			t.Fatalf("plain project editor contains %q:\n%s", hidden, view)
		}
	}
	press(m, tea.KeyTab)
	if m.projForm.focus != 4 {
		t.Fatalf("focus = %d, want emoji field", m.projForm.focus)
	}
	press(m, tea.KeyTab)
	if m.projForm.focus != projFormInputCount+1 {
		t.Fatalf("focus = %d, want agent selector", m.projForm.focus)
	}
	m.projForm.agentIdx = agentChoiceIndex("codex")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.updateProjectCalls) != 1 {
		t.Fatalf("updateProjectCalls = %v", be.updateProjectCalls)
	}
	got := be.updateProjectCalls[0].p
	if got.Kind != "plain" || got.Repo != "/tmp/notes" || got.Agent != "codex" {
		t.Fatalf("project = %+v", got)
	}
}

// TestEditProjectPreservesOutOfPaletteEmoji guards against a project whose
// Emoji isn't one of projectEmojiPalette's glyphs (e.g. hand-edited into the
// TOML config) getting silently reset to "" (auto) just by saving the edit
// form after touching an unrelated field.
func TestEditProjectPreservesOutOfPaletteEmoji(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{
		"notes": {Kind: "git", Repo: "/tmp/notes", BaseBranch: "main", Emoji: "😀"},
	}}
	be := &fakeBackend{}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24

	m.Update(slashKey())
	m.Update(keyRune("e"))
	m.projForm.inputs[1].SetValue("/tmp/notes2") // touch an unrelated field
	run(m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(be.updateProjectCalls) != 1 {
		t.Fatalf("updateProjectCalls = %v", be.updateProjectCalls)
	}
	if got := be.updateProjectCalls[0].p.Emoji; got != "😀" {
		t.Fatalf("emoji = %q, want unchanged 😀", got)
	}
}

func TestNewProjectTabCyclesFocus(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	m.Update(slashKey())
	m.Update(keyRune("n"))
	total := projFormInputCount + 3
	for i := 1; i < total; i++ {
		press(m, tea.KeyTab)
		if m.projForm.focus != i {
			t.Fatalf("after %d tabs focus = %d", i, m.projForm.focus)
		}
	}
	press(m, tea.KeyTab) // wraps
	if m.projForm.focus != 0 {
		t.Fatalf("focus = %d", m.projForm.focus)
	}
	press(m, tea.KeyShiftTab)
	if m.projForm.focus != total-1 {
		t.Fatalf("focus = %d", m.projForm.focus)
	}
	press(m, tea.KeyEsc)
	if m.mode != ModeProjectPicker {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestNewProjectNotGitRepoOffersInit(t *testing.T) {
	be := &fakeBackend{addProjectErr: gitwt.ErrNotGitRepo}
	m := newTestModel(be)
	m.Update(slashKey())
	m.Update(keyRune("n"))
	m.projForm.inputs[0].SetValue("newproj")
	m.projForm.inputs[1].SetValue("/tmp/newproj")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ModeProjectInitChoice {
		t.Fatalf("mode = %v", m.mode)
	}
	if v := m.View(); !strings.Contains(v, "not a git repo") && !strings.Contains(strings.ToLower(v), "init") {
		t.Fatalf("init choice view:\n%s", v)
	}

	// 'i' initializes a git repo there
	run(m, keyRune("i"))
	if len(be.initProjectCalls) != 1 || be.initProjectCalls[0].name != "newproj" {
		t.Fatalf("initProjectCalls = %v", be.initProjectCalls)
	}
	if m.mode != ModeProjectPicker || !strings.Contains(m.flash, "initialized git repo") {
		t.Fatalf("mode=%v flash=%q", m.mode, m.flash)
	}
}

func TestProjectInitChoicePlainAndBack(t *testing.T) {
	be := &fakeBackend{addProjectErr: gitwt.ErrNotGitRepo}
	m := newTestModel(be)
	m.Update(slashKey())
	m.Update(keyRune("n"))
	m.projForm.inputs[0].SetValue("plainy")
	m.projForm.inputs[1].SetValue("/tmp/plainy")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})

	// 'b' goes back to the form
	m.Update(keyRune("b"))
	if m.mode != ModeNewProject {
		t.Fatalf("mode = %v", m.mode)
	}

	run(m, tea.KeyMsg{Type: tea.KeyEnter}) // re-trigger init choice
	if m.mode != ModeProjectInitChoice {
		t.Fatalf("mode = %v", m.mode)
	}
	run(m, keyRune("s")) // keep as plain folder
	if len(be.plainCalls) != 1 || be.plainCalls[0].name != "plainy" {
		t.Fatalf("plainCalls = %v", be.plainCalls)
	}
	if m.mode != ModeProjectPicker || !strings.Contains(m.flash, "plain") {
		t.Fatalf("mode=%v flash=%q", m.mode, m.flash)
	}
}

func TestProjectInitChoiceInitErrorReturnsToForm(t *testing.T) {
	be := &fakeBackend{addProjectErr: gitwt.ErrNotGitRepo, initProjectErr: errors.New("mkdir denied")}
	m := newTestModel(be)
	m.Update(slashKey())
	m.Update(keyRune("n"))
	m.projForm.inputs[0].SetValue("p")
	m.projForm.inputs[1].SetValue("/tmp/p")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	run(m, keyRune("i"))
	if m.mode != ModeNewProject || !strings.Contains(m.projForm.err, "mkdir denied") {
		t.Fatalf("mode=%v err=%q", m.mode, m.projForm.err)
	}
}

func TestNewProjectPlainAddError(t *testing.T) {
	be := &fakeBackend{addProjectErr: errors.New("name taken")}
	m := newTestModel(be)
	m.Update(slashKey())
	m.Update(keyRune("n"))
	m.projForm.inputs[0].SetValue("dup")
	m.projForm.inputs[1].SetValue("/tmp/dup")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != ModeNewProject || !strings.Contains(m.projForm.err, "name taken") {
		t.Fatalf("mode=%v err=%q", m.mode, m.projForm.err)
	}
	if v := m.View(); !strings.Contains(v, "name taken") {
		t.Fatalf("form error not rendered:\n%s", v)
	}
}

func TestConfirmDeleteProjectFlow(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("D"))
	if m.mode != ModeConfirmDeleteProject {
		t.Fatalf("mode = %v", m.mode)
	}
	if v := m.View(); !strings.Contains(strings.ToLower(v), "remove") && !strings.Contains(strings.ToLower(v), "delete") {
		t.Fatalf("confirm project view:\n%s", v)
	}

	m.Update(keyRune("n"))
	if m.mode != ModeList || len(be.removeProjectCalls) != 0 {
		t.Fatalf("mode=%v calls=%v", m.mode, be.removeProjectCalls)
	}

	m.Update(keyRune("D"))
	run(m, keyRune("y"))
	if len(be.removeProjectCalls) != 1 || be.removeProjectCalls[0] != "demo" {
		t.Fatalf("removeProjectCalls = %v", be.removeProjectCalls)
	}
	if !strings.Contains(m.flash, "removed project demo") {
		t.Fatalf("flash = %q", m.flash)
	}
}

func TestConfirmDeleteProjectErrorFlashes(t *testing.T) {
	be := &fakeBackend{removeProjectErr: errors.New("has active sessions")}
	m := newTestModel(be)
	m.Update(keyRune("D"))
	run(m, keyRune("y"))
	if m.mode != ModeList || m.flashKind != "error" || !strings.Contains(m.flash, "active sessions") {
		t.Fatalf("mode=%v flash=%q (%s)", m.mode, m.flash, m.flashKind)
	}
}

func TestDeleteProjectWithSessionsIsBlocked(t *testing.T) {
	for _, archived := range []bool{false, true} {
		be := &fakeBackend{sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a", Archived: archived}}}
		m := newTestModel(be)
		m.Update(keyRune("D"))
		if m.mode != ModeList || m.flashKind != "error" || !strings.Contains(m.flash, "delete them first") {
			t.Fatalf("archived=%v mode=%v flash=%q (%s)", archived, m.mode, m.flash, m.flashKind)
		}
		if len(be.removeProjectCalls) != 0 {
			t.Fatalf("archived=%v removeProjectCalls = %v", archived, be.removeProjectCalls)
		}
	}
}

func TestDeleteProjectWithNoProjectsFlashesError(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{}}
	be := &fakeBackend{}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24
	m.mode = ModeList
	m.Update(keyRune("D"))
	if m.flashKind != "error" || !strings.Contains(m.flash, "no projects") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
}

func TestOpenSessionFlow(t *testing.T) {
	be := &fakeBackend{
		sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}},
		openHint: "run: tmux attach -t moomux-a",
	}
	m := newTestModel(be)
	run(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(be.openCalls) != 1 || be.openCalls[0] != "demo:a" {
		t.Fatalf("openCalls = %v", be.openCalls)
	}
	if !strings.Contains(m.flash, "opened demo:a") || !strings.Contains(m.flash, "tmux attach") {
		t.Fatalf("flash = %q", m.flash)
	}

	be.openErr = errors.New("no terminal")
	run(m, keyRune("o"))
	if m.flashKind != "error" || !strings.Contains(m.flash, "no terminal") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
}

func TestKillTmuxFlow(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}}}
	m := newTestModel(be)
	run(m, keyRune("x"))
	if len(be.killCalls) != 1 || be.killCalls[0] != "demo:a" {
		t.Fatalf("killCalls = %v", be.killCalls)
	}
	if !strings.Contains(m.flash, "parked") {
		t.Fatalf("flash = %q", m.flash)
	}
}

func TestArchiveFlow(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:a", Project: "demo", Name: "a"},
		{ID: "demo:b", Project: "demo", Name: "b", Archived: true},
	}}
	m := newTestModel(be)

	run(m, keyRune("a"))
	if len(be.archiveCalls) != 1 || be.archiveCalls[0] != (archiveCall{"demo:a", true}) {
		t.Fatalf("archiveCalls = %v", be.archiveCalls)
	}
	if !strings.Contains(m.flash, "archived") {
		t.Fatalf("flash = %q", m.flash)
	}

	// 'A' switches to the archived view; 'a' there restores
	m.Update(keyRune("A"))
	if !m.showArchived || len(m.sessions) != 2 {
		t.Fatalf("showArchived=%v sessions=%v", m.showArchived, m.sessions)
	}
	run(m, keyRune("a"))
	if got := be.archiveCalls[len(be.archiveCalls)-1]; got.archived {
		t.Fatalf("expected restore, got %+v", got)
	}
	if !strings.Contains(m.flash, "restored") {
		t.Fatalf("flash = %q", m.flash)
	}
}

func TestArchiveErrorFlashes(t *testing.T) {
	be := &fakeBackend{
		sessions:   []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}},
		archiveErr: errors.New("disk full"),
	}
	m := newTestModel(be)
	run(m, keyRune("a"))
	if m.flashKind != "error" || !strings.Contains(m.flash, "disk full") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
}

func TestNavigationAndProjectSwitching(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{
		"alpha": {Repo: "/tmp/alpha"},
		"beta":  {Repo: "/tmp/beta"},
	}}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "alpha:a", Project: "alpha", Name: "a"},
		{ID: "alpha:b", Project: "alpha", Name: "b"},
		{ID: "beta:c", Project: "beta", Name: "c"},
	}}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24

	m.Update(keyRune("j"))
	if m.cursor != 1 {
		t.Fatalf("cursor = %d", m.cursor)
	}
	m.Update(keyRune("j")) // wraps
	if m.cursor != 0 {
		t.Fatalf("cursor = %d", m.cursor)
	}
	m.Update(keyRune("k")) // wraps back
	if m.cursor != 1 {
		t.Fatalf("cursor = %d", m.cursor)
	}

	press(m, tea.KeyTab)
	if m.projects[m.activeProj] != "beta" || len(m.sessions) != 1 {
		t.Fatalf("proj=%q sessions=%v", m.projects[m.activeProj], m.sessions)
	}
	press(m, tea.KeyShiftTab)
	if m.projects[m.activeProj] != "alpha" {
		t.Fatalf("proj=%q", m.projects[m.activeProj])
	}
}

// Cycling skips over projects with no sessions at all, so a repo with many
// freshly-added-but-unused projects doesn't require tabbing through each one.
func TestProjectCyclingSkipsEmptyProjects(t *testing.T) {
	cfg := &config.Config{
		Projects: map[string]config.Project{
			"alpha": {Repo: "/tmp/alpha"},
			"empty": {Repo: "/tmp/empty"},
			"beta":  {Repo: "/tmp/beta"},
		},
		Order: []string{"alpha", "empty", "beta"},
	}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "alpha:a", Project: "alpha", Name: "a"},
		{ID: "beta:c", Project: "beta", Name: "c"},
	}}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24

	if m.projects[m.activeProj] != "alpha" {
		t.Fatalf("start proj=%q", m.projects[m.activeProj])
	}
	press(m, tea.KeyTab)
	if m.projects[m.activeProj] != "beta" {
		t.Fatalf("after tab: proj=%q, want beta (empty skipped)", m.projects[m.activeProj])
	}
	press(m, tea.KeyShiftTab)
	if m.projects[m.activeProj] != "alpha" {
		t.Fatalf("after shift+tab: proj=%q, want alpha (empty skipped)", m.projects[m.activeProj])
	}

	// "}" / "{" step one at a time and do land on the empty project — the
	// only way to reach it now that Tab/]/[ skip past it.
	m.Update(keyRune("}"))
	if m.projects[m.activeProj] != "empty" || len(m.sessions) != 0 {
		t.Fatalf("after }}: proj=%q sessions=%v, want empty with no sessions", m.projects[m.activeProj], m.sessions)
	}
	m.Update(keyRune("{"))
	if m.projects[m.activeProj] != "alpha" || len(m.sessions) != 1 || m.sessions[0].ID != "alpha:a" {
		t.Fatalf("after {{: proj=%q sessions=%v, want alpha with its session", m.projects[m.activeProj], m.sessions)
	}
}

// When the active project is the only one with sessions, Tab/shift+tab must
// stay put rather than hopping to an empty neighbor — there's nothing to
// cycle to.
func TestProjectCyclingStaysWhenOnlyActiveHasSessions(t *testing.T) {
	cfg := &config.Config{
		Projects: map[string]config.Project{
			"alpha": {Repo: "/tmp/alpha"},
			"empty": {Repo: "/tmp/empty"},
		},
		Order: []string{"alpha", "empty"},
	}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "alpha:a", Project: "alpha", Name: "a"},
	}}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24

	press(m, tea.KeyTab)
	if m.projects[m.activeProj] != "alpha" {
		t.Fatalf("after tab: proj=%q, want alpha (only project with sessions)", m.projects[m.activeProj])
	}
	press(m, tea.KeyShiftTab)
	if m.projects[m.activeProj] != "alpha" {
		t.Fatalf("after shift+tab: proj=%q, want alpha (only project with sessions)", m.projects[m.activeProj])
	}
}

// Mobile/remote terminals often can't send shift+arrow or shift+tab as a single
// keypress, so every chorded action has a plain-letter alternate.
func TestPlainLetterAlternatesForChordedKeys(t *testing.T) {
	cfg := &config.Config{Projects: map[string]config.Project{
		"alpha": {Repo: "/tmp/alpha"},
		"beta":  {Repo: "/tmp/beta"},
	}}
	be := &fakeBackend{sessions: []session.Session{
		{ID: "alpha:a", Project: "alpha", Name: "a"},
		{ID: "alpha:b", Project: "alpha", Name: "b"},
		{ID: "beta:c", Project: "beta", Name: "c"},
	}}
	m := New(cfg, be, make(chan watcher.Snapshot), func() {})
	m.width, m.height = 80, 24

	// "]" / "[" switch project like tab / shift+tab.
	m.Update(keyRune("]"))
	if m.projects[m.activeProj] != "beta" || len(m.sessions) != 1 {
		t.Fatalf("after ]: proj=%q sessions=%v", m.projects[m.activeProj], m.sessions)
	}
	m.Update(keyRune("["))
	if m.projects[m.activeProj] != "alpha" {
		t.Fatalf("after [: proj=%q", m.projects[m.activeProj])
	}

	// "J" / "K" reorder the selected session like shift+↓ / shift+↑.
	run(m, keyRune("J"))
	m.cursor = 1
	run(m, keyRune("K"))
	want := []moveSessionCall{{id: "alpha:a", delta: 1}, {id: "alpha:b", delta: -1}}
	if len(be.moveSessionCalls) != 2 || be.moveSessionCalls[0] != want[0] || be.moveSessionCalls[1] != want[1] {
		t.Fatalf("moveSessionCalls = %+v, want %+v", be.moveSessionCalls, want)
	}

	// "L" / "H" reorder the active project like shift+→ / shift+←.
	run(m, keyRune("L"))
	m.activeProj = 1
	run(m, keyRune("H"))
	wantProj := []moveProjectCall{{name: "alpha", delta: 1}, {name: "beta", delta: -1}}
	if len(be.moveProjectCalls) != 2 || be.moveProjectCalls[0] != wantProj[0] || be.moveProjectCalls[1] != wantProj[1] {
		t.Fatalf("moveProjectCalls = %+v, want %+v", be.moveProjectCalls, wantProj)
	}
}

// The form fields navigate with tab/shift+tab, so "[" and "]" must stay ordinary
// text input there rather than switching project underneath the form.
func TestBracketsAreTextInputInForms(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)

	m.Update(keyRune("n"))
	typeText(m, "feat[1]")
	run(m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(be.createCalls) != 1 || be.createCalls[0].name != "feat[1]" {
		t.Fatalf("createCalls = %+v", be.createCalls)
	}
}

func TestRefreshRunsStatusCmd(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a"}}}
	m := newTestModel(be)
	_, cmd := m.Update(keyRune("r"))
	if cmd == nil {
		t.Fatal("refresh must return a status refresh command")
	}
	msg := cmd()
	refreshed, ok := msg.(StatusRefreshedMsg)
	if !ok {
		t.Fatalf("msg = %T", msg)
	}
	m.Update(refreshed)
}

func TestHelpMode(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	m.Update(keyRune("?"))
	if m.mode != ModeHelp {
		t.Fatalf("mode = %v", m.mode)
	}
	if v := m.View(); !strings.Contains(strings.ToLower(v), "help") && !strings.Contains(v, "?") {
		t.Fatalf("help view:\n%s", v)
	}
	m.Update(keyRune("?"))
	if m.mode != ModeList {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestQuitReturnsQuitCmd(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	_, cmd := m.Update(keyRune("q"))
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected tea.QuitMsg")
	}
}

func TestCtrlCQuitsFromEveryOverlay(t *testing.T) {
	for _, mode := range []Mode{
		ModeNewForm, ModeConfirmDelete, ModeNewProject, ModeConfirmDeleteProject,
		ModeProjectInitChoice, ModeTagForm, ModeEditSession, ModeEditProject,
	} {
		m := newTestModel(&fakeBackend{})
		m.mode = mode
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Fatalf("mode %v: ctrl+c swallowed", mode)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("mode %v: expected tea.QuitMsg", mode)
		}
	}
}

func TestBusyOverlayStillCancels(t *testing.T) {
	for _, mode := range []Mode{ModeEditSession, ModeEditProject} {
		m := newTestModel(&fakeBackend{})
		m.mode = mode
		m.busy = true
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m.mode != ModeList {
			t.Fatalf("mode %v: esc did not close busy overlay", mode)
		}
	}
}

func TestWindowSizeMsg(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Fatalf("size = %dx%d", m.width, m.height)
	}
}

func TestWindowSizePreservesTextInputCursor(t *testing.T) {
	m := newTestModel(&fakeBackend{})
	m.mode = ModeNewForm
	m.nameInput.SetValue("abcdef")
	m.nameInput.SetCursor(2)

	m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})

	if got := m.nameInput.Position(); got != 2 {
		t.Fatalf("cursor position after resize = %d, want 2", got)
	}
}

func TestStatusMessages(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"}}}
	m := newTestModel(be)

	_, cmd := m.Update(StatusTickMsg{Snap: watcher.Snapshot{
		States: map[string]watcher.State{"/wt/a": watcher.Working},
		Err:    errors.New("scan hiccup"),
	}})
	if cmd == nil {
		t.Fatal("status tick must re-arm the listener")
	}
	if m.states["/wt/a"] != watcher.Working || !strings.Contains(m.flash, "scan hiccup") {
		t.Fatalf("states=%v flash=%q", m.states, m.flash)
	}
	if m.flashKind != "error" {
		t.Fatalf("flashKind = %q, want %q", m.flashKind, "error")
	}

	m.Update(StatusRefreshedMsg{
		TmuxAlive: map[string]bool{"demo:a": true},
		Prompts:   map[string]string{"demo:a": "do the thing"},
	})
	if !m.tmuxAlive["demo:a"] || m.prompts["demo:a"] != "do the thing" {
		t.Fatalf("alive=%v prompts=%v", m.tmuxAlive, m.prompts)
	}
	// existing prompt is not overwritten
	m.Update(StatusRefreshedMsg{Prompts: map[string]string{"demo:a": "other"}})
	if m.prompts["demo:a"] != "do the thing" {
		t.Fatalf("prompt overwritten: %q", m.prompts["demo:a"])
	}

	m.Update(StatusChannelClosedMsg{})
	if !strings.Contains(m.flash, "status watcher stopped") {
		t.Fatalf("flash = %q", m.flash)
	}
	if m.flashKind != "error" {
		t.Fatalf("flashKind = %q, want %q", m.flashKind, "error")
	}

	// the session list renders with a live status and prompt
	if v := m.View(); !strings.Contains(v, "a") {
		t.Fatalf("view:\n%s", v)
	}
}

func TestStatusTickPrunesRemovedSessionStates(t *testing.T) {
	be := &fakeBackend{sessions: []session.Session{
		{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"},
		{ID: "demo:removed", Project: "demo", Name: "removed", WorktreePath: "/wt/removed"},
	}}
	m := newTestModel(be)

	m.Update(StatusTickMsg{Snap: watcher.Snapshot{
		States: map[string]watcher.State{"/wt/a": watcher.Working, "/wt/removed": watcher.Working},
	}})
	if _, ok := m.states["/wt/removed"]; !ok {
		t.Fatal("setup: expected stale path present before session removal")
	}

	// Session for /wt/removed no longer exists in the backend (deleted).
	be.sessions = []session.Session{{ID: "demo:a", Project: "demo", Name: "a", WorktreePath: "/wt/a"}}
	m.Update(StatusTickMsg{Snap: watcher.Snapshot{
		States: map[string]watcher.State{"/wt/a": watcher.Done},
	}})

	if _, ok := m.states["/wt/removed"]; ok {
		t.Fatalf("states = %v, want /wt/removed pruned after its session was removed", m.states)
	}
	if m.states["/wt/a"] != watcher.Done {
		t.Fatalf("states[/wt/a] = %v, want still tracked", m.states["/wt/a"])
	}
}

func TestListenStatus(t *testing.T) {
	ch := make(chan watcher.Snapshot, 1)
	ch <- watcher.Snapshot{}
	if _, ok := listenStatus(ch)().(StatusTickMsg); !ok {
		t.Fatal("expected StatusTickMsg")
	}
	close(ch)
	if _, ok := listenStatus(ch)().(StatusChannelClosedMsg); !ok {
		t.Fatal("expected StatusChannelClosedMsg")
	}
}

// Creating a session in a non-active project should switch the active
// project to follow it, so the new session is immediately visible.
func TestSessionCreatedSwitchesActiveProject(t *testing.T) {
	be := &fakeBackend{}
	m := newMultiProjectTestModel(be)
	if m.projects[m.activeProj] != "alpha" {
		t.Fatalf("start proj=%q", m.projects[m.activeProj])
	}
	be.sessions = []session.Session{{ID: "beta:a", Project: "beta", Name: "a"}}
	m.Update(SessionCreatedMsg{Session: session.Session{ID: "beta:a", Project: "beta", Name: "a"}})
	if m.projects[m.activeProj] != "beta" {
		t.Fatalf("proj=%q, want beta", m.projects[m.activeProj])
	}
}

func TestSessionMovedErrorFlashes(t *testing.T) {
	be := &fakeBackend{}
	m := newTestModel(be)
	m.Update(SessionMovedMsg{ID: "demo:a", Err: errors.New("reorder failed")})
	if m.flashKind != "error" || !strings.Contains(m.flash, "reorder failed") {
		t.Fatalf("flash = %q (%s)", m.flash, m.flashKind)
	}
	m.Update(ProjectMovedMsg{Name: "demo", Err: errors.New("save failed")})
	if !strings.Contains(m.flash, "save failed") {
		t.Fatalf("flash = %q", m.flash)
	}
}
