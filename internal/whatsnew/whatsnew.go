// Package whatsnew serves the release notes baked into this build, for the
// "what's new" screen every front end shows after an upgrade.
//
// The release workflow writes data/whatsnew.md (GitHub's generated notes for
// this tag plus the last nine releases', each under a "# vX.Y.Z" line, newest
// first) before it builds. The file is gitignored so the tree stays clean for
// goreleaser; a local build has none, so Load returns nil and nothing shows.
package whatsnew

import (
	"embed"
	"regexp"
	"strings"

	"github.com/erickgnclvs/moomux/internal/updatecheck"
)

//go:embed all:data
var files embed.FS

type Section struct {
	Title string   `json:"title"` // empty for a release from before the notes had categories
	Items []string `json:"items"`
}

type Release struct {
	Version  string    `json:"version"` // bare, no "v"
	Sections []Section `json:"sections"`
}

// Notes is what the core serves: the releases it has notes for, newest first,
// and the version it runs. A client keeps its own last-seen version and asks
// After what is new since.
type Notes struct {
	Version  string    `json:"version"`
	Releases []Release `json:"releases"`
}

// Load returns the baked notes for the running version, or nil for a build
// without any (dev, or built outside the release workflow).
func Load(version string) *Notes {
	data, err := files.ReadFile("data/whatsnew.md")
	version = strings.TrimPrefix(version, "v")
	if err != nil || version == "" || version == "dev" {
		return nil
	}
	releases := Parse(string(data))
	if len(releases) == 0 {
		return nil
	}
	return &Notes{Version: version, Releases: releases}
}

// After returns the releases newer than seen, up to current. Nothing seen yet
// (a fresh install, or the first upgrade to a version with this screen) gives
// the running release alone rather than the whole baked history.
func After(releases []Release, seen, current string) []Release {
	var out []Release
	for _, r := range releases {
		if seen == "" {
			if r.Version == current {
				out = append(out, r)
			}
		} else if updatecheck.Newer(seen, r.Version) && !updatecheck.Newer(current, r.Version) {
			out = append(out, r)
		}
	}
	return out
}

var byline = regexp.MustCompile(` by @\S+ in \S+$`)

// Parse cuts GitHub's generated-notes markdown, one body per "# vX.Y.Z" line,
// down to what a user reads: the "###" categories .github/release.yml defines
// and their bullets, minus the " by @author in <PR url>" tail. "New
// Contributors" and "Full Changelog" go too, and so does a release with
// nothing left.
func Parse(markdown string) []Release {
	var all []Release
	skipping := false
	for _, raw := range strings.Split(markdown, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
		case strings.HasPrefix(line, "# "):
			all = append(all, Release{Version: strings.TrimPrefix(line[2:], "v")})
			skipping = false
		case strings.HasPrefix(line, "## "):
			skipping = line == "## New Contributors"
		case skipping || len(all) == 0:
		case strings.HasPrefix(line, "### "):
			r := &all[len(all)-1]
			r.Sections = append(r.Sections, Section{Title: line[4:]})
		case strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "- "):
			r := &all[len(all)-1]
			if len(r.Sections) == 0 {
				r.Sections = append(r.Sections, Section{})
			}
			s := &r.Sections[len(r.Sections)-1]
			s.Items = append(s.Items, byline.ReplaceAllString(line[2:], ""))
		}
	}
	var out []Release
	for _, r := range all {
		var secs []Section
		for _, s := range r.Sections {
			if len(s.Items) > 0 {
				secs = append(secs, s)
			}
		}
		if len(secs) > 0 {
			out = append(out, Release{Version: r.Version, Sections: secs})
		}
	}
	return out
}
