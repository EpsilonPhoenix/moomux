package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/erickgnclvs/moomux/internal/config"
)

// saveConfig must log a Save failure rather than silently discard it —
// otherwise cfg.TmuxSetupAsked/AutoTmuxAsked being lost would reopen the
// first-run prompts on every subsequent launch with no indication why.
func TestSaveConfigLogsFailure(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	saveConfig(filepath.Join(blocker, "config.toml"), &config.Config{})

	if !bytes.Contains(buf.Bytes(), []byte("config save failed")) {
		t.Fatalf("expected a logged failure, got: %s", buf.String())
	}
}
