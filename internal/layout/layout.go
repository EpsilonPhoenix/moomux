// Package layout loads an optional, worktree-local pane arrangement so a
// project can define its own tmux window layout (rows/columns of panes,
// with the agent placed anywhere in it) instead of moomux's default
// two-pane split.
package layout

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// FileName is the optional layout file moomux looks for at the root of a
// session's worktree.
const FileName = ".moomux-panes.toml"

// PaneSpec is one node of a pane layout tree. A node with no Children is a
// leaf: either the agent pane (Agent: true) or a plain pane optionally
// running Cmd. A node with Children divides its space among them along
// Direction ("row": side-by-side columns, "col": stacked rows); Size on a
// child is its share of the parent's space (e.g. "60%"), defaulting to an
// even split among siblings that omit it.
type PaneSpec struct {
	Direction string     `toml:"direction,omitempty"`
	Size      string     `toml:"size,omitempty"`
	Agent     bool       `toml:"agent,omitempty"`
	Cmd       string     `toml:"cmd,omitempty"`
	Children  []PaneSpec `toml:"children,omitempty"`
}

// WindowSpec is one tmux window: an optional Name (tmux auto-names it when
// empty) plus that window's own pane tree. Whichever window contains the
// Agent: true leaf is given focus when the session is created, and always
// takes moomux's own session display name instead of Name — both apply
// regardless of that window's position in the file, it doesn't have to be
// listed first or last.
type WindowSpec struct {
	Name string `toml:"name,omitempty"`
	PaneSpec
}

// Load reads and validates the layout file at the root of worktreePath.
// Returns (nil, nil) if no such file exists — callers fall back to the
// default layout.
func Load(worktreePath string) ([]WindowSpec, error) {
	data, err := os.ReadFile(filepath.Join(worktreePath, FileName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var file struct {
		Windows []WindowSpec `toml:"windows"`
	}
	if err := toml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	if len(file.Windows) == 0 {
		return nil, fmt.Errorf("%s: no [[windows]] defined", FileName)
	}
	agents := 0
	for i := range file.Windows {
		if err := validateShape(&file.Windows[i].PaneSpec); err != nil {
			return nil, fmt.Errorf("%s: window %q: %w", FileName, file.Windows[i].Name, err)
		}
		agents += countAgents(&file.Windows[i].PaneSpec)
	}
	if agents != 1 {
		return nil, fmt.Errorf("%s: exactly one pane across all windows must set agent = true (found %d)", FileName, agents)
	}
	return file.Windows, nil
}

// validateShape walks one window's tree enforcing the structure
// realizeLayout depends on: every split node has a real direction and at
// least two children, and no node is ambiguously both a split and a leaf.
func validateShape(n *PaneSpec) error {
	if len(n.Children) == 0 {
		if n.Direction != "" {
			return errors.New("leaf pane has a direction but no children")
		}
		return nil
	}
	if n.Agent {
		return errors.New("a split node cannot also be the agent pane")
	}
	if n.Cmd != "" {
		return errors.New("a split node cannot also have a cmd")
	}
	if n.Direction != "row" && n.Direction != "col" {
		return fmt.Errorf("invalid direction %q, want \"row\" or \"col\"", n.Direction)
	}
	if len(n.Children) < 2 {
		return errors.New("a split node needs at least 2 children")
	}
	for i := range n.Children {
		if err := validateShape(&n.Children[i]); err != nil {
			return err
		}
	}
	return nil
}

// HasAgent reports whether the Agent: true leaf lives anywhere in p's
// subtree — used by tmux.NewSessionWithLayout to find which window should
// get moomux's passed-through display name when left unnamed.
func (p PaneSpec) HasAgent() bool {
	return countAgents(&p) > 0
}

// countAgents counts leaves with Agent: true in n's subtree.
func countAgents(n *PaneSpec) int {
	if len(n.Children) == 0 {
		if n.Agent {
			return 1
		}
		return 0
	}
	total := 0
	for i := range n.Children {
		total += countAgents(&n.Children[i])
	}
	return total
}

// ParsePercent parses a size like "60%" or "60" into 60.0.
func ParsePercent(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(s), "%"), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return v, nil
}
