// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The worklist reports a source as possibly having more work behind it when its
// lane came back exactly at its bound. Two of those bounds live behind a seam
// the attention package cannot import, so it keeps its own copy — and a copy
// nobody checks is a wrong number waiting.
//
// Under-reporting is the one way the reach figures must not fail: a source
// silently marked complete tells a rep there is no more work of that kind, and
// no assertion anywhere fails to say otherwise. So this reads BOTH sides out of
// the tree and fails when they disagree, in either direction.

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

func TestTheWorklistsCopyOfALaneBoundMatchesTheLaneItself(t *testing.T) {
	t.Parallel()

	mirrors := []struct {
		name      string
		ownerFile string
		ownerDecl string
		copyDecl  string
	}{
		{
			name:      "quiet deals",
			ownerFile: "internal/compose/slipping.go",
			ownerDecl: `slippingScanLimit\s*=\s*(\d+)`,
			copyDecl:  `quietDealBound\s*=\s*(\d+)`,
		},
		{
			name:      "relationship decay",
			ownerFile: "internal/compose/attentionlanesseam.go",
			ownerDecl: `decayCandidateCap\s*=\s*(\d+)`,
			copyDecl:  `decayBound\s*=\s*(\d+)`,
		},
	}
	// The file the queue keeps its copies in. Named rather than searched
	// because the mirror is the SUBJECT here: a gate that hunted for the
	// constant wherever it had moved to would still pass after somebody
	// deleted it, which is the one failure this pairing exists to prevent.
	//
	// It moved once already — from worklist.go, when the assembler was split —
	// so if it moves again, repoint this line rather than adding a second read.
	const copiesLive = "internal/compose/attention/unseen.go"
	copies := read(t, copiesLive)
	for _, mirror := range mirrors {
		owner := number(t, read(t, mirror.ownerFile), mirror.ownerDecl, mirror.ownerFile)
		held := number(t, copies, mirror.copyDecl, copiesLive)
		if owner != held {
			t.Errorf("%s: the lane reads %d rows but the worklist believes %d — the smaller number "+
				"decides which side is wrong, and a source truncated past the copy reports itself complete",
				mirror.name, owner, held)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func number(t *testing.T, body, pattern, where string) int {
	t.Helper()
	found := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if found == nil {
		t.Fatalf("%s no longer declares %s — the mirror it holds cannot be checked", where, pattern)
	}
	value, err := strconv.Atoi(found[1])
	if err != nil {
		t.Fatalf("%s: %q is not a number: %v", where, found[1], err)
	}
	return value
}
