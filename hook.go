package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/erickgnclvs/moomux/internal/claudehook"
	"github.com/erickgnclvs/moomux/internal/codexhook"
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
	case "codex":
		return handleCodexHook(action, os.Stdin, home)
	default:
		return fmt.Errorf("unsupported agent %q", agent)
	}
}

func readHookPayload(stdin io.Reader) (hookPayload, error) {
	var p hookPayload
	data, err := io.ReadAll(stdin)
	if err != nil {
		return p, fmt.Errorf("read hook payload: %w", err)
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("parse hook payload: %w", err)
	}
	return p, nil
}

func handleClaudeHook(action string, stdin io.Reader, home string) error {
	p, err := readHookPayload(stdin)
	if err != nil {
		return err
	}
	if p.SessionID == "" {
		return fmt.Errorf("hook payload missing session_id")
	}
	if action == "set" {
		return claudehook.SetNeedsInput(home, p.SessionID, p.CWD)
	}
	return claudehook.Clear(home, p.SessionID)
}

// handleCodexHook mirrors handleClaudeHook, but Codex's marker is keyed by
// cwd (see codexhook.markerPath) rather than a session id, so only cwd is
// required here.
func handleCodexHook(action string, stdin io.Reader, home string) error {
	p, err := readHookPayload(stdin)
	if err != nil {
		return err
	}
	if p.CWD == "" {
		return fmt.Errorf("hook payload missing cwd")
	}
	if action == "set" {
		return codexhook.SetNeedsInput(home, p.CWD)
	}
	return codexhook.Clear(home, p.CWD)
}
