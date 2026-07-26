# Project and Session Agent Editing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `e` and `E` edit flows so session agent settings and existing project settings can be changed without modifying a live tmux process.

**Architecture:** Add validated backend mutations for project and session configuration, leaving the existing session record as the source of truth when a parked tmux session is recreated. Extend the Bubble Tea model with focused edit modes that reuse current selectors and project form controls, and verify the terminal rendering with deterministic screenshot scenarios.

**Tech Stack:** Go, Bubble Tea, Bubbles textinput, Lip Gloss, BurntSushi TOML, Go `testing`, the existing `cmd/uishot` screenshot harness.

---

## File Map

- `internal/app/app.go`: validate agents, persist project edits with rollback, persist session-agent edits with rollback, and allocate an OpenCode port before recreating a parked tmux session.
- `internal/app/app_test.go`: backend and session lifecycle regression tests.
- `internal/tui/model.go`: backend interface additions, edit modes, and edit-form state initialization.
- `internal/tui/keys.go`: `e` and `E` bindings.
- `internal/tui/messages.go`: async edit result messages.
- `internal/tui/update.go`: open, submit, cancel, and result handling for both edit flows.
- `internal/tui/form.go`: session editor and edit-project rendering.
- `internal/tui/help.go`: shortcut documentation.
- `internal/tui/click_test.go`: fake backend mutation recording shared by TUI tests.
- `internal/tui/update_test.go`: edit-flow behavior tests.
- `internal/tui/help_test.go`: shortcut help assertions.
- `cmd/uishot/main.go`: deterministic `edit-session` and `edit-project` scenarios.
- `README.md`: include edit shortcuts in the concise key summary.

### Task 1: Backend Configuration Mutations

**Files:**
- Modify: `internal/app/app.go`
- Test: `internal/app/app_test.go`

- [ ] **Step 1: Write failing project-update tests**

Add tests that create a real temporary git repository and assert:

```go
func TestUpdateProject(t *testing.T) {
    repo := t.TempDir()
    mustGit(t, repo, "init", "-b", "main")
    a, _, _, _ := newTestApp(t, map[string]config.Project{
        "demo": {Kind: "git", Repo: repo, BaseBranch: "main", Agent: "claude"},
    })

    updated := config.Project{
        Repo: repo, BaseBranch: "trunk", BranchPrefix: "alan",
        Agent: "codex", NoWorktree: true,
    }
    if err := a.UpdateProject("demo", updated); err != nil {
        t.Fatal(err)
    }
    got := a.Cfg.Projects["demo"]
    if got.Kind != "git" || got.BaseBranch != "trunk" ||
        got.BranchPrefix != "alan" || got.Agent != "codex" || !got.NoWorktree {
        t.Fatalf("project = %+v", got)
    }
}
```

Also cover unknown project, invalid agent, non-repository git path, and a
plain-project update that preserves `Kind == "plain"` while clearing
`BaseBranch`, `BranchPrefix`, and `NoWorktree`.

- [ ] **Step 2: Write failing session-agent mutation tests**

Add tests for a successful persisted update, unknown session, invalid agent,
and rollback. The success case must assert no tmux call occurred:

```go
func TestSetSessionAgentDoesNotTouchTmux(t *testing.T) {
    a, _, tm, _ := newTestApp(t, gitProject("/repo"))
    original := session.Session{
        ID: "demo:a", Project: "demo", Name: "a",
        Agent: "claude", AgentPort: 4099,
    }
    if err := a.Store.Put(original); err != nil {
        t.Fatal(err)
    }

    got, err := a.SetSessionAgent(original.ID, "codex")
    if err != nil {
        t.Fatal(err)
    }
    if got.Agent != "codex" || got.AgentPort != 4099 {
        t.Fatalf("session = %+v", got)
    }
    if len(tm.calls) != 0 {
        t.Fatalf("tmux calls = %v", tm.calls)
    }
}
```

- [ ] **Step 3: Run the focused backend tests and verify RED**

Run:

```bash
rtk go test ./internal/app -run 'Test(UpdateProject|SetSessionAgent)' -count=1
```

Expected: compilation fails because `UpdateProject` and `SetSessionAgent` do
not exist.

- [ ] **Step 4: Implement validated backend mutations**

Add a shared validator:

```go
func validateAgent(agent string) error {
    switch agent {
    case "claude", "codex", "opencode":
        return nil
    default:
        return fmt.Errorf("unknown agent %q", agent)
    }
}
```

Implement:

```go
func (a *App) UpdateProject(name string, updated config.Project) error
func (a *App) SetSessionAgent(id, agent string) (session.Session, error)
```

`UpdateProject` must preserve the stored kind, expand and validate the repo,
normalize git/plain-only fields, assign the new value in memory, call
`config.Save`, and restore the previous map value on failure.

`SetSessionAgent` must preserve `AgentPort`, call `Store.Put`, and restore the
old in-memory value with `Store.Put(old)` if the first write fails. The rollback
call may also fail on disk, but it resets the store map before returning the
original persistence error.

- [ ] **Step 5: Run focused and package tests and verify GREEN**

Run:

```bash
rtk go test ./internal/app -run 'Test(UpdateProject|SetSessionAgent)' -count=1
rtk go test ./internal/app ./internal/config ./internal/session -count=1
```

Expected: all selected packages pass.

### Task 2: Parked OpenCode Session Launching

**Files:**
- Modify: `internal/app/app.go`
- Test: `internal/app/app_test.go`

- [ ] **Step 1: Write the failing missing-port relaunch test**

Add a parked session with `Agent: "opencode"` and `AgentPort: 0`, arrange for
`HasSession` to report absent, then call `OpenSession`. Assert the fake tmux
receives `opencode --port 4096` and the session store now contains port 4096.

- [ ] **Step 2: Write the live-session non-mutation test**

Create a live tmux session whose stored agent was edited to `codex`. Assert
`OpenSession` attaches without `NewSession`, without killing tmux, and without
changing any other session metadata.

- [ ] **Step 3: Run lifecycle tests and verify RED**

Run:

```bash
rtk go test ./internal/app -run 'TestOpenSession(DeadAllocatesOpenCodePort|Alive)' -count=1
```

Expected: the missing-port case fails because current relaunch code sends plain
`opencode` and does not persist a port.

- [ ] **Step 4: Allocate and persist the port before tmux creation**

In the `!has` branch of `OpenSession`, use `s.AgentName()` as today. When it is
`opencode`:

```go
if s.AgentPort == 0 {
    s.AgentPort = a.nextOpenCodePort()
    if err := a.Store.Put(s); err != nil {
        return "", fmt.Errorf("store opencode port: %w", err)
    }
}
cmd = fmt.Sprintf("opencode --port %d", s.AgentPort)
```

Do not update the session agent from the project and do not enter this branch
for a live tmux session.

- [ ] **Step 5: Run lifecycle and app tests and verify GREEN**

Run:

```bash
rtk go test ./internal/app -run 'TestOpenSession' -count=1
rtk go test ./internal/app -count=1
```

Expected: all OpenSession and app tests pass.

### Task 3: TUI Session and Project Editors

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/keys.go`
- Modify: `internal/tui/messages.go`
- Modify: `internal/tui/update.go`
- Modify: `internal/tui/form.go`
- Modify: `internal/tui/click_test.go`
- Test: `internal/tui/update_test.go`

- [ ] **Step 1: Extend the fake backend and write failing shortcut tests**

Add `SetSessionAgent` and `UpdateProject` methods to `fakeBackend`, recording:

```go
type sessionAgentCall struct{ id, agent string }
type updateProjectCall struct {
    name string
    p    config.Project
}
```

Add tests proving `e` opens `ModeEditSession` only when a session exists,
prefills its agent, cycles with arrows, saves the expected backend call, and
cancels without a call. Add tests proving `E` opens `ModeEditProject`, prefills
the active project's values, preserves its name and kind on submission, and
cancels without a call.

- [ ] **Step 2: Run the TUI edit tests and verify RED**

Run:

```bash
rtk go test ./internal/tui -run 'TestEdit(Session|Project)' -count=1
```

Expected: compilation fails because the backend methods, modes, and edit form
state do not exist.

- [ ] **Step 3: Add keys, modes, form state, and backend interface methods**

Add `EditSession` bound to `e` and `EditProject` bound to `E`. Add
`ModeEditSession` and `ModeEditProject`, plus:

```go
type sessionForm struct {
    agentIdx int
    err      string
}
```

Add the approved backend signatures and result messages:

```go
type SessionAgentUpdatedMsg struct {
    Session session.Session
    Err     error
}

type ProjectUpdatedMsg struct {
    Name string
    Err  error
}
```

Create helpers that map an effective agent name to `agentChoices` and prefill
`projectForm` from `config.Project`. Keep the name input read-only in edit mode
by rendering it as context rather than focusing or submitting it.

- [ ] **Step 4: Implement list entry and edit update flows**

In `updateList`, `e` opens the selected session editor and `E` opens the active
project editor. Implement:

```go
func (m *Model) updateEditSession(msg tea.KeyMsg) (tea.Model, tea.Cmd)
func (m *Model) updateEditProject(msg tea.KeyMsg) (tea.Model, tea.Cmd)
```

Both functions handle cancel, selector/navigation changes, and async submit.
Result handling refreshes sessions or projects, keeps the current selection,
returns to `ModeList` on success, and leaves the relevant form open with an
inline error on failure.

For plain projects, the edit-project focus cycle includes only repo and agent.
For git projects it includes repo, base branch, branch prefix, agent, and the
worktree toggle. Add-project behavior remains unchanged.

- [ ] **Step 5: Render both editors**

Add `renderEditSession` with read-only project and session context plus the
agent selector. Refactor project form rendering only enough to share the
existing controls while giving edit mode the title `Edit project`, showing the
project name as fixed context, and hiding git-only controls for plain projects.

- [ ] **Step 6: Run focused and full TUI tests and verify GREEN**

Run:

```bash
rtk go test ./internal/tui -run 'TestEdit(Session|Project)' -count=1
rtk go test ./internal/tui -count=1
```

Expected: all TUI tests pass.

### Task 4: Help, Screenshots, Documentation, and Full Verification

**Files:**
- Modify: `internal/tui/help.go`
- Test: `internal/tui/help_test.go`
- Modify: `cmd/uishot/main.go`
- Modify: `README.md`

- [ ] **Step 1: Write failing help assertions**

Extend the help test to assert the rendered overlay contains `edit session`,
`edit project`, `e`, and `E`.

- [ ] **Step 2: Run the help test and verify RED**

Run:

```bash
rtk go test ./internal/tui -run TestHelp -count=1
```

Expected: failure because edit commands are absent.

- [ ] **Step 3: Update help and README**

Add `e` under Sessions and `E` under Projects in `helpGroups`. Add `e` / `E`
to the README key summary without changing unrelated documentation.

- [ ] **Step 4: Add deterministic screenshot scenarios**

Extend `cmd/uishot`'s fake backend with the two edit methods and add:

```go
"edit-session": {"e"},
"edit-project": {"E"},
```

Ensure sample project data includes agent, base branch, branch prefix, and
worktree values that make prefill visible.

- [ ] **Step 5: Run formatting and the complete test suite**

Run:

```bash
rtk gofmt -w internal/app/app.go internal/app/app_test.go internal/tui/model.go internal/tui/keys.go internal/tui/messages.go internal/tui/update.go internal/tui/form.go internal/tui/help.go internal/tui/click_test.go internal/tui/update_test.go internal/tui/help_test.go cmd/uishot/main.go
rtk go test ./... -count=1
rtk go vet ./...
rtk git diff --check
```

Expected: all commands exit zero.

- [ ] **Step 6: Capture and inspect screenshots**

Run:

```bash
rtk ./scripts/screenshot.sh edit-session /tmp/moomux-edit-session.png
rtk ./scripts/screenshot.sh edit-project /tmp/moomux-edit-project.png
rtk ./scripts/screenshot.sh edit-session /tmp/moomux-edit-session-narrow.png --width 70 --height 28
rtk ./scripts/screenshot.sh edit-project /tmp/moomux-edit-project-narrow.png --width 70 --height 32
```

If the script does not accept trailing dimensions, run `cmd/uishot` with
`-width` and `-height` through the existing render pipeline instead. Inspect
all four PNGs for text overlap, selector alignment, stable form dimensions,
and readable narrow-terminal behavior.

- [ ] **Step 7: Review final scope**

Run:

```bash
rtk git status --short
rtk git diff --stat
rtk git diff
```

Confirm the worktree contains the two uncommitted design documents plus only
the implementation, test, screenshot-harness, help, and README changes
described above. Do not commit.
