// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestThePreviewOffersAReasonForEveryPolicyRefusal(t *testing.T) {
	t.Parallel()
	for _, code := range commsauthz.RefusalCodes() {
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
		want := code != commsauthz.ReasonHardBounce && code != commsauthz.ReasonNoSubject
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

// The reasons the record would carry travel with the refused recipient. An
// answer with none leaves the field out rather than sending an empty list.
func TestThePreviewCarriesTheSupportedAlternatives(t *testing.T) {
	t.Parallel()
	out := previewResponse(SendPreview{Decisions: commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{
			Verdict: commsauthz.VerdictReview, ReasonCode: commsauthz.ReasonNoEvidence, Mode: commsauthz.ModeEnforce,
			Requested:             commsauthz.CategoryActiveDealFollowup,
			SupportedAlternatives: []commsauthz.Category{commsauthz.CategoryRequestedFollowup},
		},
		{Verdict: commsauthz.VerdictReview, ReasonCode: commsauthz.ReasonNoEvidence, Mode: commsauthz.ModeEnforce},
	}}})
	offered := out.Recipients[0].SupportedAlternatives
	if offered == nil || len(*offered) != 1 || (*offered)[0] != "requested_followup" {
		t.Errorf("supported_alternatives = %v, want [requested_followup]", offered)
	}
	if none := out.Recipients[1].SupportedAlternatives; none != nil {
		t.Errorf("supported_alternatives = %v, want absent", *none)
	}
}

// The preview names refused recipients in the order the review and the 409
// do: repair, the subject's stop, the footer, missing evidence. A recipient
// the send reaches comes after every refusal.
func TestThePreviewRanksRefusedRecipientsLikeTheReview(t *testing.T) {
	t.Parallel()
	refusal := func(address, code string) commsauthz.Decision {
		return commsauthz.Decision{
			Recipient:  connector.Recipient{Email: address},
			Verdict:    commsauthz.VerdictDeny,
			ReasonCode: code,
			Mode:       commsauthz.ModeEnforce,
		}
	}
	findings := refusal("footer-finding@a.test", commsauthz.ReasonNoEvidence)
	findings.Evidence.FooterFindings = []commsauthz.FooterFinding{{Kind: "privacy"}}
	out := previewResponse(SendPreview{Decisions: commsauthz.DecisionSet{Decisions: []commsauthz.Decision{
		{Recipient: connector.Recipient{Email: "allowed@a.test"}, Verdict: commsauthz.VerdictAllow, ReasonCode: "allowed", Mode: commsauthz.ModeEnforce},
		refusal("no-evidence@a.test", commsauthz.ReasonNoEvidence),
		refusal("footer@a.test", commsauthz.ReasonFooterIncomplete),
		findings,
		refusal("objected@a.test", commsauthz.ReasonObjection),
		refusal("bounced@a.test", commsauthz.ReasonHardBounce),
	}}})
	got := make([]string, 0, len(out.Recipients))
	for _, r := range out.Recipients {
		got = append(got, string(r.Address))
	}
	want := []string{
		"bounced@a.test", "objected@a.test", "footer@a.test", "footer-finding@a.test",
		"no-evidence@a.test", "allowed@a.test",
	}
	if !slices.Equal(got, want) {
		t.Errorf("preview order %v, want %v", got, want)
	}
}
