// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// A proposal whose prior rate no longer holds is refused. The sentence names the
// currency, the rate now in force, and the refresh to run.
func TestAStaleRateProposalSaysWhatMovedAndHowToRefresh(t *testing.T) {
	for _, tc := range []struct {
		name     string
		proposal fxRateProposal
		prior    string
		found    bool
		want     string
	}{
		{
			name:     "the prior rate was replaced",
			proposal: fxRateProposal{FromCurrency: "EUR", Rate: "1.1", ExpectedPriorRate: "1.05"},
			prior:    "1.0800000000", found: true,
			want: "The EUR exchange rate changed to 1.0800000000 after this proposal was made, so it was not applied. " +
				"Refresh the rates from their sources for a current proposal.",
		},
		{
			name:     "a rate appeared where none was in force",
			proposal: fxRateProposal{FromCurrency: "EUR", Rate: "1.1"},
			prior:    "1.0800000000", found: true,
			want: "The EUR exchange rate changed to 1.0800000000 after this proposal was made, so it was not applied. " +
				"Refresh the rates from their sources for a current proposal.",
		},
		{
			name:     "the prior rate is no longer in force",
			proposal: fxRateProposal{FromCurrency: "EUR", Rate: "1.1", ExpectedPriorRate: "1.05"},
			want: "The EUR exchange rate this proposal was made against is no longer in force, so it was not applied. " +
				"Refresh the rates from their sources for a current proposal.",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := fxPriorMatches(tc.proposal, tc.prior, tc.found)
			if !errors.Is(err, apperrors.ErrVersionSkew) || err.Error() != tc.want {
				t.Fatalf("answered %v, want version skew reading %q", err, tc.want)
			}
		})
	}
}
