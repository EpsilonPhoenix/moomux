package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/erickgnclvs/moomux/internal/app"
	"github.com/erickgnclvs/moomux/internal/config"
	"github.com/erickgnclvs/moomux/internal/gitwt"
	"github.com/erickgnclvs/moomux/internal/session"
	"github.com/erickgnclvs/moomux/internal/terminal"
	"github.com/erickgnclvs/moomux/internal/tmux"
	"github.com/erickgnclvs/moomux/internal/tmuxconf"
	"github.com/erickgnclvs/moomux/internal/tui"
	"github.com/erickgnclvs/moomux/internal/watcher"
)

// set by goreleaser via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("moomux %s (%s) built %s\n", version, commit, date)
		return
	}
	if err := checkDeps(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) >= 2 && os.Args[1] == "spawn" {
		if err := runSpawn(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "moomux:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "moomux:", err)
		os.Exit(1)
	}
}

// checkDeps verifies the external binaries moomux shells out to are on
// $PATH. `make install`/`make run` check tmux/git via check-deps, but
// Homebrew and `go install` users bypass the Makefile entirely, so we check
// again here and fail with an actionable message instead of a raw exec error
// the first time a tmux/git call happens.
func checkDeps() error {
	var missing []string
	for _, bin := range []string{"tmux", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"moomux: missing required dependencies: %s\n\nInstall with:\n  macOS:  brew install %s\n  Ubuntu: sudo apt install %s\n  Fedora: sudo dnf install %s",
		strings.Join(missing, ", "), strings.Join(missing, " "), strings.Join(missing, " "), strings.Join(missing, " "),
	)
}

// promptTmuxSetup offers, on first run only, to append moomux's recommended
// settings (mouse support, passthrough, scrollback, 1-indexed panes — see
// README.md) to ~/.tmux.conf. It always marks cfg.TmuxSetupAsked so this
// never runs again regardless of the answer, and saves that immediately so a
// crash or Ctrl-C right after can't reopen the prompt on every launch.
// Skipped entirely on a non-interactive stdin (e.g. CI, piped input), which
// is treated the same as declining.
func promptTmuxSetup(cfg *config.Config, cfgPath string) {
	path := tmuxconf.Path()
	cfg.TmuxSetupAsked = true

	if tmuxconf.AlreadyApplied(path) {
		_ = config.Save(cfgPath, cfg)
		return
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		_ = config.Save(cfgPath, cfg)
		return
	}

	fmt.Printf("moomux launches plain tmux sessions, so mouse support, clickable/scrollable\n")
	fmt.Printf("panes, and a larger scrollback come from your own tmux config. Add this to\n%s?\n", path)
	fmt.Print(tmuxconf.Snippet)
	fmt.Print("Add it now? [y/N] ")

	answer := ""
	if line, err := bufio.NewReader(os.Stdin).ReadString('\n'); err == nil {
		answer = strings.ToLower(strings.TrimSpace(line))
	}

	if answer == "y" || answer == "yes" {
		if err := tmuxconf.Apply(path); err != nil {
			fmt.Fprintln(os.Stderr, "moomux: could not update", path+":", err)
		} else {
			fmt.Println("Added — reload an open tmux session with: tmux source-file", path)
		}
	} else {
		fmt.Println("Skipped. Add it later from README.md's \"Recommended tmux config\" section.")
	}
	fmt.Println()

	_ = config.Save(cfgPath, cfg)
}

// promptAutoTmux offers, on first run only, to always relaunch moomux
// inside a dedicated tmux session ("moomux") from now on. Like
// promptTmuxSetup, it always marks cfg.AutoTmuxAsked and saves immediately
// so the prompt never reappears, and is skipped on a non-interactive stdin.
func promptAutoTmux(cfg *config.Config, cfgPath string) {
	cfg.AutoTmuxAsked = true

	if !isatty.IsTerminal(os.Stdin.Fd()) {
		_ = config.Save(cfgPath, cfg)
		return
	}

	fmt.Println("moomux can always start inside its own tmux session (attaching to one named")
	fmt.Println("\"moomux\" if it already exists), so it survives closing your terminal.")
	fmt.Print("Always start moomux inside tmux? [y/N] ")

	answer := ""
	if line, err := bufio.NewReader(os.Stdin).ReadString('\n'); err == nil {
		answer = strings.ToLower(strings.TrimSpace(line))
	}

	if answer == "y" || answer == "yes" {
		cfg.AutoTmux = true
		fmt.Println("Enabled. Change any time by editing auto_tmux in", cfgPath)
	} else {
		fmt.Println("Skipped. Enable later by setting auto_tmux = true in", cfgPath)
	}
	fmt.Println()

	_ = config.Save(cfgPath, cfg)
}

// relaunchInTmux replaces the current process with `tmux new-session -A -s
// moomux <self>`, attaching to an existing "moomux" session or creating one.
// checkDeps has already verified tmux is on $PATH by the time this runs.
func relaunchInTmux() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	return syscall.Exec(tmuxBin, []string{"tmux", "new-session", "-A", "-s", "moomux", self}, os.Environ())
}

// newApp loads config/session state and wires up an App, with logging
// pointed at moomux.log. Shared by the TUI (run) and the non-interactive
// spawn subcommand — neither the tmux-setup/auto-tmux first-run prompts nor
// relaunchInTmux belong in the non-interactive path, so those stay in run().
func newApp() (*app.App, error) {
	cfgPath := config.DefaultPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", cfgPath, err)
	}

	store := &session.Store{Path: session.DefaultPath()}
	if err := store.Load(); err != nil {
		return nil, fmt.Errorf("load sessions: %w", err)
	}

	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".local", "share", "moomux")
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "moomux.log")
	if lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	}

	return &app.App{
		Cfg:          cfg,
		CfgPath:      cfgPath,
		Store:        store,
		Tmux:         tmux.New(),
		Terminal:     terminal.Detect(),
		Git:          gitwt.New(),
		WorktreeRoot: app.WorktreeRootDefault(),
	}, nil
}

// runSpawn implements `moomux spawn`: create a session (worktree + tmux +
// agent, same as the TUI's "new session" action) and, if -prompt is given,
// type it into the agent's pane as its first task. Fire-and-forget — it
// prints the new tmux session name and exits without waiting on the agent.
func runSpawn(args []string) error {
	fs := flag.NewFlagSet("spawn", flag.ExitOnError)
	project := fs.String("project", "", "project name (required)")
	name := fs.String("name", "", "session name (derived from -branch if omitted)")
	agent := fs.String("agent", "", "agent override (claude, codex, opencode)")
	branch := fs.String("branch", "", "existing branch to check out, instead of creating a new one")
	ticket := fs.String("ticket", "", "ticket URL to attach to the session")
	prompt := fs.String("prompt", "", "initial prompt to type into the new session's agent pane")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *project == "" {
		return fmt.Errorf("spawn: -project is required")
	}

	a, err := newApp()
	if err != nil {
		return err
	}

	s, hint, err := a.CreateSession(*project, *name, *agent, *branch, *ticket)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	if hint != "" {
		fmt.Println(hint)
	}
	fmt.Println(s.TmuxSession)

	if *prompt != "" {
		// ponytail: fixed delay, not a readiness poll — good enough for a
		// fire-and-forget v1. Upgrade to polling pane content for a ready
		// marker if agent startup time ever outgrows this.
		time.Sleep(2 * time.Second)
		if err := a.SendPrompt(s.TmuxSession, *prompt); err != nil {
			return fmt.Errorf("send prompt: %w", err)
		}
	}
	return nil
}

func run() error {
	a, err := newApp()
	if err != nil {
		return err
	}
	cfg, cfgPath := a.Cfg, a.CfgPath

	if !cfg.TmuxSetupAsked {
		promptTmuxSetup(cfg, cfgPath)
	}
	if !cfg.AutoTmuxAsked {
		promptAutoTmux(cfg, cfgPath)
	}
	if cfg.AutoTmux && os.Getenv("TMUX") == "" {
		if err := relaunchInTmux(); err != nil {
			fmt.Fprintln(os.Stderr, "moomux: could not start inside tmux:", err)
		}
	}

	home, _ := os.UserHomeDir()
	ctx, cancel := context.WithCancel(context.Background())
	statusCh := make(chan watcher.Snapshot, 4)
	multi := buildWatcher(home)
	go multi.Run(ctx, statusCh)

	m := tui.New(cfg, a, statusCh, cancel)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		cancel()
		return err
	}
	cancel()
	return nil
}

func buildWatcher(home string) watcher.Watcher {
	return &watcher.MultiWatcher{Watchers: []watcher.Watcher{
		// Claude Code: JSON session files in ~/.claude/sessions/
		&watcher.DirWatcher{Dir: filepath.Join(home, ".claude", "sessions")},
		// Codex: activity tracked in SQLite DB (~/.codex/state_N.sqlite)
		&watcher.SQLiteWatcher{
			DB:    filepath.Join(home, ".codex", "state_*.sqlite"),
			Query: "SELECT cwd, MAX(updated_at_ms) FROM threads GROUP BY cwd",
		},
		// OpenCode: activity tracked in SQLite DB (~/.local/share/opencode/opencode.db)
		&watcher.SQLiteWatcher{
			DB:    filepath.Join(home, ".local", "share", "opencode", "opencode.db"),
			Query: "SELECT directory, MAX(time_updated) FROM session GROUP BY directory",
		},
	}}
}
