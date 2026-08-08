// Package session persists moomux session metadata to JSON.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Session struct {
	ID           string    `json:"id"`
	Project      string    `json:"project"`
	Name         string    `json:"name"`
	Branch       string    `json:"branch"`
	WorktreePath string    `json:"worktree_path"`
	TmuxSession  string    `json:"tmux_session"`
	CreatedAt    time.Time `json:"created_at"`
	Agent        string    `json:"agent,omitempty"`       // "claude", "codex", "opencode"; empty = "claude"
	Dangerous    bool      `json:"dangerous,omitempty"`   // run Agent with its permission-skipping flag (claude: --dangerously-skip-permissions, codex: --yolo); no-op for opencode
	AgentPort    int       `json:"agent_port,omitempty"`  // HTTP port for OpenCode API; 0 = not applicable
	Ticket       string    `json:"ticket,omitempty"`      // ticket URL (e.g. Asana, Jira, Linear)
	PR           string    `json:"pr,omitempty"`          // pull request URL (e.g. GitHub, GitLab)
	Order        int64     `json:"order,omitempty"`       // manual sort position within a project; 0 = unset, falls back to CreatedAt
	Archived     bool      `json:"archived,omitempty"`    // hidden from the default list, but not deleted
	NewBranch    bool      `json:"new_branch,omitempty"`  // true if moomux created Branch fresh (vs. checking out an existing one); safe to delete on session delete
	TermTabID    string    `json:"term_tab_id,omitempty"` // terminal-specific tab/window id this session was last opened in (currently only iTerm2 sets it); lets reopen jump back to it instead of creating a new tab
	Prompt       string    `json:"prompt,omitempty"`      // first prompt typed into the agent at creation time, captured directly rather than relying on the agent's own log
}

// AgentName returns the effective agent name, defaulting to "claude" for legacy sessions.
func (s Session) AgentName() string {
	if s.Agent == "" {
		return "claude"
	}
	return s.Agent
}

type fileShape struct {
	Version  int                `json:"version"`
	Sessions map[string]Session `json:"sessions"`
}

type Store struct {
	Path string

	mu       sync.Mutex
	sessions map[string]Session
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = map[string]Session{}
	return s.reloadLocked()
}

// reloadLocked re-reads the store file into memory. Every mutating method
// calls this first so its write lands on top of whatever other moomux
// processes (e.g. a `moomux spawn` sharing this same sessions.json) have
// saved since this Store last loaded, instead of overwriting their changes
// with a stale in-memory snapshot. Caller must hold mu.
func (s *Store) reloadLocked() error {
	if s.sessions == nil {
		s.sessions = map[string]Session{}
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var f fileShape
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	if f.Sessions != nil {
		s.sessions = f.Sessions
	}
	return nil
}

func (s *Store) save() error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f := fileShape{Version: 1, Sessions: s.sessions}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	// A fixed ".tmp" name races when multiple moomux processes share this
	// store: one process's write/rename can clobber or steal another's
	// in-flight temp file. A per-invocation temp file avoids that.
	tmp, err := os.CreateTemp(dir, filepath.Base(s.Path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.Path)
}

func (s *Store) Put(sess Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	s.sessions[sess.ID] = sess
	return s.save()
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	delete(s.sessions, id)
	return s.save()
}

// SetArchived flips a session's Archived flag and persists it. The session
// itself (worktree, tmux entry, metadata) is left untouched — archiving only
// hides it from the default list.
func (s *Store) SetArchived(id string, archived bool) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return Session{}, err
	}
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, fmt.Errorf("unknown session %q", id)
	}
	sess.Archived = archived
	s.sessions[id] = sess
	if err := s.save(); err != nil {
		return Session{}, err
	}
	return sess, nil
}

// Reload re-reads the store file from disk, picking up sessions written by
// another moomux process (e.g. `moomux spawn`) since this Store last loaded.
func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reloadLocked()
}

func (s *Store) Get(id string) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

// All returns every session ordered by manual Order ascending (0 = unset,
// so unordered sessions sort first — matching where a freshly created
// session should land), falling back to CreatedAt descending among
// sessions with equal Order.
func (s *Store) All() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0, len(s.sessions))
	for _, v := range s.sessions {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ByProject(project string) []Session {
	all := s.All()
	out := make([]Session, 0, len(all))
	for _, sess := range all {
		if sess.Project == project {
			out = append(out, sess)
		}
	}
	return out
}

// Reorder assigns sequential Order values (1..N) to the given sessions, in
// the order given, and persists the store in a single write. Callers pass a
// full project's sessions (e.g. from ByProject) after rearranging them, so
// the rest of that project's ordering stays self-consistent; sessions
// outside the given slice are left untouched.
func (s *Store) Reorder(sessions []Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	// sessions may be a snapshot the caller fetched before this reload;
	// writing it back wholesale would clobber any other field a concurrent
	// process changed since (e.g. a tag set from another moomux process).
	// Apply only the Order change, to the freshly reloaded session.
	for i, sess := range sessions {
		current, ok := s.sessions[sess.ID]
		if !ok {
			continue
		}
		current.Order = int64(i + 1)
		s.sessions[sess.ID] = current
	}
	return s.save()
}

func MakeID(project, name string) string { return project + ":" + name }

func DefaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "moomux", "sessions.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "moomux", "sessions.json")
}
