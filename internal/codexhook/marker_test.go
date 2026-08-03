package codexhook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetNeedsInputThenClear(t *testing.T) {
	home := t.TempDir()

	if err := SetNeedsInput(home, "/wt/a"); err != nil {
		t.Fatalf("SetNeedsInput: %v", err)
	}
	path := markerPath(home, "/wt/a")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	var m marker
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal marker: %v", err)
	}
	if m.CWD != "/wt/a" || m.Status != "needs-input" {
		t.Fatalf("unexpected marker contents: %+v", m)
	}

	if err := Clear(home, "/wt/a"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected marker to be removed, stat err = %v", err)
	}
}

func TestClearMissingMarkerIsNotAnError(t *testing.T) {
	home := t.TempDir()
	if err := Clear(home, "/wt/nope"); err != nil {
		t.Fatalf("Clear on missing marker: %v", err)
	}
}

func TestMarkerDir(t *testing.T) {
	got := MarkerDir("/home/alan")
	want := filepath.Join("/home/alan", ".codex", "moomux-notify")
	if got != want {
		t.Fatalf("MarkerDir = %q, want %q", got, want)
	}
}
