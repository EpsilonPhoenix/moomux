package terminal

import (
	"log/slog"
	"os/exec"
	"strings"
)

// weztermClient implements TabReopener on top of the wezterm mux server's
// `wezterm cli` (see Detect's WEZTERM_PANE case, which only picks this path
// when the mux server is reachable). It's a self-contained type rather than
// an addition to remoteOpener so that other remoteOpener users (kitty,
// alacritty) don't falsely satisfy TabReopener — alacritty in particular has
// no tab-by-id concept.
type weztermClient struct {
	run      func(args ...string) (string, error)
	fallback TerminalOpener
}

func newWeztermClient(fallback TerminalOpener) *weztermClient {
	return &weztermClient{run: weztermRun, fallback: fallback}
}

func weztermRun(args ...string) (string, error) {
	out, err := exec.Command("wezterm", args...).CombinedOutput()
	return string(out), err
}

func (c *weztermClient) OpenSession(tmuxSession, title string) (string, error) {
	_, hint, err := c.OpenTab("", tmuxSession, title)
	return hint, err
}

// OpenTab brings tabID's pane to the front if it still exists, otherwise
// spawns a fresh pane (attaching tmuxSession) and returns its id.
//
// tabID is the id of the wezterm pane that `cli spawn` created for the
// session — wezterm has no separate tab-id concept exposed over the CLI, and
// a pane is 1:1 with the tab it fills for a `cli spawn`-created tab.
func (c *weztermClient) OpenTab(tabID, tmuxSession, title string) (string, string, error) {
	if tabID != "" {
		if _, err := c.run("cli", "activate-pane", "--pane-id", tabID); err == nil {
			return tabID, "", nil
		}
		slog.Debug("wezterm: pane gone, opening a new one", "tab_id", tabID)
	}
	// "=" pins tmux's -t to an exact session-name match; a bare name falls
	// back to prefix matching and can attach to the wrong session.
	out, err := c.run("cli", "spawn", "--", "tmux", "attach", "-t", "="+tmuxSession)
	if err != nil {
		slog.Debug("wezterm: spawn failed, falling back", "err", err)
		hint, ferr := c.fallback.OpenSession(tmuxSession, title)
		return "", hint, ferr
	}
	return strings.TrimSpace(out), "", nil
}
