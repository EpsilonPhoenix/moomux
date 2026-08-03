package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetNeedsInputThenClear(t *testing.T) {
	home := t.TempDir()

	if err := SetNeedsInput(home, "sess1", "/wt/a"); err != nil {
		t.Fatalf("SetNeedsInput: %v", err)
	}
	path := markerPath(home, "sess1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if got := string(data); got == "" {
		t.Fatal("marker file is empty")
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marker: %v", err)
	}
	if m.CWD != "/wt/a" || m.Status != "needs-input" {
		t.Fatalf("unexpected marker contents: %+v", m)
	}

	if err := Clear(home, "sess1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected marker to be removed, stat err = %v", err)
	}
}

func TestClearMissingMarkerIsNotAnError(t *testing.T) {
	home := t.TempDir()
	if err := Clear(home, "nope"); err != nil {
		t.Fatalf("Clear on missing marker: %v", err)
	}
}

func TestSessionsDir(t *testing.T) {
	got := SessionsDir("/home/alan")
	want := filepath.Join("/home/alan", ".claude", "sessions")
	if got != want {
		t.Fatalf("SessionsDir = %q, want %q", got, want)
	}
}
