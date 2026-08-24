// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package backendarch

// The rulebook is read in full by every session and every gate prompt, so its
// length is a running cost rather than a matter of taste. Left alone it only ever
// grows: each addition is individually defensible, nothing ever asks whether the
// whole still earns its size, and the file arrives at a thousand lines nobody
// reads to the end.
//
// So this is a RATCHET, not a target. The ceiling is the size the rulebook
// actually is; the rule is that the number goes DOWN. Lowering it is an ordinary
// part of a change that shortens the file. Raising it is the thing to argue
// about, and a reviewer gets to see the argument because the number moves in the
// diff.
//
// Deliberately not a target of 150 or 200: the rulebook is at the size its
// binding rules take, and a ceiling below that would be met by deleting a rule
// rather than by writing better — which is the one outcome this must not buy.

import (
	"os"
	"strings"
	"testing"
)

// ceilings maps each rulebook to the most lines it may carry. Lower these when a
// change shortens one; a raise needs a reason in the pull request.
var ceilings = map[string]int{
	"../AGENTS.md":          320,
	"../frontend/AGENTS.md": 160,
}

func TestNoRulebookGrowsPastItsCeiling(t *testing.T) {
	for path, ceiling := range ceilings {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		got := len(strings.Split(strings.TrimRight(string(raw), "\n"), "\n"))
		if got > ceiling {
			t.Errorf("%s is %d lines, over its ceiling of %d.\n"+
				"Every session and every gate prompt pays for this file, so it is a ratchet: "+
				"cut the addition down, move the reasoning to docs/, or — if the rule genuinely "+
				"belongs here and cannot be shorter — raise the ceiling in this test and say why "+
				"in the pull request.", path, got, ceiling)
		}
		// A ceiling far above the file is not a ratchet any more: it stops
		// noticing growth long before the number is reached. Keep it tight
		// enough that the next addition has to look at it.
		if slack := ceiling - got; slack > 60 {
			t.Errorf("%s is %d lines against a ceiling of %d — %d lines of slack. "+
				"Lower the ceiling to about the current size; a ratchet that has drifted this far "+
				"above its subject reports PASS through growth it was written to catch.",
				path, got, ceiling, slack)
		}
	}
}
