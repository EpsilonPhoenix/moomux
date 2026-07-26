# Project and Session Agent Editing

## Goal

Allow users to edit an existing project's settings and an existing session's
coding agent from the TUI without changing any running tmux process.

The design separates three layers:

1. A project defines defaults used when a session is created.
2. A session owns the agent setting copied from that project default, unless
   the user chooses a different agent while creating the session.
3. A tmux process is a running realization of the session configuration.
   Editing configuration never restarts or mutates a live tmux process.

## User Interface

### Session editor

Pressing `e` opens an editor for the selected session. The overlay shows the
project and session name as read-only context and exposes only the agent
selector:

- `Left` and `Right` select `claude`, `codex`, or `opencode`.
- `Enter` saves.
- `Esc` cancels.

The session's project and name are intentionally immutable. Moving a session
between projects would change its ownership and ID. Renaming it would affect
its ID, tmux session name, worktree directory, and potentially its branch.
Those operations require migration behavior and are outside this feature.

### Project editor

Pressing `E` opens an editor for the active project. It reuses the add-project
form's controls, navigation, selectors, toggles, and field hints, with values
prefilled from the current project.

For git projects, the editable fields are:

- Repository path
- Base branch
- Branch prefix
- Default agent
- Worktree behavior

For plain projects, the editable fields are:

- Repository path
- Default agent

The project name and kind (`git` or `plain`) are read-only. Changing either
requires migration behavior and is outside this feature.

The existing form controls remain consistent:

- `Tab`, `Shift+Tab`, `Up`, and `Down` move between fields.
- `Left` and `Right` change selectors and toggles.
- `Enter` saves.
- `Esc` cancels.

The help overlay lists `e` as "edit session" and `E` as "edit project." This
matches the existing lowercase session and uppercase project convention used
by `d` and `D`.

## State Semantics

`Project.Agent` is a default, not a live reference. Creating a session copies
the chosen agent into `Session.Agent`. The new-session form continues to
preselect the active project's default while allowing a one-off override.

After creation, project and session agent settings are independent:

- Editing `Project.Agent` affects only sessions created afterward.
- Editing `Session.Agent` affects the next tmux process created for that
  session.
- Neither edit changes or restarts a live tmux session.
- Opening a session with a live tmux process only attaches to it.
- Opening a parked session recreates tmux using `Session.Agent`.

Other edited project settings also affect only sessions created afterward.
Existing sessions retain their stored worktree paths. Editing a repository path
or worktree behavior does not move or rewrite existing worktrees.

### OpenCode ports

`Session.AgentPort` is retained when the session's agent changes. This prevents
another session from taking a port that may still belong to a live OpenCode
tmux process after its session configuration has been edited.

When a parked session is opened with `Session.Agent == "opencode"` and has no
stored port, the application allocates the next available OpenCode port and
persists it before creating tmux. Subsequent restarts reuse that port.

## Backend Operations

The TUI backend gains two operations:

```go
UpdateProject(name string, project config.Project) error
SetSessionAgent(id, agent string) (session.Session, error)
```

`UpdateProject`:

1. Confirms the named project exists.
2. Preserves the existing project's immutable `Kind`.
3. Validates and expands the repository path.
4. Requires git-project paths to identify a git repository.
5. Applies git-only defaults and fields only to git projects.
6. Writes the updated configuration.
7. Restores the previous in-memory project value if the write fails.

For a plain project, saving follows current plain-project behavior: the target
directory may be created if it does not exist, and git-only settings remain
cleared.

`SetSessionAgent`:

1. Confirms the session exists.
2. Accepts only `claude`, `codex`, or `opencode`.
3. Updates `Session.Agent` without touching tmux.
4. Persists through the existing session store.
5. Restores the previous in-memory session value if persistence fails.
6. Returns the updated session for the TUI message flow.

Agent validation should be shared by project and session mutations so invalid
configuration cannot reach command selection.

## TUI Flow

The TUI gains `ModeEditSession` and `ModeEditProject`.

The session editor uses a small form model containing the selected agent index
and any inline error. The project editor reuses the existing `projectForm`
state, initialized from the active project, while its mode determines whether
submission adds or updates a project.

Both save operations run as Bubble Tea commands. While a save is in flight,
duplicate submission is suppressed. Their result messages contain enough
context to keep the relevant project or session selected.

On success, the editor closes and the list shows:

- `updated session <name>`
- `updated project <name>`

On validation or persistence failure, the editor remains open and renders the
error inline. Canceling either editor performs no backend call.

## Error Handling

- Unknown project and session identifiers return explicit errors.
- Unknown agent values are rejected rather than falling back to Claude.
- Invalid git repository paths keep the project editor open.
- Configuration save failure restores the prior project value in memory.
- Session-store failure restores the prior session value in memory, leaves the
  prior persisted session intact, and keeps the session editor open.
- Failure to allocate or persist an OpenCode port prevents tmux creation and
  returns an error; it does not launch an untracked process.
- Editing configuration never kills or recreates a live tmux process.

## Testing

### Backend

- Project updates persist each editable git-project field.
- Plain-project edits expose and persist only repository and agent settings.
- Project kind remains unchanged.
- Unknown projects, invalid agents, and invalid repository paths fail.
- A failed config save restores the previous in-memory project.
- Session-agent edits persist without tmux calls.
- A failed session-store write restores the previous in-memory agent.
- Unknown sessions and invalid agents fail.
- Existing OpenCode ports survive agent edits.

### Session lifecycle

- Editing a project default does not alter existing session records.
- A new session copies the current project default unless explicitly
  overridden.
- Editing a live session's agent does not restart or kill tmux.
- Opening a live session attaches without applying an edited session agent.
- Opening a parked session launches its edited `Session.Agent`.
- A parked OpenCode session reuses its stored port.
- A parked session newly changed to OpenCode allocates and persists a port
  before tmux creation.

### TUI

- `e` opens an agent-only editor prefilled from the selected session.
- `E` opens a project editor prefilled from the active project.
- Project and session context remains selected after successful saves.
- Save and cancel dispatch the expected backend calls.
- Backend errors remain visible in the appropriate form.
- Duplicate submissions are suppressed while saving.
- The help overlay documents both shortcuts.

### Visual verification

Add screenshot scenarios for both edit overlays. Capture and inspect each at
normal and narrow terminal sizes, confirming that labels, selectors, hints,
and error text remain readable without overlap or resizing.

## Out of Scope

- Restarting a live tmux session when configuration changes
- Renaming a session
- Moving a session between projects
- Renaming a project
- Converting between git and plain project kinds
- Moving existing repositories, worktrees, branches, or tmux sessions
