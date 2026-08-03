package tui

var quipsWorking = []string{
	"on it...",
	"crunching...",
	"in the zone",
	"don't interrupt me",
	"almost there...",
	"trust the process",
	"heads down",
	"grinding...",
	"locked in",
	"making it happen",
	"cooking something up",
	"deep focus",
}

var quipsDone = []string{
	"moo...",
	"all done",
	"chewing cud",
	"your move",
	"ready when you are",
	"standing by",
	"at your service",
	"just here vibing",
	"anytime now...",
	"still here",
	"no rush... or is there?",
	"wrapped up",
}

var quipsNeedsInput = []string{
	"need a word",
	"stuck on a fence",
	"c'mon, look over here",
	"blocked, moo!",
	"waiting on you specifically",
	"can't go further without you",
	"permission, please",
	"hey, over here",
	"decision needed",
	"paused for you",
	"your call to make",
	"stalled, need input",
}

var quipsParked = []string{
	"zzz...",
	"taking a nap",
	"out to pasture",
	"resting",
	"offline",
	"on break",
	"do not disturb",
	"gone fishin'",
	"lights out",
	"hibernating",
	"clocked out",
	"pasture mode",
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
