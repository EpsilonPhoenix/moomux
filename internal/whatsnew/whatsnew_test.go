package whatsnew

import (
	"reflect"
	"testing"
)

const sample = `# v0.0.97
<!-- Release notes generated using configuration in .github/release.yml at main -->

## What's Changed
### New
* Send PR links by @afitzgerald in https://github.com/o/r/pull/99
### Fixed
* Keep a held delete key repeating by @someone-else in https://github.com/o/r/pull/97
### Empty
## New Contributors
* @someone-else made their first contribution in https://github.com/o/r/pull/97

**Full Changelog**: https://github.com/o/r/compare/v0.0.96...v0.0.97

# v0.0.96
## What's Changed

# v0.0.10
## What's Changed
* One by @a in https://x/1
`

func TestParse(t *testing.T) {
	want := []Release{
		{Version: "0.0.97", Sections: []Section{
			{Title: "New", Items: []string{"Send PR links"}},
			{Title: "Fixed", Items: []string{"Keep a held delete key repeating"}},
		}},
		// 0.0.96 had nothing user-facing, so it is not here.
		{Version: "0.0.10", Sections: []Section{{Items: []string{"One"}}}},
	}
	if got := Parse(sample); !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %+v, want %+v", got, want)
	}
	if got := Parse("* before any version\n**Full Changelog**: https://x"); len(got) != 0 {
		t.Fatalf("bullets before any version = %+v, want none", got)
	}
}

func TestAfter(t *testing.T) {
	all := Parse(sample)
	versions := func(rs []Release) []string {
		var v []string
		for _, r := range rs {
			v = append(v, r.Version)
		}
		return v
	}
	for _, c := range []struct {
		seen, current string
		want          []string
	}{
		// Numeric, not lexical: 0.0.10 is newer than 0.0.9.
		{"0.0.9", "0.0.97", []string{"0.0.97", "0.0.10"}},
		{"0.0.10", "0.0.97", []string{"0.0.97"}},
		{"", "0.0.97", []string{"0.0.97"}},
		// A running release with nothing user-facing says nothing to a new install.
		{"", "0.0.96", nil},
		// Releases newer than the running one are not shown.
		{"0.0.9", "0.0.50", []string{"0.0.10"}},
	} {
		if got := versions(After(all, c.seen, c.current)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("After(seen=%q, current=%q) = %v, want %v", c.seen, c.current, got, c.want)
		}
	}
}

func TestLoadWithoutBakedNotes(t *testing.T) {
	if n := Load("0.0.97"); n != nil {
		t.Fatalf("Load with no data file = %+v, want nil", n)
	}
}
