// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Turning the deals module's coverage flags into the lane's two champion facts.
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
	uncovered, covered, withheld, absent := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	importedSilent, importedWithheld, importedSeatless := ids.NewV7(), ids.NewV7(), ids.NewV7()
	importedNamed := ids.NewV7()
	answer := championAnswer{
		cover: map[ids.UUID]deals.ChampionCover{
			uncovered: {},
			covered:   {Covered: true, ChampionNamed: true},
			// Covered stays false beside Withheld deliberately: a withheld read
			// answers "no champion seen", which is exactly the pair that must not
			// become a finding.
			withheld:         {Withheld: true},
			importedSilent:   {},
			importedWithheld: {Withheld: true},
			importedNamed:    {ChampionNamed: true},
		},
		imported: map[ids.UUID]bool{
			importedSilent: true, importedWithheld: true, importedSeatless: true, importedNamed: true,
		},
	}
	for _, tc := range []struct {
		name           string
		deal           ids.UUID
		claim, unknown bool
	}{
		{"a committee with no engaged champion", uncovered, true, false},
		{"a committee whose champion is engaged", covered, false, false},
		{"a committee the reader could not read in full", withheld, false, false},
		{"a deal created here carrying no seats at all", absent, false, false},
		{"an imported committee naming no champion", importedSilent, false, true},
		{"an imported committee the reader could not read in full", importedWithheld, false, true},
		{"an imported deal carrying no seats at all", importedSeatless, false, true},
		{"an imported deal whose champion was named here", importedNamed, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := answer.noChampion(tc.deal)
			if tc.claim != (got != nil && *got) {
				t.Errorf("states %v as no champion, want a claim=%v", derefOrNil(got), tc.claim)
			}
			if got != nil && !*got {
				t.Errorf("states a false no-champion, which the wire never carries")
			}
			if unknown := answer.championUnknown(tc.deal); unknown != tc.unknown {
				t.Errorf("champion unknown = %v, want %v", unknown, tc.unknown)
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
