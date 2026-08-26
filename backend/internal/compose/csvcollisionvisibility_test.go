// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Visibility is asked of EVERY candidate the ladder matched, not of its winner.
//
// The winner is the highest confidence with the lowest uuid breaking ties, which
// has nothing to do with who may read it. Asking only about the winner let an
// invisible record MASK a visible one behind it: with the hidden company present
// the row created and reported no duplicate, without it the visible candidate
// won and the row was skipped. The hidden company was inferable from the
// disposition either way — the same existence oracle in a different hat.
//
// candidatesOf is the seam that has to see the whole set. This is a unit gate on
// it, because building two same-named companies with different visibility takes
// two seats the import suite's harness does not have.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryMatchedCandidateIsAskedAboutNotJustTheWinner(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()

	// A fuzzy match ranks its candidates; every one of them is a company the row
	// might mean, and a visibility question skipping any of them lets that one
	// change the answer without being asked.
	fuzzy := people.OrganizationMatch{
		Decision:       people.DecisionFuzzyReview,
		OrganizationID: ids.From[ids.OrganizationKind](first),
		Ranked: []people.OrganizationCandidateScore{
			{OrganizationID: ids.From[ids.OrganizationKind](first), Confidence: 1},
			{OrganizationID: ids.From[ids.OrganizationKind](second), Confidence: 1},
		},
	}
	got := candidatesOf(fuzzy)
	if len(got) != 2 {
		t.Fatalf("candidatesOf answered %d of 2 ranked candidates — one the caller CAN see, ranked "+
			"behind one they cannot, would never be asked about and the hidden record would decide "+
			"the outcome", len(got))
	}

	// An exact (domain) collision carries no ranked set; its answer is the id
	// itself, and dropping it would make a real collision invisible.
	exact := people.OrganizationMatch{
		Decision:       people.DecisionExactCollision,
		OrganizationID: ids.From[ids.OrganizationKind](first),
	}
	if got := candidatesOf(exact); len(got) != 1 || got[0] != ids.From[ids.OrganizationKind](first) {
		t.Errorf("candidatesOf answered %v for an exact collision, want the matched id — it carries "+
			"no ranked set, so reading only Ranked would lose the collision entirely", got)
	}
}
