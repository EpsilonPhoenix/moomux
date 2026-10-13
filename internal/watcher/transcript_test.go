package watcher

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erickgnclvs/moomux/internal/prompt"
)

// tline renders one transcript entry of type typ. content, when non-nil,
// becomes message.content; extra fields (isSidechain, subtype, …) merge in.
func tline(t *testing.T, typ string, content any, extra map[string]any) string {
	t.Helper()
	e := map[string]any{"type": typ}
	if content != nil {
		e["message"] = map[string]any{"content": content}
	}
	maps.Copy(e, extra)
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// says is assistant content holding one text block.
func says(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}

// writeTranscript writes lines as session sid's transcript under dir, where
// Claude Code keeps it for a session started in cwd.
func writeTranscript(t *testing.T, dir, cwd, sid string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, prompt.EncodeCwd(cwd), sid+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		t.Fatal(err)
	}
}

func TestLastAssistantText(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  string
		ok    bool
	}{
		{
			// The order Claude Code 2.1.287 writes a turn in: the thinking
			// and text of one message as separate entries, then metadata.
			name: "text after its thinking, before metadata",
			lines: []string{
				tline(t, "user", "go", nil),
				tline(t, "assistant", []map[string]any{{"type": "thinking", "thinking": "hmm"}}, nil),
				tline(t, "assistant", says("Should I continue?"), nil),
				tline(t, "attachment", nil, map[string]any{"attachment": map[string]any{"type": "prompt_snapshot"}}),
				tline(t, "system", nil, map[string]any{"subtype": "turn_duration"}),
				tline(t, "last-prompt", nil, map[string]any{"lastPrompt": "go"}),
			},
			want: "Should I continue?", ok: true,
		},
		{
			name: "answered",
			lines: []string{
				tline(t, "assistant", says("Should I continue?"), nil),
				tline(t, "user", "yes", nil),
			},
		},
		{
			name: "interrupted tool call",
			lines: []string{
				tline(t, "assistant", says("Running the tests, ok?"), nil),
				tline(t, "assistant", []map[string]any{{"type": "tool_use", "name": "Bash"}}, nil),
				tline(t, "user", []map[string]any{{"type": "tool_result", "content": "denied"}}, nil),
			},
		},
		{
			name: "subagent and meta entries skipped",
			lines: []string{
				tline(t, "assistant", says("Which one?"), nil),
				tline(t, "assistant", says("subagent: done"), map[string]any{"isSidechain": true}),
				tline(t, "user", "Caveat: local command output follows", map[string]any{"isMeta": true}),
			},
			want: "Which one?", ok: true,
		},
		{
			name:  "no reply yet",
			lines: []string{tline(t, "user", "go", nil)},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTranscript(t, t.TempDir(), "/wt/a", "s1", c.lines...)
			got, ok := lastAssistantText(path)
			if got != c.want || ok != c.ok {
				t.Errorf("got %q, %v; want %q, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

// Only the last transcriptTail bytes are read; the line the window cuts in
// half must not hide the message after it.
func TestLastAssistantTextReadsOnlyTheTail(t *testing.T) {
	path := writeTranscript(t, t.TempDir(), "/wt/a", "s1",
		tline(t, "user", strings.Repeat("x", transcriptTail), nil),
		tline(t, "assistant", says("Still there?"), nil),
	)
	if got, ok := lastAssistantText(path); got != "Still there?" || !ok {
		t.Errorf("got %q, %v; want the last message", got, ok)
	}
}

func TestEndsWithQuestion(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"Should I proceed?", true},
		{"Done. Tests pass.", false},
		{"Want me to commit? I can also open a PR.", true},
		{"Which approach?\n\n1. Rewrite it\n2. Patch it", true},
		{"Should I save it? It stays local.\n\nSources:\n- [docs](https://example.com/a?b=1)\n- [issue](https://example.com/2)", true},
		{"PR is up: https://example.com/pull/12?expand=1", false},
		{"**Shall I merge?**", true},
		{"これでいいですか？", true},
		{"Run this:\n\n```\nis it ok?\n```", false},
		{"Is this the question?\n\nSecond paragraph.\n\nThird paragraph.", false},
		{"| ready? | yes |\n|---|---|", false},
		{"", false},
	}
	for _, c := range cases {
		if got := endsWithQuestion(c.text); got != c.want {
			t.Errorf("endsWithQuestion(%q) = %v, want %v", c.text, got, c.want)
		}
	}
}
