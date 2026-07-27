// Package browser opens URLs in the user's default browser.
package browser

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/aymanbagabas/go-osc52/v2"
)

func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// Remote reports whether the process is likely running over an SSH
// connection, where Open would launch a browser on the wrong (remote)
// machine instead of the user's own. Other transports (e.g. mosh, which
// doesn't route through sshd) set none of these and can't be detected this
// way — the TUI's R key lets a user override the auto-detection for those.
func Remote() bool {
	return os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != ""
}

// Copy writes text to the user's local clipboard via an OSC 52 escape
// sequence. Unlike a click on a link, this isn't affected by the app's
// mouse-tracking mode — it's just data the terminal reads off the pty and
// forwards to the real clipboard, even over SSH/tmux.
func Copy(text string) error {
	_, err := osc52.New(text).WriteTo(os.Stdout)
	return err
}
