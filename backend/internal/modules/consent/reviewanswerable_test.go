// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// The review state and the held-subject list both read answerableByContext, as
// does the recording store. A work item offers a context answer only where the
// store takes one.
func TestOnlyTheReasonsAStatementResolvesOfferAContextAnswer(t *testing.T) {
	subject := ids.NewV7().String()
	cases := map[string]struct {
		state string
		held  int
	}{
		commsauthz.ReasonNoEvidence:                 {ReviewNeedsContext, 1},
		commsauthz.ReasonClaimContradictsResolution: {ReviewNeedsRepair, 0},
		commsauthz.ReasonUnknownPurpose:             {ReviewNeedsRepair, 0},
		commsauthz.ReasonObjection:                  {ReviewNeedsRepair, 0},
	}
	for reason, want := range cases {
		t.Run(reason, func(t *testing.T) {
			refusals := []RefusedRecipient{{ReasonCode: reason, SubjectID: subject}}

			if got := stateFor(refusals); got != want.state {
				t.Errorf("state = %q, want %q", got, want.state)
			}
			if got := len(HeldSubjectsForReview(Review{Refusals: refusals})); got != want.held {
				t.Errorf("held subjects = %d, want %d", got, want.held)
			}
		})
	}
}
