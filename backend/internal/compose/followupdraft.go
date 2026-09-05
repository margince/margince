// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The nightly pass's SECOND answer: when the conversation it found was an
// email, propose the reply itself rather than a task to write one.
//
// A task says "follow up on this". A drafted reply says it in words the rep
// reads and sends. Only the second is something a person can finish in the
// morning inbox, and only an email thread makes it possible — a call or a
// meeting has no thread to answer and no address to answer it at, so those
// keep the task proposal.
//
// WHY held_draft AND NOT A NEW KIND. held_draft is already the drafted-email
// approval: its release effect prepares and sends the message, its precheck
// re-runs that preparation so a withdrawn consent surfaces while the row is
// still pending, and refuseRetargetedDraft pins the addressee against the edit
// path. A second kind here would be a second send path, a second consent call
// and a second card — two answers to the one question "may this message go
// out", drifting until they disagree in front of a rep.
//
// The one thing held_draft carries that this caller has no use for is the
// workflow run: releaseHeldDraft completes the run that parked the approval.
// That completion is a conditional UPDATE matching on the approval id, so with
// no parked run it touches nothing and returns nil. Nothing about the release
// assumes an automation raised the draft.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// followUpDraftPurpose is the lawful basis a drafted reply is sent under when
// the engine cannot bear the reply out on the thread's own evidence.
//
// Answering somebody who wrote to us individually rests on contract or
// legitimate interest, not on consent — the consent purpose whose German
// evidence standard applies belongs to marketing. The nightly pass only ever
// drafts a reply INTO an existing thread, which is exactly that case.
const followUpDraftPurpose = "business_correspondence"

// approvalTargetDeal is the target type every deal-scoped staging names.
//
// Three stagers write it — the close-date corrector, the follow-up task and
// the drafted reply — and a target type that disagreed between them would file
// a proposal against a record the inbox then cannot resolve. It is the
// approvals target vocabulary, not the commission entity vocabulary that
// happens to spell the same word.
//
// Held by: TestEveryStagingNamesItsDealTargetThroughOneConstant
// (backend/gates/dealtargettype_test.go)
const approvalTargetDeal = "deal"

// followUpReplySeam composes the reply and resolves who it answers.
//
// It is the automation module's Comms seam, filled by the same adapter the
// workflow executors use, so a drafted follow-up and an automation's draft are
// one drafting engine rather than two. With no model configured the adapter
// falls back to a deterministic draft, which is what a nightly pass wants: the
// rep still gets a reply to edit rather than an empty card.
type followUpReplySeam interface {
	DraftEmail(ctx context.Context, anchor ids.UUID, intent string) (subject, body string, err error)
	ReplyAddress(ctx context.Context, anchor ids.UUID) (string, error)
}

// draftFollowUpReply builds the held-draft proposal for one email-evidenced
// follow-up, or reports that this conversation cannot be answered.
//
// A thread with no resolvable counterparty is the honest failure here, and it
// is not an error: an internal note, a message whose sender never became a
// person, or a thread the pass may read but whose address it may not. The
// caller falls back to the task proposal, so the rep is still told about the
// deal — they simply get "write a follow-up" instead of a draft to send.
func draftFollowUpReply(
	ctx context.Context, drafter followUpReplySeam, proposal deals.FollowUpProposal,
) (automation.HeldDraftProposal, bool, error) {
	anchor := proposal.EvidenceActivityID.UUID
	to, err := drafter.ReplyAddress(ctx, anchor)
	switch {
	case errors.As(err, new(*activities.NoReplyAddressError)), err == nil && to == "":
		// The ONE case the task proposal is the answer to: this thread carries
		// no counterparty. Anything else — a denied read, a row-scope miss, a
		// database failure — is a real failure and must not be reported as a
		// nightly pass that quietly chose the other proposal. That reading hid
		// the failure and never retried the draft.
		//
		// A denial stays a failure HERE on purpose, even though the caller
		// treats one as a fallback. The two are different questions. This
		// function is handed an authority and asked to draft under it, so a
		// refusal means the authority did not match the work — a defect worth
		// surfacing. The caller knows something this cannot: that the authority
		// is the deal owner's, and that an owner who lacks the grant is a
		// settled fact about the workspace rather than a fault to retry.
		return automation.HeldDraftProposal{}, false, nil
	case err != nil:
		return automation.HeldDraftProposal{}, false,
			fmt.Errorf("compose: resolve who the follow-up answers: %w", err)
	}
	// The intent is EMPTY, and that is the whole care here.
	//
	// DeterministicEmailDraft appends the intent to the body verbatim, so
	// whatever is passed becomes a sentence in a message a rep sends to a
	// customer. The obvious "Follow up on <deal>" label reads as a machine
	// note dropped into the middle of a German email, and the deal name is
	// our internal word for the account, not theirs. The floor already writes
	// a complete reply from the thread it is answering; there is nothing this
	// caller knows to add that a recipient should read.
	subject, body, err := drafter.DraftEmail(ctx, anchor, "")
	if err != nil {
		return automation.HeldDraftProposal{}, false, fmt.Errorf("compose: draft the follow-up reply: %w", err)
	}
	return automation.HeldDraftProposal{
		AnchorActivityID: anchor,
		To:               to,
		Subject:          subject,
		Body:             body,
		ConsentPurpose:   followUpDraftPurpose,
		// The nightly pass only ever drafts INTO an existing thread, which is
		// the reply the engine can bear out on the anchor's own evidence.
		CommunicationContext: commsauthz.CategoryReplyToInbound,
		// Intent is the rep-facing note on the card, never body text.
		Intent: "no next step was planned after this message",
	}, true, nil
}

// stageFollowUpDraft stages the drafted reply under held_draft, remembering a
// rejection the way the task proposal does.
//
// The identity is the deal and the interaction the draft answers — the same
// two fields the task proposal is keyed on, for the same reason. Keyed on the
// deal alone, one "no" would bury every later follow-up on it; keyed on the
// message text, tomorrow's reworded draft would read as a new question the rep
// had already answered.
//
// The anchor is the TARGET as well as an identity field. held_draft's release
// reads the anchor from the payload rather than from the target, and the
// version pin waiver in approvals covers exactly that: the target is context
// here, and the anchor's liveness is the send path's own check.
func stageFollowUpDraft(
	ctx context.Context, svc *approvals.Service, summary string,
	dealID ids.UUID, evidence ids.UUID, draft automation.HeldDraftProposal,
) error {
	raw, err := json.Marshal(draft)
	if err != nil {
		return fmt.Errorf("compose: marshal the drafted follow-up: %w", err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		return fmt.Errorf("compose: canonicalize the drafted follow-up: %w", err)
	}
	// anchor_activity_id is the identity field that appears in the payload;
	// deal_id does not, so it cannot be one — canonicalIdentity requires every
	// identity field to be present in the proposed change with the same value.
	// The deal is carried by the approval's own target instead, and the anchor
	// is what actually separates one conversation from the next.
	identity, err := json.Marshal(map[string]string{"anchor_activity_id": evidence.String()})
	if err != nil {
		return fmt.Errorf("compose: marshal the drafted follow-up identity: %w", err)
	}
	_, _, err = svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           automation.HeldDraftKind,
		ProposedChange: canonical,
		DiffHash:       hash,
		TargetType:     approvalTargetDeal,
		TargetID:       dealID,
		Summary:        summary,
		Identity:       identity,
		JoinPending:    true,
	})
	return err
}
