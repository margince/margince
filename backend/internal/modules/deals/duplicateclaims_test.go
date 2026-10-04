// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAChainOfDuplicatePairsRetiresOnlyAgainstASurvivor(t *testing.T) {
	a, b, c := ids.NewV7(), ids.NewV7(), ids.NewV7()
	// A–B and B–C are open pairs; all three cite one meeting. B is claimed by
	// A, and C only by B, which this pass retires.
	got := claimsRetired([]duplicateClaim{
		{suggestion: b, claimant: a, claimantOpen: true},
		{suggestion: c, claimant: b, claimantOpen: true},
	})
	if !slices.Equal(got, []ids.UUID{b}) {
		t.Fatalf("retired %v, want only %v: C's only claimant is itself retired", got, b)
	}
}

func TestADismissedOrAcceptedClaimantRetiresWhateverItsAge(t *testing.T) {
	open, retiredFirst, dismissed := ids.NewV7(), ids.NewV7(), ids.NewV7()
	got := claimsRetired([]duplicateClaim{
		{suggestion: retiredFirst, claimant: dismissed, claimantOpen: false},
		{suggestion: open, claimant: retiredFirst, claimantOpen: true},
		{suggestion: open, claimant: dismissed, claimantOpen: false},
	})
	if !slices.Equal(got, []ids.UUID{retiredFirst, open}) {
		t.Fatalf("retired %v, want both: a dismissal claims even when an open claimant is gone", got)
	}
}
