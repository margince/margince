// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The composer offers a recorded reason where the preview says a refusal can be
// overruled. So the preview never promises one the record would refuse.
func TestThePreviewOffersAnOverrideOnlyWhereARecordedReasonIsAccepted(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		commsauthz.ReasonNoEvidence:                     true,
		commsauthz.ReasonNoMarketingConsent:             true,
		commsauthz.ReasonLegacyTransactionalUnevidenced: true,
		commsauthz.ReasonUnknownPurpose:                 false,
		commsauthz.ReasonClaimContradictsResolution:     false,
		commsauthz.ReasonObjection:                      false,
	}
	for code, want := range cases {
		decision := commsauthz.Decision{
			Verdict:    commsauthz.VerdictDeny,
			ReasonCode: code,
			Mode:       commsauthz.ModeEnforce,
		}
		out := previewResponse(SendPreview{Decisions: commsauthz.DecisionSet{Decisions: []commsauthz.Decision{decision}}})
		got := out.Recipients[0].CanBeOverruled
		if got == nil {
			t.Fatalf("%s: can_be_overruled is absent", code)
		}
		if *got != want {
			t.Errorf("%s: can_be_overruled = %v, want %v", code, *got, want)
		}
	}
}

// The preview carries the sender's claim beside the engine's reading. An
// unproven active-deal claim never resolves, and the composer names the
// missing deal from the claim.
func TestThePreviewCarriesTheClaimBesideTheResolvedCategory(t *testing.T) {
	t.Parallel()
	out := previewResponse(SendPreview{Decisions: commsauthz.DecisionSet{Decisions: []commsauthz.Decision{{
		Verdict: commsauthz.VerdictReview, ReasonCode: commsauthz.ReasonNoEvidence, Mode: commsauthz.ModeEnforce,
		Requested: commsauthz.CategoryActiveDealFollowup, Resolved: commsauthz.CategoryRequestedFollowup,
	}}}})
	got := out.Recipients[0]
	if got.RequestedCategory == nil || *got.RequestedCategory != "active_deal_followup" {
		t.Errorf("requested_category = %v, want active_deal_followup", got.RequestedCategory)
	}
	if got.ResolvedCategory == nil || *got.ResolvedCategory != "requested_followup" {
		t.Errorf("resolved_category = %v, want it unchanged as requested_followup", got.ResolvedCategory)
	}
}

// An address the send would file is marked new, in any case the rep typed.
// An address a record holds is not.
func TestThePreviewMarksTheAddressTheSendWouldFile(t *testing.T) {
	t.Parallel()
	out := previewResponse(SendPreview{
		Decisions: commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
			{Recipient: connector.Recipient{Email: "New.Buyer@Example.test"}, Verdict: commsauthz.VerdictAllow},
			{Recipient: connector.Recipient{Email: "known@example.test"}, Verdict: commsauthz.VerdictAllow},
		}},
		NewContacts: map[string]bool{"new.buyer@example.test": true},
	})
	if got := out.Recipients[0].NewContact; got == nil || !*got {
		t.Errorf("new_contact for the new address = %v, want true", got)
	}
	if got := out.Recipients[1].NewContact; got != nil {
		t.Errorf("new_contact for an address on file = %v, want absent", *got)
	}
}
