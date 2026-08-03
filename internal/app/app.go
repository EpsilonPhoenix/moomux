// Package app glues config, session store, tmux, terminal and gitwt into a TUI Backend.
package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/erickgnclvs/moomux/internal/claudehook"
	"github.com/erickgnclvs/moomux/internal/codexhook"
	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/gitwt"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/terminal"
	"github.com/erickgnclvs/moomux/internal/tmux"
)

type App struct {
	Cfg          *config.Config
	CfgPath      string
	Store        *session.Store
	Tmux         *tmux.Client
	Terminal     terminal.TerminalOpener
	Git          *gitwt.Client
	WorktreeRoot string
}

// agentCmd returns the CLI binary name for the given agent.
func agentCmd(agent string) string {
	switch agent {
	case "codex":
		return "codex"
	case "opencode":
		return "opencode"
	default:
		return "claude"
	}
}

// needsInputInstallers maps an agent name to the function that wires its
// "needs input" hooks into the user's global config. Agents with no entry
// here (opencode) don't support the needs-input state yet — add a sibling
// package and a map entry to bring one in. changed reports whether the call
// actually wrote something new (see codexHooksHint).
//
// Both installers are global rather than per-worktree/per-project: each
// agent's own trust model would otherwise force re-approving hooks on every
// new worktree (see claudehook.EnsureHooksInstalled and codexhook.EnsureHooks
// doc comments for why).
var needsInputInstallers = map[string]func(home string) (changed bool, err error){
	"claude": claudehook.EnsureHooksInstalled,
	"codex":  codexhook.EnsureHooks,
}

func validateAgent(agent string) error {
	switch agent {
	case "claude", "codex", "opencode":
		return nil
	default:
		return fmt.Errorf("unknown agent %q", agent)
	}
}

// TmuxSessionName returns the tmux session name for a moomux session. The
// short ID hash keeps names unique across projects — "moomux-<name>" alone
// collides when two projects each have a session with the same name, and a
// name-only scheme is also ambiguous ("a" + "b-c" vs "a-b" + "c"). Nothing
// ever parses this back; only uniqueness and greppability matter.
func TmuxSessionName(id, name string) string {
	sum := sha256.Sum256([]byte(id))
	return "moomux-" + name + "-" + hex.EncodeToString(sum[:2])
}

func WorktreeRootDefault() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "moomux", "worktrees")
}

// nextOpenCodePort returns the next available port for an OpenCode session.
// Starts at 4096 and increments past the highest AgentPort recorded across
// all sessions; AgentPort is only ever non-zero on OpenCode sessions, so
// this is effectively scoped to those without needing an explicit filter.
func (a *App) nextOpenCodePort() int {
	port := 4096
	for _, s := range a.Store.All() {
		if s.AgentPort >= port {
			port = s.AgentPort + 1
		}
	}
	return port
}

func (a *App) Projects() []string { return a.Cfg.OrderedProjectNames() }

// MoveProject shifts the project with the given name by delta positions (-1
// left, +1 right) in the manual project order and persists it. It's a no-op
// if the move would go out of bounds.
func (a *App) MoveProject(name string, delta int) error {
	if err := config.Reload(a.CfgPath, a.Cfg); err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	order := a.Projects()
	idx := -1
	for i, n := range order {
		if n == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("unknown project %q", name)
	}
	j := idx + delta
	if j < 0 || j >= len(order) {
		return nil
	}
	order[idx], order[j] = order[j], order[idx]
	prev := a.Cfg.Order
	a.Cfg.Order = order
	if err := config.Save(a.CfgPath, a.Cfg); err != nil {
		a.Cfg.Order = prev
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func (a *App) Sessions() []session.Session { return a.Store.All() }

// sanitizeName collapses anything that isn't alphanumeric/-/_ to "-", so the
// result is safe as a git branch name, filesystem path component, and tmux
// session name all at once.
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "session"
	}
	return out
}

// deriveNameFromBranch turns a branch name like "feature/login-page" into a
// filesystem/tmux-safe session name like "login-page".
func deriveNameFromBranch(branch string) string {
	name := branch
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return sanitizeName(name)
}

// uniqueNameFromBranch derives a session name from branch and, if it already
// collides with an existing session in project, appends -2, -3, ... until free.
func (a *App) uniqueNameFromBranch(project, branch string) string {
	base := deriveNameFromBranch(branch)
	name := base
	for i := 2; ; i++ {
		if _, ok := a.Store.Get(session.MakeID(project, name)); !ok {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

// tmuxSessionUsingWorktree returns the tmux session name of any tracked
// session whose worktree is path and whose tmux session is still alive, so
// callers can avoid force-removing a worktree out from under a live pane
// (see internal/tmux/tmux.go's PaneCwd doc comment for what that breaks).
// Empty result means no live session is using path.
func (a *App) tmuxSessionUsingWorktree(path string) (string, error) {
	for _, s := range a.Store.All() {
		if s.WorktreePath != path {
			continue
		}
		has, err := a.Tmux.HasSession(s.TmuxSession)
		if err != nil {
			return "", err
		}
		if has {
			return s.TmuxSession, nil
		}
	}
	return "", nil
}

// CreateSession's hint, when non-empty, is a user-facing instruction
// (e.g. "run: tmux attach -t ...") to show alongside success — it is
// not an error.
func (a *App) CreateSession(project, name, agent, existingBranch, ticket string) (session.Session, string, error) {
	proj, ok := a.Cfg.Projects[project]
	if !ok {
		return session.Session{}, "", fmt.Errorf("unknown project %q", project)
	}
	if name == "" {
		if existingBranch == "" {
			return session.Session{}, "", fmt.Errorf("session name required")
		}
		name = a.uniqueNameFromBranch(project, existingBranch)
	} else {
		name = sanitizeName(name)
	}
	if agent == "" {
		agent = proj.AgentName()
	}
	if err := validateAgent(agent); err != nil {
		return session.Session{}, "", err
	}
	if _, exists := a.Store.Get(session.MakeID(project, name)); exists {
		// Same name means the same worktree path — creating it again would
		// hijack the existing session's checkout.
		return session.Session{}, "", fmt.Errorf("session %q already exists in project %q", name, project)
	}
	var wt string
	tmuxName := TmuxSessionName(session.MakeID(project, name), name)
	branch := ""

	if proj.UsesWorktree() {
		wt = filepath.Join(a.WorktreeRoot, project, name)
	} else {
		wt = proj.Repo
	}

	slog.Info("create session", "project", project, "name", name, "agent", agent, "worktree", wt, "branch", existingBranch)

	if proj.UsesWorktree() {
		fetchTarget := proj.BaseBranch
		if existingBranch != "" {
			branch = existingBranch
			fetchTarget = existingBranch
		} else {
			branch = name
			if proj.BranchPrefix != "" {
				branch = proj.BranchPrefix + "/" + name
			}
		}
		if a.Git.HasRemote(proj.Repo, "origin") {
			_ = a.Git.Fetch(proj.Repo, fetchTarget) // best-effort
		}
		var err error
		if existingBranch != "" {
			if staleWT, ferr := a.Git.WorktreeForBranch(proj.Repo, branch); ferr == nil && staleWT != "" && staleWT != wt {
				clean, cerr := a.Git.IsWorktreeClean(staleWT)
				if cerr != nil {
					return session.Session{}, "", fmt.Errorf("check existing worktree %s: %w", staleWT, cerr)
				}
				if !clean {
					return session.Session{}, "", fmt.Errorf("branch %q is already checked out at %s with uncommitted changes", branch, staleWT)
				}
				if busy, terr := a.tmuxSessionUsingWorktree(staleWT); terr != nil {
					return session.Session{}, "", fmt.Errorf("check tmux sessions for %s: %w", staleWT, terr)
				} else if busy != "" {
					return session.Session{}, "", fmt.Errorf("branch %q is already checked out at %s, in use by tmux session %q", branch, staleWT, busy)
				}
				if err := a.Git.RemoveWorktree(proj.Repo, staleWT); err != nil {
					return session.Session{}, "", fmt.Errorf("remove stale worktree %s: %w", staleWT, err)
				}
				slog.Info("removed stale clean worktree for branch", "branch", branch, "path", staleWT)
			}
			err = a.Git.AddWorktreeExisting(proj.Repo, wt, branch)
		} else {
			err = a.Git.AddWorktree(proj.Repo, wt, branch, proj.BaseBranch)
		}
		if err != nil {
			slog.Error("git worktree add failed", "repo", proj.Repo, "path", wt, "branch", branch, "err", err)
			return session.Session{}, "", fmt.Errorf("git worktree add: %w", err)
		}
		slog.Info("worktree added", "path", wt, "branch", branch)
	}
	// Not gated on proj.UsesWorktree(): both installers ignore the worktree
	// path entirely (see needsInputInstallers's doc comment) and write to a
	// fixed global location, so a plain/no-worktree project deserves it just
	// as much as a worktree one.
	hooksHint := ""
	if install, ok := needsInputInstallers[agent]; ok {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Warn("needs-input hook install failed", "agent", agent, "err", err)
		} else if changed, err := install(home); err != nil {
			slog.Warn("needs-input hook install failed", "agent", agent, "err", err)
		} else if changed {
			hooksHint = codexHooksHint(agent)
		}
	}
	if agent == "claude" {
		if home, err := os.UserHomeDir(); err != nil {
			slog.Warn("claude trust write failed", "err", err)
		} else if err := claudehook.TrustDirectory(home, wt); err != nil {
			slog.Warn("claude trust write failed", "path", wt, "err", err)
		}
	}
	cmd := agentCmd(agent)
	agentPort := 0
	if agent == "opencode" {
		agentPort = a.nextOpenCodePort()
		cmd = fmt.Sprintf("opencode --port %d", agentPort)
	}

	if err := a.Tmux.NewSession(tmuxName, wt, cmd, name); err != nil {
		slog.Error("tmux new-session failed", "name", tmuxName, "cwd", wt, "err", err)
		return session.Session{}, "", fmt.Errorf("tmux new-session: %w", err)
	}
	slog.Info("tmux session created", "name", tmuxName)
	tabID, hint, err := a.openTerminal("", tmuxName, name)
	if err != nil {
		// The worktree and tmux session already exist at this point;
		// failing would strand them outside the store. Degrade to a
		// manual-attach hint instead.
		slog.Error("terminal open failed", "tmux_session", tmuxName, "name", name, "err", err)
		hint = fmt.Sprintf("couldn't open a terminal (%v) — attach yourself: tmux attach -t %s", err, tmuxName)
	}
	if hooksHint != "" {
		hint = joinHints(hooksHint, hint)
	}
	slog.Info("terminal opened", "tmux_session", tmuxName)

	s := session.Session{
		ID:           session.MakeID(project, name),
		Project:      project,
		Name:         name,
		Branch:       branch,
		NewBranch:    proj.UsesWorktree() && existingBranch == "",
		WorktreePath: wt,
		TmuxSession:  tmuxName,
		CreatedAt:    time.Now().UTC(),
		Agent:        agent,
		AgentPort:    agentPort,
		Ticket:       ticket,
		TermTabID:    tabID,
	}
	if err := a.Store.Put(s); err != nil {
		slog.Error("store put failed", "id", s.ID, "err", err)
		err = fmt.Errorf("store: %w", err)
		if hint != "" {
			// The tmux session is already running at this point (see
			// above); don't let the caller lose the manual-attach hint
			// just because persisting the session record also failed.
			err = fmt.Errorf("%w; %s", err, hint)
		}
		return s, hint, err
	}
	return s, hint, nil
}

// SendPrompt types text into a session's agent pane, for handing a freshly
// spawned session its initial task. No-op if prompt is empty.
func (a *App) SendPrompt(tmuxSession, prompt string) error {
	if prompt == "" {
		return nil
	}
	return a.Tmux.SendKeys(tmuxSession, prompt)
}

// paneStablePoll/paneStableChecks/paneReadyTimeout tune StartFirstPrompt's
// readiness wait: an agent CLI's startup render (splash, model warmup, hook
// setup) takes a variable amount of time, and typing into it before it's
// idle at its input box means the keystrokes land on a screen that never
// reads them — visible as text landing on the plain shell prompt, before the
// agent has even opened. Waiting for two consecutive identical captures is a
// generic proxy for "done redrawing, waiting on input" that doesn't need to
// know anything about the specific agent CLI's UI — but the idle shell
// prompt right after the launch command was typed looks just as "stable" as
// an idle agent input box does, so waitForPaneReady first waits for the pane
// to change at all (proving the agent process actually took over the
// terminal) before it starts checking for stability, using the whole
// readiness budget for that wait rather than a shorter carve-out of it.
const (
	paneStablePoll   = 300 * time.Millisecond
	paneStableChecks = 2
	paneReadyTimeout = 15 * time.Second
)

// waitForPaneReady blocks until tmuxSession's pane content has visibly
// changed from its pre-launch state and then stops changing across
// paneStableChecks consecutive polls, or paneReadyTimeout elapses overall —
// whichever comes first. Timing out during either phase is not an error: the
// caller proceeds best-effort regardless, same as the fixed-delay behavior
// this replaces.
func (a *App) waitForPaneReady(tmuxSession string) {
	deadline := time.Now().Add(paneReadyTimeout)
	before, _ := a.Tmux.CapturePane(tmuxSession)

	last := before
	changed := false
	for !changed && time.Now().Before(deadline) {
		time.Sleep(paneStablePoll)
		if cur, err := a.Tmux.CapturePane(tmuxSession); err == nil {
			changed = cur != before
			last = cur
		}
	}
	if !changed {
		// Never saw anything but the pre-launch state within the whole
		// budget — proceeding to check "stability" against it would just
		// immediately declare it stable, defeating the point of this wait.
		return
	}

	stable := 0
	for {
		cur, err := a.Tmux.CapturePane(tmuxSession)
		if err == nil && cur == last && cur != "" {
			stable++
			if stable >= paneStableChecks {
				return
			}
		} else {
			stable = 0
		}
		last = cur
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(paneStablePoll)
	}
}

// firstPromptSubmitDelay is the gap between typing the prompt and pressing
// Enter — see Client.SendLiteral's doc comment for why they must be two
// separate send-keys calls, not one.
const firstPromptSubmitDelay = 300 * time.Millisecond

// StartFirstPrompt waits for a freshly created session's agent pane to
// finish starting up, types prompt into it, and separately presses Enter to
// start the agent working on it. No-op if prompt is empty.
func (a *App) StartFirstPrompt(tmuxSession, prompt string) error {
	if prompt == "" {
		return nil
	}
	a.waitForPaneReady(tmuxSession)
	if err := a.Tmux.SendLiteral(tmuxSession, prompt); err != nil {
		return err
	}
	time.Sleep(firstPromptSubmitDelay)
	return a.Tmux.PressEnter(tmuxSession)
}

// MoveSession shifts the session with the given id by delta positions (-1
// up, +1 down) within its project's session list, and persists the new
// order. It's a no-op if the move would go out of bounds.
func (a *App) MoveSession(id string, delta int) error {
	s, ok := a.Store.Get(id)
	if !ok {
		return fmt.Errorf("unknown session %q", id)
	}
	peers := a.Store.ByProject(s.Project)
	idx := -1
	for i, p := range peers {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("unknown session %q", id)
	}
	j := idx + delta
	if j < 0 || j >= len(peers) {
		return nil
	}
	peers[idx], peers[j] = peers[j], peers[idx]
	return a.Store.Reorder(peers)
}

func (a *App) SetSessionTags(id, ticket, pr string) (session.Session, error) {
	s, ok := a.Store.Get(id)
	if !ok {
		return session.Session{}, fmt.Errorf("unknown session %q", id)
	}
	s.Ticket = ticket
	s.PR = pr
	if err := a.Store.Put(s); err != nil {
		return s, fmt.Errorf("store: %w", err)
	}
	return s, nil
}

func (a *App) SetSessionAgent(id, agent string) (session.Session, error) {
	if err := validateAgent(agent); err != nil {
		return session.Session{}, err
	}
	s, ok := a.Store.Get(id)
	if !ok {
		return session.Session{}, fmt.Errorf("unknown session %q", id)
	}
	previous := s
	s.Agent = agent
	if err := a.Store.Put(s); err != nil {
		_ = a.Store.Put(previous)
		return session.Session{}, fmt.Errorf("store: %w", err)
	}
	return s, nil
}

// SetSessionArchived hides (or restores) a session from the default list
// without touching its tmux session or worktree — the reverse of
// DeleteSession, which is destructive.
func (a *App) SetSessionArchived(id string, archived bool) (session.Session, error) {
	return a.Store.SetArchived(id, archived)
}

// repairNeedsInputHooks backfills a session's needs-input hooks on open, for
// sessions created before this feature existed (or before its agent got an
// entry in needsInputInstallers, or before a newer moomux build added a hook
// event an older one didn't install). Each installer is idempotent, so
// running this on every open is cheap and safe. Not gated on the project
// using worktrees — both installers ignore the worktree path and write to a
// fixed global location (see needsInputInstallers), so a plain-project
// session deserves the repair just as much as a worktree one. Returns a hint
// (see codexHooksHint) if this call is what just installed or changed
// codex's hooks, or "" otherwise.
func (a *App) repairNeedsInputHooks(s session.Session) string {
	if _, ok := a.Cfg.Projects[s.Project]; !ok {
		return ""
	}
	install, ok := needsInputInstallers[s.AgentName()]
	if !ok {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("needs-input hook repair failed", "agent", s.AgentName(), "err", err)
		return ""
	}
	changed, err := install(home)
	if err != nil {
		slog.Warn("needs-input hook repair failed", "agent", s.AgentName(), "err", err)
		return ""
	}
	if !changed {
		return ""
	}
	return codexHooksHint(s.AgentName())
}

// codexHooksHint returns a message telling the user how to activate the
// codex hooks that were just installed or changed, or "" if agent isn't
// codex. Codex requires an explicit `/hooks` review before it will run a new
// or changed hook entry (trust is keyed by the file's path and per-entry
// content hash — see codexhook.EnsureHooks's doc comment) — without this
// nudge, needs-input would silently never activate for the new/changed
// entry and there'd be no indication why. Callers only call this when
// needsInputInstallers reported changed=true, so it naturally re-fires
// whenever a future moomux release adds another hook event, not just once
// ever.
func codexHooksHint(agent string) string {
	if agent != "codex" {
		return ""
	}
	return "Codex needs-input hooks were installed/updated in ~/.codex/hooks.json — run /hooks inside Codex once to trust them"
}

// joinHints combines two non-empty hint strings for display, e.g. a
// one-time educational note alongside a manual-attach fallback. Either may
// be empty.
func joinHints(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}

func (a *App) OpenSession(id string) (string, error) {
	s, ok := a.Store.Get(id)
	if !ok {
		return "", fmt.Errorf("unknown session %q", id)
	}
	hooksHint := a.repairNeedsInputHooks(s)
	has, err := a.Tmux.HasSession(s.TmuxSession)
	slog.Info("open session", "id", id, "tmux_session", s.TmuxSession, "worktree", s.WorktreePath, "tmux_has_session", has)
	if err != nil {
		slog.Error("HasSession error", "id", id, "err", err)
		return "", err
	}
	if has {
		if cwd, err := a.Tmux.PaneCwd(s.TmuxSession); err == nil && cwd != s.WorktreePath {
			slog.Info("tmux session cwd mismatch, recreating", "tmux_session", s.TmuxSession, "pane_cwd", cwd, "want", s.WorktreePath)
			if err := a.Tmux.KillSession(s.TmuxSession); err != nil {
				slog.Error("KillSession failed", "id", id, "err", err)
				return "", err
			}
			has = false
		}
	}
	if !has {
		if want := TmuxSessionName(s.ID, s.Name); s.TmuxSession != want {
			// Lazy migration to the collision-free naming scheme: rename
			// only while the tmux session is dead and being recreated
			// anyway, so a live session never loses its identifier.
			slog.Info("migrating tmux session name", "from", s.TmuxSession, "to", want)
			s.TmuxSession = want
			if err := a.Store.Put(s); err != nil {
				return "", fmt.Errorf("store tmux name: %w", err)
			}
		}
		slog.Info("tmux session absent, recreating", "tmux_session", s.TmuxSession, "cwd", s.WorktreePath)
		cmd := agentCmd(s.AgentName())
		if s.AgentName() == "opencode" {
			if s.AgentPort == 0 {
				s.AgentPort = a.nextOpenCodePort()
				if err := a.Store.Put(s); err != nil {
					return "", fmt.Errorf("store opencode port: %w", err)
				}
			}
			cmd = fmt.Sprintf("opencode --port %d", s.AgentPort)
		}
		if err := a.Tmux.NewSession(s.TmuxSession, s.WorktreePath, cmd, s.Name); err != nil {
			slog.Error("NewSession failed", "id", id, "tmux_session", s.TmuxSession, "cwd", s.WorktreePath, "err", err)
			return "", err
		}
	}
	a.Tmux.ConfigureTitleTracking(s.TmuxSession, s.Name)
	tabID, hint, err := a.openTerminal(s.TermTabID, s.TmuxSession, s.Name)
	if err != nil {
		// The tmux session is up regardless; give the user a way in.
		slog.Error("Terminal.OpenSession failed", "id", id, "tmux_session", s.TmuxSession, "name", s.Name, "err", err)
		hint = fmt.Sprintf("couldn't open a terminal (%v) — attach yourself: tmux attach -t %s", err, s.TmuxSession)
	}
	if hooksHint != "" {
		hint = joinHints(hooksHint, hint)
	}
	if tabID != s.TermTabID {
		s.TermTabID = tabID
		if err := a.Store.Put(s); err != nil {
			slog.Error("store iterm tab id failed", "id", id, "err", err)
		}
	}
	slog.Info("session opened", "id", id)
	return hint, nil
}

// openTerminal opens tmuxSession in a terminal, reusing tabID if the
// terminal supports jumping back to a specific tab (currently just
// iTerm2). Returns the tab id to remember for next time — empty for
// terminals without tab addressing.
func (a *App) openTerminal(tabID, tmuxSession, name string) (newTabID, hint string, err error) {
	if reopener, ok := a.Terminal.(terminal.TabReopener); ok {
		return reopener.OpenTab(tabID, tmuxSession, name)
	}
	hint, err = a.Terminal.OpenSession(tmuxSession, name)
	return "", hint, err
}

// TmuxAliveAll returns id→alive for every stored session using a single
// tmux list-sessions call instead of one has-session subprocess per session.
func (a *App) TmuxAliveAll() map[string]bool {
	live := a.Tmux.LiveSessions()
	all := a.Store.All()
	result := make(map[string]bool, len(all))
	for _, s := range all {
		result[s.ID] = live[s.TmuxSession]
	}
	return result
}

// KillTmux kills the tmux session but keeps the moomux session entry
// (and its worktree) intact, so it can be re-opened later.
func (a *App) KillTmux(id string) error {
	s, ok := a.Store.Get(id)
	if !ok {
		return fmt.Errorf("unknown session %q", id)
	}
	has, err := a.Tmux.HasSession(s.TmuxSession)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	return a.Tmux.KillSession(s.TmuxSession)
}

func (a *App) validateProject(name string, p *config.Project) error {
	if name == "" {
		return fmt.Errorf("project name required")
	}
	if strings.ContainsAny(name, " \t/\\") {
		return fmt.Errorf("project name cannot contain spaces or slashes")
	}
	if _, exists := a.Cfg.Projects[name]; exists {
		return fmt.Errorf("project %q already exists", name)
	}
	if p.Repo == "" {
		return fmt.Errorf("repo path required")
	}
	// p.Agent == "" is a legitimate "use the default" value at rest (see
	// AgentName), so validate the resolved name rather than the raw field —
	// only a genuinely bogus value (e.g. a config typo) should be rejected.
	if err := validateAgent(p.AgentName()); err != nil {
		return err
	}
	if p.BaseBranch == "" {
		p.BaseBranch = "main"
	}
	p.Repo = config.ExpandHome(p.Repo)
	return nil
}

func (a *App) saveProject(name string, p config.Project) error {
	if err := config.Reload(a.CfgPath, a.Cfg); err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	if a.Cfg.Projects == nil {
		a.Cfg.Projects = map[string]config.Project{}
	}
	a.Cfg.Projects[name] = p
	if err := config.Save(a.CfgPath, a.Cfg); err != nil {
		delete(a.Cfg.Projects, name)
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func (a *App) AddProject(name string, p config.Project) error {
	if err := a.validateProject(name, &p); err != nil {
		return err
	}
	if err := gitwt.IsRepo(p.Repo); err != nil {
		return err
	}
	p.Kind = "git"
	return a.saveProject(name, p)
}

// InitProjectAndAdd creates the directory (if missing), runs `git init` with the
// given base branch + an empty initial commit, then saves the project.
func (a *App) InitProjectAndAdd(name string, p config.Project) error {
	if err := a.validateProject(name, &p); err != nil {
		return err
	}
	if err := gitwt.Init(p.Repo, p.BaseBranch); err != nil {
		return err
	}
	p.Kind = "git"
	return a.saveProject(name, p)
}

// AddPlainProject saves a non-git project. Sessions run directly in the
// project folder; no branches, no worktrees, no per-session isolation.
func (a *App) AddPlainProject(name string, p config.Project) error {
	if err := a.validateProject(name, &p); err != nil {
		return err
	}
	if err := os.MkdirAll(p.Repo, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", p.Repo, err)
	}
	p.Kind = "plain"
	p.BaseBranch = ""
	p.BranchPrefix = ""
	return a.saveProject(name, p)
}

func (a *App) UpdateProject(name string, updated config.Project) error {
	if err := config.Reload(a.CfgPath, a.Cfg); err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	previous, ok := a.Cfg.Projects[name]
	if !ok {
		return fmt.Errorf("unknown project %q", name)
	}
	// updated.Agent == "" is a legitimate "use the default" value at rest
	// (see AgentName), same as in validateProject — validate the resolved
	// name rather than the raw field.
	if err := validateAgent(updated.AgentName()); err != nil {
		return err
	}
	if updated.Repo == "" {
		return fmt.Errorf("repo path required")
	}
	updated.Repo = config.ExpandHome(updated.Repo)
	updated.Kind = previous.Kind

	if updated.NoWorktree != previous.NoWorktree {
		// Existing sessions were created under the old mode; their
		// WorktreePath either is or isn't the repo itself, and deleting
		// them under the flipped mode would target the wrong path.
		for _, s := range a.Store.All() {
			if s.Project == name {
				return fmt.Errorf("cannot change worktree mode while project %q has sessions — delete them first", name)
			}
		}
	}

	if previous.IsPlain() {
		if err := os.MkdirAll(updated.Repo, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", updated.Repo, err)
		}
		updated.BaseBranch = ""
		updated.BranchPrefix = ""
		updated.NoWorktree = false
	} else {
		if err := gitwt.IsRepo(updated.Repo); err != nil {
			return err
		}
		if updated.BaseBranch == "" {
			updated.BaseBranch = "main"
		}
	}

	a.Cfg.Projects[name] = updated
	if err := config.Save(a.CfgPath, a.Cfg); err != nil {
		a.Cfg.Projects[name] = previous
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

func (a *App) RemoveProject(name string) error {
	if err := config.Reload(a.CfgPath, a.Cfg); err != nil {
		return fmt.Errorf("reload config: %w", err)
	}
	if _, ok := a.Cfg.Projects[name]; !ok {
		return fmt.Errorf("unknown project %q", name)
	}
	for _, s := range a.Store.All() {
		if s.Project == name {
			return fmt.Errorf("project %q still has sessions (incl. archived) — delete them first", name)
		}
	}
	saved := a.Cfg.Projects[name]
	delete(a.Cfg.Projects, name)
	if err := config.Save(a.CfgPath, a.Cfg); err != nil {
		a.Cfg.Projects[name] = saved
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// pathWithin reports whether path is strictly inside root.
func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (a *App) DeleteSession(id string) error {
	s, ok := a.Store.Get(id)
	if !ok {
		return fmt.Errorf("unknown session %q", id)
	}
	if has, _ := a.Tmux.HasSession(s.TmuxSession); has {
		if err := a.Tmux.KillSession(s.TmuxSession); err != nil {
			return fmt.Errorf("tmux kill-session: %w", err)
		}
	}
	if proj, ok := a.Cfg.Projects[s.Project]; ok {
		if proj.UsesWorktree() {
			if err := a.Git.RemoveWorktree(proj.Repo, s.WorktreePath); err != nil {
				return fmt.Errorf("remove worktree: %w", err)
			}
			if s.NewBranch && s.Branch != "" {
				_ = a.Git.DeleteBranch(proj.Repo, s.Branch)
			}
		}
	} else if pathWithin(a.WorktreeRoot, s.WorktreePath) {
		// Project gone from config (e.g. hand-edited TOML). Only clean up
		// paths moomux itself created — for plain/no-worktree sessions
		// WorktreePath is the user's real project folder, and deleting it
		// here would wipe their repo.
		_ = os.RemoveAll(s.WorktreePath)
	}
	return a.Store.Delete(id)
}
