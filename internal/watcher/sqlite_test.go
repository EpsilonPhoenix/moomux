package watcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestQuerySQLiteRespectsContextCancellation replaces "sqlite3" on PATH with
// a fake that sleeps far longer than the query context's timeout. Without
// exec.CommandContext, canceling ctx doesn't touch the already-started
// subprocess and querySQLite blocks for the full sleep; with it, the
// subprocess is killed and querySQLite returns as soon as the context
// expires.
func TestQuerySQLiteRespectsContextCancellation(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "sqlite3")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := querySQLite(ctx, "irrelevant.db", "SELECT 1"); err == nil {
		t.Fatal("expected error once context expired")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("querySQLite took %v, want to return shortly after the context timeout", elapsed)
	}
}
