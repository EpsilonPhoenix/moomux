// Package userscript runs user-provided scripts at points in a session's
// lifecycle (currently: right after a worktree is created).
package userscript

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

// runTimeout bounds each script so a hung one can't stall session creation.
var runTimeout = 30 * time.Second

// Dir returns the directory moomux scans for worktree-create scripts. It
// lives under the user's config dir, outside any repo moomux manages, so
// scripts placed here are never committed or pushed by the project itself.
func Dir(home string) string {
	return filepath.Join(home, ".config", "moomux", "userscripts", "worktree-create")
}

// Env carries the values passed to every script as environment variables.
type Env struct {
	Project  string
	Worktree string
	Repo     string
	Branch   string
}

// RunWorktreeCreate runs every executable file directly inside Dir(home), in
// name-sorted order, with cwd set to env.Worktree. Best-effort: a failing or
// timed-out script is reported in the returned warnings rather than as an
// error — one broken script shouldn't stop session creation.
func RunWorktreeCreate(home string, env Env) []string {
	dir := Dir(home)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // no userscripts dir: nothing to do
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	extraEnv := []string{
		"MOOMUX_PROJECT=" + env.Project,
		"MOOMUX_WORKTREE=" + env.Worktree,
		"MOOMUX_REPO=" + env.Repo,
		"MOOMUX_BRANCH=" + env.Branch,
	}

	var warnings []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Mode()&0o111 == 0 {
			continue // not executable
		}
		path := filepath.Join(dir, e.Name())
		ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
		cmd := exec.CommandContext(ctx, path)
		cmd.Dir = env.Worktree
		cmd.Env = append(os.Environ(), extraEnv...)
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("userscript %s: %v (%s)", e.Name(), err, string(out)))
		}
	}
	return warnings
}
