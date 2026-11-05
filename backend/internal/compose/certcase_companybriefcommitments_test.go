// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

// A scenario can carry the promises production sends, each under its own label.
// The certified "what is open" request then has the same input as the product.
func TestACompanyBriefFixtureCarriesItsOpenCommitments(t *testing.T) {
	in, label, err := companyBriefInput(companyBriefFixture{
		Name: "Brandt",
		Commitments: []companyBriefCommitmentFixture{
			{Label: "audit", Claim: "Share the security audit", Side: "ours", Due: "2026-07-28T09:00:00Z"},
			{Label: "invoice", Claim: "Pay the invoice", Side: "theirs", Holder: "Dana Buyer"},
		},
	})
	if err != nil {
		t.Fatalf("companyBriefInput: %v", err)
	}
	if len(in.OpenCommitments) != 2 || in.OpenCommitments[0].ActivityID != label["audit"] ||
		in.OpenCommitments[1].Holder != "Dana Buyer" {
		t.Errorf("commitments = %+v, labels = %v", in.OpenCommitments, label)
	}
	if _, _, err := companyBriefInput(companyBriefFixture{
		Commitments: []companyBriefCommitmentFixture{{Label: "x", Claim: "y", Side: "mine"}},
	}); err == nil {
		t.Error("a side the contract does not carry was accepted")
	}
}
