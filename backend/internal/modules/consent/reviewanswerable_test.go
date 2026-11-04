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
		commsauthz.ReasonDealNotRequested:           {ReviewNeedsContext, 1},
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

// A refusal keeps the deal its message named, so the answer a rep gives in the
// review form is recorded about that deal and reused for the next message
// about it. A message that named no deal records an answer about none.
func TestARefusalKeepsTheDealItsAnswerIsAbout(t *testing.T) {
	deal := ids.NewV7()
	named := refusedRecipientOf(commsauthz.Decision{
		Verdict: commsauthz.VerdictReview, ReasonCode: commsauthz.ReasonDealNotRequested,
		Evidence: commsauthz.Evidence{DealID: deal},
	})
	if named.dealID() != deal {
		t.Errorf("the refusal names deal %s, want %s", named.dealID(), deal)
	}
	if none := refusedRecipientOf(commsauthz.Decision{Verdict: commsauthz.VerdictReview}); !none.dealID().IsZero() || none.DealID != nil {
		t.Errorf("a message naming no deal left deal %v on its refusal", none.DealID)
	}
}
