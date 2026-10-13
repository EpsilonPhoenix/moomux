package watcher

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// transcriptTail is how much of a transcript's end is read to find its last
// message. Attachment and metadata entries follow a turn's final text — up
// to ~113 KiB of them in transcripts seen in the wild — so the window has to
// reach well past them; a last message older than the window reads as no
// question.
const transcriptTail = 1 << 20

// asksQuestion reports whether the Claude Code transcript at path ends with
// the agent asking the user something. A guess from wording (see
// endsWithQuestion), not a signal from Claude Code: in its session file, a
// turn that ends on a question looks like any other finished turn.
func asksQuestion(path string) bool {
	text, ok := lastAssistantText(path)
	return ok && endsWithQuestion(text)
}

// transcriptEntry is the part of a transcript line lastAssistantText reads.
type transcriptEntry struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	IsMeta      bool   `json:"isMeta"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// lastAssistantText returns the text of the transcript's last message, when
// that message is the assistant's. A user entry after it — a reply, a tool
// result, an interrupt — means nothing the assistant said is still waiting
// on an answer. Everything else Claude Code appends around a turn
// (attachments, system and metadata entries, a message's separate thinking
// entry, subagent and meta entries) is skipped.
func lastAssistantText(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", false
	}
	off := max(info.Size()-transcriptTail, 0)
	buf := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return "", false
	}
	lines := bytes.Split(buf, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var e transcriptEntry
		// A line that doesn't parse is the window's cut-off first line, or
		// a last one still being written.
		if json.Unmarshal(lines[i], &e) != nil || e.IsSidechain || e.IsMeta {
			continue
		}
		switch e.Type {
		case "user":
			return "", false
		case "assistant":
			if text := textOf(e.Message.Content); text != "" {
				return text, true
			}
		}
	}
	return "", false
}

// textOf joins the text blocks of a message's content.
func textOf(content json.RawMessage) string {
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// notProse matches the lines endsWithQuestion skips: list items, table rows
// and headings.
var notProse = regexp.MustCompile(`^([-*+]\s|\d+[.)]\s|\||#{1,6}\s)`)

// sentenceQuestion matches a question mark that ends a sentence: followed by
// the end of the line, a space, or closing punctuation or markdown. That
// leaves out the '?' in a URL's query string.
var sentenceQuestion = regexp.MustCompile(`[?？]([\s*_)\]"'”’]|$)`)

// endsWithQuestion reports whether one of text's last two prose lines asks
// something. List items, table rows, headings and fenced code aren't prose,
// so a question followed by its options, or by a list of sources, still
// counts.
//
// The ceiling is that of any wording heuristic: a rhetorical question near
// the end reads as asking, and a question placed earlier in a long reply
// doesn't.
func endsWithQuestion(text string) bool {
	lines := strings.Split(text, "\n")
	inCode := false
	for i, prose := len(lines)-1, 0; i >= 0 && prose < 2; i-- {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "```"):
			inCode = !inCode
		case inCode, line == "", notProse.MatchString(line):
		default:
			prose++
			if sentenceQuestion.MatchString(line) {
				return true
			}
		}
	}
	return false
}
