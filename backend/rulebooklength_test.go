// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind budget H3

//go:build !integration

package backendarch

// A rulebook is read in full by every session and, for its Craftsmanship
// section, by every gate prompt — so its length is a running cost rather than a
// matter of taste. Left alone it only grows: each addition is individually
// defensible, nothing ever asks whether the whole still earns its size, and the
// file arrives at a thousand lines nobody reads to the end.
//
// So this is a RATCHET, not a target. The ceiling is the size a rulebook
// actually is; the rule is that the number goes DOWN. Lowering it is an ordinary
// part of a change that shortens a file. Raising it is the thing to argue about,
// and a reviewer sees the argument because the number moves in the diff.
//
// Deliberately not a target of 150 or 200: a rulebook is the size its binding
// rules take, and a ceiling below that would be met by deleting a rule rather
// than by writing better — the one outcome this must not buy.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// defaultRulebookCeiling applies to a rulebook this test does not name. It is
// deliberately generous: an unnamed rulebook is a NEW one, and the point is to
// notice it, not to dictate its size before anybody has read it.
const defaultRulebookCeiling = 400

// ceilings is keyed by path relative to the repo root. Lower an entry when a
// change shortens that rulebook; a raise needs a reason in the pull request.
//
// This map does not decide WHICH files are checked — rulebookDirs walks the tree
// for that. A gate that carried its own list of rulebooks would be the second
// copy of the tree that *Reuse before you build* rule 5 is about, and it would go
// quietly short the day a directory grew one.
var ceilings = map[string]int{
	// Each number is the size that rulebook actually is. Raising one is how a new
	// rule pays for itself in a diff a reviewer can see; the alternative — trimming
	// unrelated lines elsewhere to stay under a fixed number — buys the number and
	// loses the rule.
	"AGENTS.md": 325,
	// Raised from 160 for the AI-hue rule: indigo marks agent-authored content,
	// and a reader who does not know that paints the meaning onto a decoration.
	// The reasoning lives in the design-system README; what is here is the twelve
	// lines a caller has to obey.
	"frontend/AGENTS.md": 172,
}

func TestNoRulebookGrowsPastItsCeiling(t *testing.T) {
	dirs := rulebookDirs(t)
	if len(dirs) == 0 {
		t.Fatal("rulebookDirs found no AGENTS.md at all — this gate would pass by " +
			"having nothing to check, which is the one way it must not break")
	}

	for _, dir := range dirs {
		path := filepath.Join(dir, "AGENTS.md")
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			t.Fatalf("relativise %s: %v", path, err)
		}
		rel = filepath.ToSlash(rel)

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		got := len(strings.Split(strings.TrimRight(string(raw), "\n"), "\n"))

		ceiling, named := ceilings[rel]
		if !named {
			// A rulebook nobody has set a ceiling for still gets one, so it cannot
			// grow without limit while it waits to be named.
			if got > defaultRulebookCeiling {
				t.Errorf("%s is %d lines and this test does not name it, so it fell back to "+
					"the default ceiling of %d. Add it to `ceilings` with the size it should be.",
					rel, got, defaultRulebookCeiling)
			}
			continue
		}

		if got > ceiling {
			t.Errorf("%s is %d lines, over its ceiling of %d.\n"+
				"Every session pays for this file, so it is a ratchet: cut the addition down, "+
				"move the reasoning to docs/, or — if the rule genuinely belongs here and cannot "+
				"be shorter — raise the ceiling and say why in the pull request.", rel, got, ceiling)
		}
		// A ceiling far above the file is not a ratchet any more: it stops
		// noticing growth long before the number is reached.
		if slack := ceiling - got; slack > 60 {
			t.Errorf("%s is %d lines against a ceiling of %d — %d lines of slack. "+
				"Lower the ceiling to about the current size; a ratchet this far above its "+
				"subject reports PASS through the growth it was written to catch.",
				rel, got, ceiling, slack)
		}
	}
}
