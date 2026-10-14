// Package filecap is the one size cap on a file crossing the wire —
// SaveFile up, ReadFile and ReadBaseFile down — and the error that says a
// file is over it, so every method refuses the same files in the same
// words.
package filecap

import (
	"fmt"
	"math"
	"strconv"
)

// Max caps one file. A phone photo is a few MB; this is room for a short
// screen recording without letting a client fill the disk in one call.
const Max = 32 << 20

// TooBig says size bytes is over limit, in megabytes to one decimal place:
// the size rounded up and the limit down, so a size just over a limit never
// reads as equal to it — "32.1 MB is over the 32 MB limit", not "32 MB".
func TooBig(size, limit int64) error {
	return fmt.Errorf("%s MB is over the %s MB limit", mb(size, math.Ceil), mb(limit, math.Floor))
}

func mb(n int64, round func(float64) float64) string {
	return strconv.FormatFloat(round(float64(n)*10/(1<<20))/10, 'f', -1, 64)
}
