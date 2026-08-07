package terminal

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

type scriptRunner interface {
	Run(script string) (string, error)
}

type execScriptRunner struct{}

func (execScriptRunner) Run(script string) (string, error) {
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	return string(out), err
}

type itermClient struct {
	runner scriptRunner
}

func newITermClient() *itermClient {
	return &itermClient{runner: execScriptRunner{}}
}

func (c *itermClient) OpenSession(tmuxSession, title string) (string, error) {
	_, hint, err := c.OpenTab("", tmuxSession, title)
	return hint, err
}

// OpenTab brings tabID's tab to the front if it still exists, otherwise
// opens a fresh tab (attaching tmuxSession) and returns its id.
//
// iTerm2's AppleScript "tab" class has no id property (`id of current tab`
// errors with -1728, "can't get that property"), so tabID is actually the
// id of the tab's session — a stable per-session UUID iTerm2 does expose —
// and selecting that session also brings its tab and window to front.
func (c *itermClient) OpenTab(tabID, tmuxSession, title string) (string, string, error) {
	if tabID != "" {
		found, out, err := c.selectSession(tabID)
		if err != nil {
			return tabID, "", err
		}
		if found {
			return tabID, "", nil
		}
		slog.Debug("iterm: tab gone, opening a new one", "tab_id", tabID, "out", out)
	}
	newTabID, err := c.createTab(tmuxSession, title)
	return newTabID, "", err
}

// selectSession brings an existing tab to the front by the id of the
// session it holds, across all windows. Returns false (not an error) if no
// such session exists — tabs close independently of the tmux session they
// were attached to.
func (c *itermClient) selectSession(sessionID string) (bool, string, error) {
	script := fmt.Sprintf(`
tell application "iTerm2"
	activate
	repeat with w in windows
		repeat with t in tabs of w
			repeat with sess in sessions of t
				if id of sess is "%s" then
					select t
					select w
					return "found"
				end if
			end repeat
		end repeat
	end repeat
	return "notfound"
end tell`, escapeAppleScript(sessionID))
	out, err := c.runner.Run(script)
	slog.Debug("iterm: select session result", "session_id", sessionID, "out", out, "err", err)
	return strings.TrimSpace(out) == "found", out, err
}

// createTab opens a new iTerm2 tab, attaches tmuxSession in it, and returns
// the id of the session it holds (see OpenTab's doc comment for why).
func (c *itermClient) createTab(tmuxSession, title string) (string, error) {
	setName := ""
	if title != "" {
		escaped := escapeAppleScript(title)
		setName = fmt.Sprintf("\n\t\t\tset name to \"%s\"", escaped)
	}
	// write text runs the line through the tab's interactive shell, so the
	// "=" exact-match target has to be single-quoted — zsh's EQUALS
	// expansion would read a bare "=name" as a command-path lookup.
	script := fmt.Sprintf(`
tell application "iTerm2"
	activate
	if (count of windows) = 0 then
		create window with default profile
	end if
	tell current window
		set newTab to (create tab with default profile)
		tell current session of newTab%s
			write text "tmux attach -t '%s'"
			return id
		end tell
	end tell
end tell`, setName, escapeAppleScript("="+tmuxSession))
	slog.Debug("iterm: running applescript", "tmux_session", tmuxSession, "title", title, "set_name", setName != "", "script", script)
	out, err := c.runner.Run(script)
	slog.Debug("iterm: applescript result", "out", out, "err", err)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func escapeAppleScript(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\\' || r == '"' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}
