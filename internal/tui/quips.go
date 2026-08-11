package tui

import "github.com/erickgnclvs/moomux/internal/watcher"

var quipsWorking = []string{
	"udderly focused",
	"milking this for progress",
	"no time to graze",
	"plowing through it",
}

var quipsDone = []string{
	"that's a wrap, moove along",
	"udder success",
	"cow-culated risk paid off",
	"steaks were high, nailed it",
}

var quipsNeedsInput = []string{
	"udderly stuck without you",
	"moo-ve this along, please",
	"cow-nfused, help me out",
}

var quipsParked = []string{
	"chewing the cud, nothing to see",
	"herd nothing, seen nothing",
	"mootering off for now",
}

// quipPool returns the quip pool matching a session's state.
func quipPool(st watcher.State) []string {
	switch st {
	case watcher.Working:
		return quipsWorking
	case watcher.Done:
		return quipsDone
	case watcher.NeedsInput:
		return quipsNeedsInput
	default:
		return quipsParked
	}
}

// stateEyes returns the cow's eyes for a session's state, used by both the
// header cow and the detail panel's cowsay.
func stateEyes(st watcher.State) string {
	switch st {
	case watcher.Working:
		return "**"
	case watcher.Done:
		return "oo"
	case watcher.NeedsInput:
		return "!!"
	default:
		return "--"
	}
}

func pickQuip(sessionID string, pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	var h uint32
	for _, c := range sessionID {
		h = h*31 + uint32(c)
	}
	return pool[h%uint32(len(pool))]
}
