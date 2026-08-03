package watcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// rawSession captures the subset of fields moomux cares about from a
// ~/.claude/sessions/*.json file. Schema is best-effort: missing fields are tolerated.
type rawSession struct {
	CWD    string `json:"cwd"`
	Status string `json:"status"`
	Busy   *bool  `json:"busy,omitempty"`
	State  string `json:"state,omitempty"`
}

func parseFile(path string) (rawSession, error) {
	var rs rawSession
	data, err := os.ReadFile(path)
	if err != nil {
		return rs, err
	}
	if err := json.Unmarshal(data, &rs); err != nil {
		// Surface it: Snapshot.Err's contract promises "unparsable file"
		// reaches the caller instead of the session silently reading as
		// idle. Half-written files (the agent mid-save) clear on the next
		// tick.
		return rs, err
	}
	if rs.CWD != "" {
		rs.CWD = filepath.Clean(rs.CWD)
	}
	rs.Status = strings.ToLower(rs.Status)
	rs.State = strings.ToLower(rs.State)
	return rs, nil
}

// classify maps a rawSession to a State.
func classify(rs rawSession) State {
	if rs.Busy != nil {
		if *rs.Busy {
			return Working
		}
		return Done
	}
	switch {
	case rs.Status == "needs-input":
		return NeedsInput
	case rs.Status == "busy", rs.Status == "working", rs.State == "busy":
		return Working
	case rs.Status == "idle", rs.Status == "waiting", rs.State == "idle":
		return Done
	}
	// A status we don't recognize is not evidence the session is idle —
	// Unknown lets the merge in tick() keep any better signal for the path.
	return Unknown
}
