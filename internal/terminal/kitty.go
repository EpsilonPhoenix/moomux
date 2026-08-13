package terminal

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
)

// kittyRunner runs a `kitten @ ...` remote-control call and returns its
// stdout, mirroring scriptRunner's shape in iterm.go.
type kittyRunner interface {
	Run(args ...string) (string, error)
}

type execKittyRunner struct{}

func (execKittyRunner) Run(args ...string) (string, error) {
	out, err := exec.Command("kitten", args...).CombinedOutput()
	return string(out), err
}

// kittyClient implements TabReopener for kitty over its remote-control
// socket. Detect() only returns this when KITTY_LISTEN_ON is set; without a
// socket, `kitten @` falls back to tty-based control and fallback is used
// instead.
type kittyClient struct {
	runner   kittyRunner
	fallback TerminalOpener
}

func newKittyClient(fallback TerminalOpener) *kittyClient {
	return &kittyClient{runner: execKittyRunner{}, fallback: fallback}
}

func (c *kittyClient) OpenSession(tmuxSession, title string) (string, error) {
	_, hint, err := c.OpenTab("", tmuxSession, title)
	return hint, err
}

// OpenTab focuses tabID's tab if kitty still has it, otherwise opens a
// fresh tab (attaching tmuxSession) and returns the id of the tab kitty
// created.
func (c *kittyClient) OpenTab(tabID, tmuxSession, title string) (string, string, error) {
	if tabID != "" {
		if c.focusTab(tabID) {
			return tabID, "", nil
		}
		slog.Debug("kitty: tab gone, opening a new one", "tab_id", tabID)
	}
	return c.createTab(tmuxSession, title)
}

// focusTab brings tabID's tab to the front. kitty exits non-zero both when
// the tab is gone and on a genuine remote-control failure; either way the
// caller's best move is the same — open a fresh tab — so both cases are
// treated as "not found" here rather than needing to be told apart.
func (c *kittyClient) focusTab(tabID string) bool {
	_, err := c.runner.Run("@", "focus-tab", "--match", "id:"+tabID)
	return err == nil
}

// createTab opens tmuxSession in a new kitty tab and returns the id of the
// tab kitty created. kitty focuses a freshly launched tab by default, so
// that id is just whichever tab `kitten @ ls` reports as focused right
// after the launch call succeeds — no need to parse launch's own output.
func (c *kittyClient) createTab(tmuxSession, title string) (string, string, error) {
	if _, err := c.runner.Run(kittyTabArgs(title, "="+tmuxSession)...); err != nil {
		slog.Debug("kitty: launch failed, falling back", "err", err)
		hint, ferr := c.fallback.OpenSession(tmuxSession, title)
		return "", hint, ferr
	}
	tabID, err := c.activeTabID()
	if err != nil {
		slog.Debug("kitty: could not determine new tab id", "err", err)
		return "", "", nil
	}
	return tabID, "", nil
}

// activeTabID returns the id of kitty's currently focused tab, parsed from
// `kitten @ ls`'s JSON (a list of OS windows, each with a list of tabs).
func (c *kittyClient) activeTabID() (string, error) {
	out, err := c.runner.Run("@", "ls")
	if err != nil {
		return "", err
	}
	var osWindows []struct {
		Tabs []struct {
			ID        int  `json:"id"`
			IsFocused bool `json:"is_focused"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal([]byte(out), &osWindows); err != nil {
		return "", err
	}
	for _, w := range osWindows {
		for _, t := range w.Tabs {
			if t.IsFocused {
				return strconv.Itoa(t.ID), nil
			}
		}
	}
	return "", fmt.Errorf("kitty: no focused tab in ls output")
}
