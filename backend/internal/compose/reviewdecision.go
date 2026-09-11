// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Handing a refused send to somebody who may decide it.
//
// consent owns the review and what routing means for it. approvals owns the
// card and who may answer one. Neither may import the other, so the seam is
// declared by consent (ReviewRouter) and bound here.
//
// WHAT THE CARD CARRIES is deliberately thin: the review's id, the held
// message's id, the reason and how many people it names. Not the addresses —
// an approvals queue is read by whoever holds the grant, and a card is a worse
// place for somebody's contact details than the review it points at, which is
// scoped to the people who may see it.
//
// WHO ANSWERS IT is the approvals engine's question, and the grant it checks is
// communication_exception:create — the same one the direct-send door takes. A
// card that could be approved by somebody who may not direct a send would be a
// way around that door, and the effect would refuse after the decision had
// already committed.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reviewRouter stages a refused send as a decision somebody else can take.
type reviewRouter struct {
	approvals *approvals.Service
}

// reviewDecision is the card's payload: what a decider is answering about.
//
// It is also what the effect reads back, which is what keeps the card and the
// send about one message. The review id alone would be enough to find
// everything else; the intent is carried too so the effect does not have to
// trust a second read to still agree with what was staged.
type reviewDecision struct {
	ReviewID   string `json:"review_id"`
	IntentID   string `json:"intent_id"`
	ReasonCode string `json:"reason_code"`
	Recipients int    `json:"recipients"`
	Note       string `json:"note,omitempty"`
}

// RouteForDecisionTx implements consent.ReviewRouter.
func (r reviewRouter) RouteForDecisionTx(
	ctx context.Context, tx pgx.Tx, in consent.ReviewRouteRequest,
) (ids.UUID, error) {
	payload, err := json.Marshal(reviewDecision{
		ReviewID:   in.ReviewID.String(),
		IntentID:   in.IntentID.String(),
		ReasonCode: in.ReasonCode,
		Recipients: in.Recipients,
		Note:       in.Note,
	})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: describing the decision being asked for: %w", err)
	}
	// IDENTITY IS THE REVIEW ALONE, and it is a SUBSET of the payload rather
	// than the payload itself.
	//
	// A rep who presses the button twice is asking one question about one
	// refused message, so the second press must find the first card. Identity
	// is what decides that — and if it carried the whole payload, two presses
	// with different notes would be two identities and the decider would be
	// shown the same refusal twice.
	identity, err := json.Marshal(map[string]string{consent.FieldReviewID: in.ReviewID.String()})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: naming the question being asked: %w", err)
	}
	staged, _, err := r.approvals.StageUnlessDeclinedTx(ctx, tx, approvals.StageInput{
		Kind:           approvals.KindCommunicationReview,
		ProposedChange: payload,
		Identity:       identity,
		// The DIFF HASH is the review too, not the message. A card is
		// superseded when the question changes, and the question here is "may
		// this refused send go" — which is the same question however many
		// times it is asked about one review.
		DiffHash:    in.ReviewID.String(),
		JoinPending: true,
		Summary:     reviewSummary(in),
	})
	if err != nil {
		return ids.UUID{}, err
	}
	return staged.UUID, nil
}

// reviewSummary is the line a decider reads in the queue before opening
// anything. It says what was refused and how widely, and names no recipient.
func reviewSummary(in consent.ReviewRouteRequest) string {
	people := "1 recipient"
	if in.Recipients != 1 {
		people = fmt.Sprintf("%d recipients", in.Recipients)
	}
	return fmt.Sprintf("A send was refused for %s (%s) and somebody is asking whether it may go anyway",
		people, in.ReasonCode)
}

// reviewDecisionEffect is what approving the card does: the directed send
// itself, under the APPROVER's principal.
//
// THE APPROVER IS THE DIRECTOR. That is the whole design — the instruction
// records who decided, and the person who decided is the one who pressed
// approve, not the rep who asked. An effect that ran under the initiator's
// identity would put the rep's name on an override they were not entitled to
// make.
func reviewDecisionEffect(svc *approvals.Service, directed directedSendService) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		// REDEEMED FIRST, and not inside the send's own transaction.
		//
		// An effect that does not consume its approval leaves consumed_at null,
		// so the next decision on that row re-drives it and the message goes
		// twice. RedeemAndApply would own the transaction, and the directed
		// send opens its own — it has to, because it records a decision and
		// then resumes a held message through the ordinary send path.
		//
		// Redeeming BEFORE is safe here in a way it would not be for most
		// effects, and the reason is the instruction itself: it is spend-once
		// at the database, so a redemption that commits and a send that then
		// fails leaves a standing decision the retry reuses rather than a
		// second message. The failure mode redemption exists to prevent —
		// one approval driving two writes — is closed twice over.
		decision, err := decodeReviewDecision(proposedChange)
		if err != nil {
			return err
		}
		reviewID, err := ids.Parse(decision.ReviewID)
		if err != nil {
			return fmt.Errorf("compose: the card names no readable review: %w", err)
		}
		if _, _, err := svc.Redeem(ctx, approvalID, approvals.KindCommunicationReview, diffHash); err != nil {
			return err
		}
		_, err = directed.DirectAndSend(ctx, reviewID, consent.DirectInput{
			ReasonCode: decision.recordedReason(),
			// The asker's note is what the approver was shown, so it is what
			// the record says they acted on. A blank one still needs words:
			// the instruction refuses an empty explanation, and "approved from
			// the queue" is the truthful minimum.
			Explanation:    decision.recordedExplanation(),
			WarningVersion: consent.QueueCardWarningVersion,
			// Approving IS the acknowledgement. The card said what this is and
			// the approver pressed approve; requiring a second tick inside the
			// effect would be asking them to confirm the thing they just did.
			Acknowledged: true,
		})
		return err
	}
}

// decodeReviewDecision reads a staged card, refusing one carrying anything this
// effect does not act on.
//
// Strictly, for heldDraftReleaseEffect's reason: a human may edit a staged
// payload, and an unknown field would then be recorded as approved and ignored
// on execution — what the audit says a person approved would not be what
// happened.
func decodeReviewDecision(raw json.RawMessage) (reviewDecision, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var out reviewDecision
	if err := decoder.Decode(&out); err != nil {
		return reviewDecision{}, fmt.Errorf("compose: reading the decision being approved: %w", err)
	}
	if out.ReviewID == "" {
		return reviewDecision{}, fmt.Errorf("compose: the card names no review")
	}
	return out, nil
}

// recordedReason is the reason the instruction records for a queue decision.
//
// ALWAYS `other`, and deliberately so. The closed list on the instruction is
// about the LEGAL ground an override rests on — a contract clause, a statutory
// obligation — and a card carries no such claim: what it carries is the
// engine's refusal code, which says what was wrong rather than why sending
// anyway is justified. Mapping one onto the other would put a legal basis on
// the record that nobody asserted.
//
// The words the asker wrote travel in the explanation instead, which is where a
// reader looks for why.
func (d reviewDecision) recordedReason() string { return "other" }

// recordedExplanation is what the record says this decision rested on.
//
// The asker's note when there is one, because that is what the approver read
// and acted on. When there is none the record still has to say something — the
// instruction refuses a blank explanation, and a card approved with no words is
// exactly that: somebody saw the refusal in the queue and released it.
func (d reviewDecision) recordedExplanation() string {
	if d.Note != "" {
		return d.Note
	}
	return "Approved from the review queue; the asker gave no further reason."
}

// WithdrawCardTx implements consent.ReviewRouter.
//
// Forced expiry rather than deletion: the card becomes terminal and says why,
// so a decider who had it open learns the question was answered elsewhere
// instead of finding an empty row where their work was.
func (r reviewRouter) WithdrawCardTx(
	ctx context.Context, tx pgx.Tx, approvalID ids.UUID, reason string,
) error {
	// The boolean says whether this withdrawal was the one that closed it. A
	// false is a card somebody had already decided, which is not a failure —
	// the question is answered either way, which is the whole point of
	// retracting it.
	if _, err := r.approvals.WithdrawInTx(ctx, tx, ids.ApprovalID{UUID: approvalID}, reason); err != nil {
		return fmt.Errorf("compose: retracting the card whose question has been answered: %w", err)
	}
	return nil
}

// reviewDecisionDeclined is what saying no does: the review goes back to the
// person who asked.
//
// NOT resolved, and not left waiting. The message is still held and still
// refused — what ended is the asking. The rep can route it to somebody else,
// add the evidence the engine wanted, or give up.
func reviewDecisionDeclined() approvals.DeclinedEffect {
	return func(ctx context.Context, tx pgx.Tx, _ ids.ApprovalID, proposedChange json.RawMessage) error {
		return returnRoutedReview(ctx, tx, proposedChange, "a reviewer declined to send it")
	}
}

// reviewDecisionExpired is the same move for a card nobody answered in time.
// A rep waiting on an answer that is never coming is the state this prevents.
func reviewDecisionExpired() approvals.ExpiredEffect {
	return func(ctx context.Context, tx pgx.Tx, _ ids.ApprovalID, proposedChange json.RawMessage) error {
		return returnRoutedReview(ctx, tx, proposedChange, "nobody decided it before the card expired")
	}
}

// returnRoutedReview is the shared body a decline and an expiry both run, so
// there is one answer to "nobody is going to decide this" rather than two.
func returnRoutedReview(ctx context.Context, tx pgx.Tx, proposedChange json.RawMessage, why string) error {
	decision, err := decodeReviewDecision(proposedChange)
	if err != nil {
		return err
	}
	reviewID, err := ids.Parse(decision.ReviewID)
	if err != nil {
		return fmt.Errorf("compose: the card names no readable review: %w", err)
	}
	return consent.ReturnToAskerTx(ctx, tx, reviewID, why)
}

// retractionRouter is a router built for RETRACTION ALONE, on an approvals
// service with no effects registered.
//
// WHY NOT approvalsServiceWithEffects, which every other router here uses. That
// constructor registers the held-message effects, and registering those builds
// a send store — which is the store this router is being wired INTO. The two
// constructions call each other and the process overflows its stack before it
// finishes booting, which is how this was found: a stack overflow inside
// newCounterpartyStore, thirty frames from anything about reviews.
//
// A BARE SERVICE IS THE RIGHT ANSWER RATHER THAN A WORKAROUND, because
// WithdrawInTx runs no effect. It locks the row, moves it to a terminal status
// and records why. Effects are what an APPROVAL does; a retraction is the
// question being taken off the table, and there is nothing to run.
//
// IT MUST NEVER BE USED FOR ROUTING, which does need the full service: a card
// staged through this one would be answered by a service that cannot run what
// approving it means, so the decider would press approve and nothing would
// send. Nothing routes through it today — reviewCloser's only call is
// WithdrawCardTx — and the type is unexported and built at exactly the two
// wiring sites below, so widening it is a deliberate edit rather than an
// accident.
func retractionRouter(pool *pgxpool.Pool) reviewRouter {
	return reviewRouter{approvals: approvals.NewService(InstallationDB(pool))}
}

// reviewCloser closes the review a settled message leaves behind.
//
// A seam rather than a direct call because activities may not import consent.
// It carries the ROUTER, which is the half the old spelling was missing: a
// review handed to a decider names an approval card, and ending the review
// without retracting that card leaves somebody being asked to send a message
// that is cancelled, moved, erased or already delivered.
type reviewCloser struct {
	router consent.ReviewRouter
}

// CloseReviewForIntentTx implements activities.ReviewCloser.
//
// THE TRANSLATION LIVES HERE, at the seam, because each side owns half of it.
// activities knows what happened to the message and may not name consent's
// states; consent knows what the review row should say and has never heard of a
// scheduled send. Compose is the one place that has both.
func (c reviewCloser) CloseReviewForIntentTx(
	ctx context.Context, tx pgx.Tx, intentID ids.UUID, outcome activities.ReviewOutcome,
) error {
	closure, known := reviewClosureFor(outcome)
	if !known {
		// A vocabulary this build does not understand, which can only mean
		// activities grew an outcome and this map was not extended. Refusing is
		// the safe direction: closing it as something else would write the
		// wrong account of why a message stopped, and
		// TestEveryReviewOutcomeHasAClosure keeps it from reaching a build.
		return fmt.Errorf(
			"compose: %q is not an outcome this build knows how to close a review for", outcome)
	}
	return consent.CloseReviewForIntentTx(ctx, tx, intentID, closure, c.router)
}

// reviewClosureFor maps one module's word for what happened onto the other's
// word for what the row should say.
func reviewClosureFor(outcome activities.ReviewOutcome) (consent.ReviewClosure, bool) {
	switch outcome {
	case activities.ReviewOutcomeCancelled:
		return consent.ClosedByCancellation(), true
	case activities.ReviewOutcomeSent:
		return consent.ClosedBySending(), true
	}
	return consent.ReviewClosure{}, false
}

// reviewLookup answers which review stands over a held message, so a scheduled
// row can offer a route to the work that would unstop it.
//
// A seam rather than a direct call because activities may not import consent.
// It carries the pool rather than a store because the read is one statement and
// needs nothing else.
type reviewLookup struct {
	pool *pgxpool.Pool
}

// LiveReviewsForIntents implements activities.ReviewLookup.
func (l reviewLookup) LiveReviewsForIntents(
	ctx context.Context, intents []ids.UUID,
) (map[ids.UUID]ids.UUID, error) {
	return consent.NewStore(InstallationDB(l.pool)).LiveReviewsForIntents(ctx, intents)
}
