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
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("querySQLite took %v, want to return shortly after the context timeout", elapsed)
	}
}

// TestSQLiteWatcherMarkerDirWinsOverStaleRow replaces "sqlite3" on PATH with
// a fake that reports a stale (long-idle) row for /tmp/wt-a, which alone
// would classify as Waiting. A codexhook marker for the same cwd must still
// win in the same tick — mirroring DirWatcher's Claude-side guarantee that a
// stale native status write never hides a needs-input marker.
func TestSQLiteWatcherMarkerDirWinsOverStaleRow(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "sqlite3")
	script := "#!/bin/sh\necho \"/tmp/wt-a\t1\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "state.sqlite")
	if err := os.WriteFile(dbPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	markerDir := t.TempDir()
	writeJSON(t, filepath.Join(markerDir, "sess.json"), map[string]any{
		"cwd":    "/tmp/wt-a",
		"status": "needs-input",
	})

	w := &SQLiteWatcher{DB: dbPath, Query: "irrelevant", MarkerDir: markerDir, Interval: 10 * time.Millisecond}
	ch := make(chan Snapshot, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go w.Run(ctx, ch)

	select {
	case snap := <-ch:
		if got := snap.States["/tmp/wt-a"]; got != NeedsInput {
			t.Fatalf("got %v, want NeedsInput", got)
		}
	case <-ctx.Done():
		t.Fatal("no snapshot received")
	}
}

// TestSQLiteWatcherMarkerDirWithoutDB covers a needs-input hook firing
// before Codex's own state db exists yet (e.g. very first launch): the DB
// glob matching zero files must not skip the MarkerDir scan.
func TestSQLiteWatcherMarkerDirWithoutDB(t *testing.T) {
	markerDir := t.TempDir()
	writeJSON(t, filepath.Join(markerDir, "sess.json"), map[string]any{
		"cwd":    "/tmp/wt-a",
		"status": "needs-input",
	})

	w := &SQLiteWatcher{
		DB:        filepath.Join(t.TempDir(), "nonexistent-*.sqlite"),
		Query:     "irrelevant",
		MarkerDir: markerDir,
		Interval:  10 * time.Millisecond,
	}
	ch := make(chan Snapshot, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go w.Run(ctx, ch)

	select {
	case snap := <-ch:
		if got := snap.States["/tmp/wt-a"]; got != NeedsInput {
			t.Fatalf("got %v, want NeedsInput", got)
		}
	case <-ctx.Done():
		t.Fatal("no snapshot received")
	}
}
