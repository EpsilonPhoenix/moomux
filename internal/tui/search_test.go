package tui

import (
	"testing"

	"github.com/erickgnclvs/moomux/internal/session"
)

func TestMatchSessionsFiltersByNameCaseInsensitive(t *testing.T) {
	all := []session.Session{
		{ID: "1", Project: "moomux", Name: "feature-auth"},
		{ID: "2", Project: "moomux", Name: "bugfix-login"},
		{ID: "3", Project: "eg_system", Name: "FEATURE-billing"},
	}

	got := matchSessions(all, "feature")

	if len(got) != 2 {
		t.Fatalf("matchSessions(%q) = %d results, want 2 (got %+v)", "feature", len(got), got)
	}
	for _, s := range got {
		if s.ID != "1" && s.ID != "3" {
			t.Errorf("unexpected match %+v", s)
		}
	}
}

func TestMatchSessionsIncludesArchivedSessions(t *testing.T) {
	all := []session.Session{
		{ID: "1", Project: "moomux", Name: "old-feature", Archived: true},
		{ID: "2", Project: "moomux", Name: "new-feature", Archived: false},
	}

	got := matchSessions(all, "feature")

	if len(got) != 2 {
		t.Fatalf("matchSessions(%q) = %d results, want 2 (archived sessions must be included)", "feature", len(got))
	}
}

func TestMatchSessionsEmptyQueryReturnsAllSortedByProjectThenName(t *testing.T) {
	all := []session.Session{
		{ID: "1", Project: "zeta", Name: "b-session"},
		{ID: "2", Project: "alpha", Name: "z-session"},
		{ID: "3", Project: "alpha", Name: "a-session"},
	}

	got := matchSessions(all, "")

	if len(got) != 3 {
		t.Fatalf("matchSessions(\"\") = %d results, want 3", len(got))
	}
	wantOrder := []string{"3", "2", "1"} // alpha/a-session, alpha/z-session, zeta/b-session
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Errorf("position %d: got id %s, want %s (order=%+v)", i, got[i].ID, id, got)
		}
	}
}

func TestMatchSessionsNoMatchReturnsEmpty(t *testing.T) {
	all := []session.Session{{ID: "1", Project: "moomux", Name: "feature-auth"}}

	got := matchSessions(all, "nonexistent")

	if len(got) != 0 {
		t.Fatalf("matchSessions with no match = %d results, want 0", len(got))
	}
}
