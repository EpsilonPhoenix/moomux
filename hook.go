package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/erickgnclvs/moomux/internal/claudehook"
)

type hookPayload struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
}

// runHook implements `moomux hook <agent> set|clear`, invoked by the hooks
// each agent's package (internal/claudehook today; internal/codexhook etc.
// later) installs into a worktree's config. The agent segment lets each
// backend define its own payload shape and marker mechanism without the
// others — or main.go's dispatch — needing to change.
func runHook(args []string) error {
	if len(args) != 2 || (args[1] != "set" && args[1] != "clear") {
		return fmt.Errorf("usage: moomux hook <agent> set|clear")
	}
	agent, action := args[0], args[1]
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	switch agent {
	case "claude":
		return handleClaudeHook(action, os.Stdin, home)
	default:
		return fmt.Errorf("unsupported agent %q", agent)
	}
}

func handleClaudeHook(action string, stdin io.Reader, home string) error {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return fmt.Errorf("read hook payload: %w", err)
	}
	var p hookPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("parse hook payload: %w", err)
	}
	if p.SessionID == "" {
		return fmt.Errorf("hook payload missing session_id")
	}
	if action == "set" {
		return claudehook.SetNeedsInput(home, p.SessionID, p.CWD)
	}
	return claudehook.Clear(home, p.SessionID)
}
