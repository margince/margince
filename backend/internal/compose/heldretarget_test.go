// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a human may correct on a staged draft, and what they may not.
//
// ADR-0036 §4 lets an approver release a corrected version of a staged action,
// and the correction is CONTENT. The approvals edit scope enforces that by
// pinning anything shaped like a uuid — which leaves the fields that decide
// WHO the message reaches and WHAT IT IS unprotected, because both are prose.
//
// A unit test rather than an integration one: refuseRetargetedDraft is a pure
// comparison, and a database would only slow down the case it cannot make
// clearer.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// stagedDraft is the proposal an approver read, against which every edit below
// is judged.
func stagedDraft() automation.HeldDraftProposal {
	return automation.HeldDraftProposal{
		AnchorActivityID:     ids.NewV7(),
		To:                   "customer@example.test",
		Subject:              "Recap: kickoff",
		Body:                 "here is what we agreed",
		ConsentPurpose:       "business_correspondence",
		CommunicationContext: commsauthz.CategoryReplyToInbound,
	}
}

// retargetedPaths reads the paths a refusal names, failing the test when the
// edit was allowed through.
func retargetedPaths(t *testing.T, edited automation.HeldDraftProposal) []string {
	t.Helper()
	err := refuseRetargetedDraft(stagedDraft(), edited)
	if err == nil {
		t.Fatal("the edit was allowed through; want a RetargetedEditError")
	}
	var retargeted *approvals.RetargetedEditError
	if !errors.As(err, &retargeted) {
		t.Fatalf("refusal is %T (%v), want *approvals.RetargetedEditError", err, err)
	}
	return retargeted.Paths
}

// TestCorrectingTheWordsIsAllowed holds the other half of the rule. A guard
// that refused everything would pass every test below and break the feature
// ADR-0036 §4 exists for, so the permitted edit is asserted first.
func TestCorrectingTheWordsIsAllowed(t *testing.T) {
	edited := stagedDraft()
	edited.Subject = "Recap: our kickoff call"
	edited.Body = "here is what we agreed, with the dates filled in"

	if err := refuseRetargetedDraft(stagedDraft(), edited); err != nil {
		t.Fatalf("editing subject and body was refused: %v", err)
	}
}

// TestReaimingTheDraftIsRefused is the addressee half, already true before the
// context joined it. It is asserted here so a change to the refusal cannot
// quietly trade one pinned field for another.
func TestReaimingTheDraftIsRefused(t *testing.T) {
	edited := stagedDraft()
	edited.To = "someone.else@example.test"

	if got := retargetedPaths(t, edited); len(got) != 1 || got[0] != "to" {
		t.Errorf("refusal names %v, want exactly [to]", got)
	}
}

// TestRelicensingTheDraftAtReleaseIsRefused is why this file exists.
//
// The context is what the engine decides on. An approver who could edit it
// would re-license the message at the moment of sending it: a marketing send
// re-labelled as a reply reaches a lane that asks for no consent at all. It is
// the same violation the purpose has always been refused for, in a field that
// reads as a description rather than as a permission — which is exactly why
// nothing else would have caught it.
func TestRelicensingTheDraftAtReleaseIsRefused(t *testing.T) {
	edited := stagedDraft()
	edited.CommunicationContext = commsauthz.CategoryMarketing

	got := retargetedPaths(t, edited)
	if len(got) != 1 || got[0] != "communication_context" {
		t.Errorf("refusal names %v, want exactly [communication_context]", got)
	}
}

// TestEveryMovedFieldIsNamed proves the refusal reports the whole edit rather
// than stopping at the first thing it found. An approver told only that the
// addressee moved would fix that and hit the same 422 again on the context.
func TestEveryMovedFieldIsNamed(t *testing.T) {
	edited := stagedDraft()
	edited.To = "someone.else@example.test"
	edited.ConsentPurpose = "marketing_email"
	edited.CommunicationContext = commsauthz.CategoryMarketing

	got := retargetedPaths(t, edited)
	want := map[string]bool{"to": true, "consent_purpose": true, "communication_context": true}
	if len(got) != len(want) {
		t.Fatalf("refusal names %v, want all three of to, consent_purpose, communication_context", got)
	}
	for _, path := range got {
		if !want[path] {
			t.Errorf("refusal names unexpected path %q", path)
		}
	}
}

// TestTheReleasedSendCarriesWhatTheDraftClaimed is the reason the context is
// threaded at all.
//
// The engine decides on SendEmailInput.Context. A release that built the send
// without it would compile, pass every refusal test above, and reach the engine
// claiming nothing — falling through to the legacy purpose model, which is the
// model this change exists to stop depending on. Nothing else fails when that
// one assignment goes, so this asserts it directly.
func TestTheReleasedSendCarriesWhatTheDraftClaimed(t *testing.T) {
	staged := stagedDraft()

	_, in := sendFromHeldDraft(staged)

	if in.Context != commsauthz.CategoryReplyToInbound {
		t.Errorf("released send Context = %q, want %q — the engine decides on this field",
			in.Context, commsauthz.CategoryReplyToInbound)
	}
	// The purpose still travels too: a draft whose anchor no longer bears the
	// reply out reaches the legacy fallback, which reads this and nothing else.
	if in.ConsentPurpose != staged.ConsentPurpose {
		t.Errorf("released send ConsentPurpose = %q, want %q",
			in.ConsentPurpose, staged.ConsentPurpose)
	}
	if len(in.Recipients) != 1 || in.Recipients[0] != staged.To {
		t.Errorf("released send Recipients = %v, want exactly [%s]", in.Recipients, staged.To)
	}
}

// fixedDrafter is the drafting seam with the model taken out: it answers the
// same reply for any anchor, so a test about WHAT THE DRAFT CLAIMS is not also
// a test about what a model wrote.
type fixedDrafter struct {
	to string
}

func (f fixedDrafter) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	return "Re: your question", "answering the point you raised", nil
}

func (f fixedDrafter) ReplyAddress(context.Context, ids.UUID) (string, error) {
	return f.to, nil
}

// TestTheNightlyFollowUpClaimsTheReplyItIs holds the nightly pass to the same
// bar as the recap starter.
//
// The pass only ever drafts INTO an existing thread, which is the one case the
// engine can bear out on the anchor's own evidence. Claiming nothing would send
// it to the legacy purpose model instead — the model this change stops
// depending on — and nothing about the draft would look wrong until the day
// that purpose is archived and every follow-up starts being refused.
func TestTheNightlyFollowUpClaimsTheReplyItIs(t *testing.T) {
	proposal := deals.FollowUpProposal{
		DealID:             ids.New[ids.DealKind](),
		EvidenceActivityID: ids.New[ids.ActivityKind](),
		EvidenceDirection:  "inbound",
	}

	draft, ok, err := draftFollowUpReply(
		context.Background(), fixedDrafter{to: "customer@example.test"}, proposal)
	if err != nil {
		t.Fatalf("drafting the follow-up: %v", err)
	}
	if !ok {
		t.Fatal("the thread produced no draft; want one it can answer")
	}
	if draft.CommunicationContext != commsauthz.CategoryReplyToInbound {
		t.Errorf("follow-up draft context = %q, want %q",
			draft.CommunicationContext, commsauthz.CategoryReplyToInbound)
	}
	if draft.ConsentPurpose != followUpDraftPurpose {
		t.Errorf("follow-up draft purpose = %q, want %q",
			draft.ConsentPurpose, followUpDraftPurpose)
	}
}
