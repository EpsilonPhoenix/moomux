// Package browser opens URLs in the user's default browser.
package browser

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"

	"github.com/aymanbagabas/go-osc52/v2"
)

// Open launches rawURL in the user's default browser. rawURL is rejected
// unless it parses as an absolute http(s) URL: it may come from link text
// rendered by an agent session, and passing an arbitrary string straight to
// exec.Command would let a crafted value (e.g. one starting with "-") be
// read as a flag by "open"/"xdg-open"/rundll32, or open non-http schemes
// the platform opener treats specially (e.g. "file://").
func Open(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("refusing to open non-http(s) URL: %q", rawURL)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the child once it exits; without this every opened link leaves
	// a zombie for the lifetime of the long-running TUI process.
	go func() { _ = cmd.Wait() }()
	return nil
}

// Remote reports whether the process is likely running over an SSH
// connection, where Open would launch a browser on the wrong (remote)
// machine instead of the user's own. Other transports (e.g. mosh, which
// doesn't route through sshd) set none of these and can't be detected this
// way — the TUI's R key lets a user override the auto-detection for those.
//
// Only SSH_TTY is checked. SSH_CONNECTION and SSH_CLIENT are set by sshd but,
// on macOS, get cached into the per-user launchd environment on first SSH
// login and then persist for every process (including local Terminal/iTerm
// windows) until logout — so checking them false-positives as "remote" long
// after the SSH session ended. SSH_TTY is tied to the actual pty and isn't
// cached that way.
func Remote() bool {
	return os.Getenv("SSH_TTY") != ""
}

// Copy writes text to the user's local clipboard via an OSC 52 escape
// sequence. Unlike a click on a link, this isn't affected by the app's
// mouse-tracking mode — it's just data the terminal reads off the pty and
// forwards to the real clipboard, even over SSH/tmux.
func Copy(text string) error {
	_, err := osc52.New(text).WriteTo(os.Stdout)
	return err
}
