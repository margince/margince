// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning the deals module's two coverage flags into the lane's one nullable
// fact.
//
// Three inputs reach this and only ONE of them may become a claim. The other
// two — a committee the reader could not read in full, and a deal with no
// committee at all — are absences that render identically to the finding the
// moment they are rounded down to it, and the rounding is silent: the row
// simply says nobody is carrying a deal somebody is carrying, and the rep acts
// on it.

import (
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyAKnownUncoveredCommitteeBecomesAClaim(t *testing.T) {
	uncovered, covered, withheld := ids.NewV7(), ids.NewV7(), ids.NewV7()
	absent := ids.NewV7()
	cover := map[ids.UUID]deals.ChampionCover{
		uncovered: {},
		covered:   {Covered: true},
		// Covered stays false beside Withheld deliberately: a withheld read
		// answers "no champion seen", which is exactly the pair that must not
		// become a finding. A fixture setting both would let a check reading
		// only Covered pass for the wrong reason.
		withheld: {Withheld: true},
	}
	for _, tc := range []struct {
		name  string
		deal  ids.UUID
		claim bool
	}{
		{"a committee with no engaged champion", uncovered, true},
		{"a committee whose champion is engaged", covered, false},
		{"a committee the reader could not read in full", withheld, false},
		{"a deal carrying no seats at all", absent, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := noChampionOf(cover, tc.deal)
			if tc.claim {
				if got == nil || !*got {
					t.Errorf("states %v, want a no-champion claim", derefOrNil(got))
				}
				return
			}
			if got != nil {
				t.Errorf("states %v, want no claim at all", *got)
			}
		})
	}
}

// derefOrNil renders the tri-state for a failure message without panicking on
// the absent case the assertion is complaining about.
//
// A string rather than an any: the value only ever reaches a %v in a failure
// message, so rendering it here says what the caller does with it and keeps a
// bare any out of a signature.
func derefOrNil(v *bool) string {
	if v == nil {
		return "nothing"
	}
	return strconv.FormatBool(*v)
}
