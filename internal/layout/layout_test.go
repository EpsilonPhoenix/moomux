package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	windows, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if windows != nil {
		t.Fatalf("windows = %+v, want nil", windows)
	}
}

func TestLoadValidNestedGrid(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `
[[windows]]
name = "main"
direction = "row"

[[windows.children]]
size = "60%"
agent = true

[[windows.children]]
direction = "col"
size = "40%"

  [[windows.children.children]]
  cmd = "npm run dev"

  [[windows.children.children]]
  cmd = "tail -f logs/dev.log"
`)
	windows, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("windows = %+v, want 1", windows)
	}
	root := windows[0]
	if root.Name != "main" || root.Direction != "row" || len(root.Children) != 2 {
		t.Fatalf("windows[0] = %+v", root)
	}
	if !root.Children[0].Agent {
		t.Fatalf("children[0] = %+v, want agent leaf", root.Children[0])
	}
	if got := len(root.Children[1].Children); got != 2 {
		t.Fatalf("nested children = %d, want 2", got)
	}
}

func TestLoadMultipleWindows(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `
[[windows]]
agent = true

[[windows]]
name = "logs"
direction = "col"

[[windows.children]]
cmd = "tail -f logs/dev.log"

[[windows.children]]
cmd = "docker compose logs -f"
`)
	windows, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(windows) != 2 {
		t.Fatalf("windows = %+v, want 2", windows)
	}
	if !windows[0].Agent {
		t.Fatalf("windows[0] = %+v, want agent leaf", windows[0])
	}
	if windows[1].Name != "logs" || len(windows[1].Children) != 2 {
		t.Fatalf("windows[1] = %+v", windows[1])
	}
}

func TestLoadRejectsNoWindows(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `theme = "unrelated"`)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for missing [[windows]]")
	}
}

func TestLoadRejectsNoAgent(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `
[[windows]]
direction = "row"

[[windows.children]]
cmd = "npm run dev"

[[windows.children]]
cmd = "tail -f log"
`)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for missing agent leaf")
	}
}

func TestLoadRejectsAgentSpreadAcrossWindows(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `
[[windows]]
agent = true

[[windows]]
agent = true
`)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for two agent leaves across windows")
	}
}

func TestLoadRejectsSingleChildSplit(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `
[[windows]]
direction = "row"

[[windows.children]]
agent = true
`)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for split node with <2 children")
	}
}

func TestLoadRejectsBadDirection(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, `
[[windows]]
direction = "sideways"

[[windows.children]]
agent = true

[[windows.children]]
cmd = "x"
`)
	if _, err := Load(dir); err == nil {
		t.Fatal("want error for invalid direction")
	}
}

func TestParsePercent(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want float64
	}{{"60%", 60}, {"60", 60}, {" 33% ", 33}} {
		got, err := ParsePercent(tc.in)
		if err != nil {
			t.Fatalf("ParsePercent(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParsePercent(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if _, err := ParsePercent("nope"); err == nil {
		t.Fatal("want error for non-numeric size")
	}
}
