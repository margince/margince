// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// The reference direction is one-way: AGENTS.md links down into docs/, and
// nothing under docs/ links back up to a rulebook. This gate holds the second
// half, which is the half that rots.
//
// An upward link is nearly always a link to a HEADING, and a heading is the part
// of a rulebook most likely to be reworded. When one is, the anchor stops
// resolving and nothing notices: markdown has no build step, a dead anchor
// scrolls to the top of the file instead of erroring, and the reader concludes
// the rule is not there. Six such links went dead simultaneously the day these
// sections were reorganised, all of them pointing at anchors that had been
// correct when written.
//
// A page that NAMES a rule in prose — "the rulebook's *Reuse before you build*"
// — survives every rename, costs a reader one search, and cannot go quietly
// wrong. That is the shape this asks for, and it is also why the rule is stated
// as a direction rather than as "keep the anchors correct": correctness that
// depends on remembering is what gates are for.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// upwardLink matches a markdown link whose target climbs out of docs/ to a
// rulebook, with or without an anchor: `](../AGENTS.md#craftsmanship)`.
//
// It deliberately does NOT match a bare mention. Naming `AGENTS.md` in prose or
// in backticks is the sanctioned way to point at a rule, so only an actual link
// target is a finding.
var upwardLink = regexp.MustCompile(`]\(\s*(?:\.\./)+(?:AGENTS|CLAUDE)\.md(?:#[^)]*)?\s*\)`)

func TestNothingUnderDocsLinksUpToARulebook(t *testing.T) {
	const docsRoot = "../docs"

	scanned := 0
	err := filepath.WalkDir(docsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		scanned++
		for i, line := range strings.Split(string(raw), "\n") {
			for _, hit := range upwardLink.FindAllString(line, -1) {
				t.Errorf("%s:%d links up to a rulebook: %s\n"+
					"The reference direction is one-way — AGENTS.md links down into docs/, never the reverse. "+
					"An upward link points at a heading, and a reworded heading breaks it without breaking any "+
					"build: the anchor silently scrolls to the top and the reader concludes the rule is missing. "+
					"Name the rule in prose instead (\"the rulebook's *Reuse before you build*\"), which survives "+
					"the rename, and add the downward link from AGENTS.md if one is missing.",
					path, i+1, hit)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", docsRoot, err)
	}
	if scanned == 0 {
		t.Fatalf("no markdown found under %s — this gate is reading a tree shape that is gone", docsRoot)
	}
}
