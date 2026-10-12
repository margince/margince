// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The domain's claimant leads the ladder's answer once: a fuzzy score for the
// same company folds into it, keeping whether the names were the same, and
// every other candidate follows in the ladder's order.
func TestTheNameDomainClaimantLeadsTheLadderOnce(t *testing.T) {
	claimant := ids.From[ids.CompanyKind](ids.NewV7())
	other := ids.From[ids.CompanyKind](ids.NewV7())
	match := CompanyMatch{Decision: DecisionFuzzyReview, Ranked: []CompanyCandidateScore{
		{CompanyID: other, Confidence: 0.9},
		{CompanyID: claimant, Confidence: 0.8, ExactName: true},
	}}

	got := rankNameDomainClaimantFirst(match, []CompanyCandidateScore{{CompanyID: claimant, Confidence: 1}})

	if got.CompanyID != claimant || len(got.Ranked) != 2 {
		t.Fatalf("the answer leads with %v over %d candidates, want the claimant over 2", got.CompanyID, len(got.Ranked))
	}
	if got.Ranked[0].CompanyID != claimant || !got.Ranked[0].ExactName || got.Ranked[1].CompanyID != other {
		t.Fatalf("ranked %+v, want the claimant first carrying its exact name, then the other", got.Ranked)
	}
	if unchanged := rankNameDomainClaimantFirst(match, nil); len(unchanged.Ranked) != 2 ||
		unchanged.Ranked[0].CompanyID != other {
		t.Fatalf("with no claimant the answer is %+v, want the ladder's own", unchanged.Ranked)
	}
}
