// Package config loads and writes moomux's TOML configuration.
package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Project struct {
	Kind         string `toml:"kind,omitempty"` // "git" (default) or "plain"
	Repo         string `toml:"repo"`
	BranchPrefix string `toml:"branch_prefix,omitempty"`
	BaseBranch   string `toml:"base_branch,omitempty"`
	Agent        string `toml:"agent,omitempty"` // "claude" (default), "codex", "opencode"
	// PromptAgent, when true, skips preselecting Agent as the default in the
	// new-session form — the user must explicitly pick an agent before the
	// form can be submitted.
	PromptAgent bool `toml:"prompt_agent,omitempty"`
	// NoWorktree, when true on a "git" project, keeps every session in Repo
	// itself instead of giving each one its own worktree/branch — the same
	// single-folder behavior as a "plain" project, but for a real git repo.
	NoWorktree bool `toml:"no_worktree,omitempty"`
}

func (p Project) IsPlain() bool { return p.Kind == "plain" }

// UsesWorktree reports whether sessions for this project should each get
// their own git worktree. False for plain projects and for git projects
// that opted out via NoWorktree — both cases run every session directly in
// p.Repo.
func (p Project) UsesWorktree() bool { return !p.IsPlain() && !p.NoWorktree }

// OrderedProjectNames returns configured project names in the user's manual
// order (c.Order), followed by any names not yet in that order (new
// projects, or configs from before manual ordering existed) sorted
// alphabetically.
func (c *Config) OrderedProjectNames() []string {
	seen := make(map[string]bool, len(c.Projects))
	out := make([]string, 0, len(c.Projects))
	for _, name := range c.Order {
		if _, ok := c.Projects[name]; ok && !seen[name] {
			out = append(out, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(c.Projects)-len(out))
	for name := range c.Projects {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// AgentName returns the effective agent name, defaulting to "claude".
func (p Project) AgentName() string {
	if p.Agent == "" {
		return "claude"
	}
	return p.Agent
}

type Config struct {
	Projects map[string]Project `toml:"projects"`
	// Order is the user's manual project ordering (front-to-back). Names not
	// listed here (new projects, or configs written before this existed)
	// sort alphabetically after the ordered ones.
	Order []string `toml:"order,omitempty"`
	// TmuxSetupAsked marks that the user has already been asked whether to
	// add moomux's recommended ~/.tmux.conf settings, so the prompt only
	// ever runs once regardless of their answer.
	TmuxSetupAsked bool `toml:"tmux_setup_asked,omitempty"`
	// AutoTmux, when true, relaunches moomux inside a dedicated tmux
	// session ("moomux") on startup if it isn't already running inside one.
	AutoTmux bool `toml:"auto_tmux,omitempty"`
	// AutoTmuxAsked marks that the user has already been asked whether to
	// enable AutoTmux, so the prompt only ever runs once.
	AutoTmuxAsked bool `toml:"auto_tmux_asked,omitempty"`
}

func Load(path string) (*Config, error) {
	cfg := &Config{Projects: map[string]Project{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Projects == nil {
		cfg.Projects = map[string]Project{}
	}
	for k, p := range cfg.Projects {
		p.Repo = ExpandHome(p.Repo)
		cfg.Projects[k] = p
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Write-then-rename (same as the session store): encoding straight into
	// the live file truncates it first, so a crash or full disk mid-encode
	// destroys every project definition.
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "moomux", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "moomux", "config.toml")
}

// ExpandHome expands a leading "~" to the user's home directory. Load
// already applies this to every stored Repo; callers validating freshly
// typed input before it's ever saved (e.g. app.validateProject) need it too.
func ExpandHome(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}
