// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What closes a review.
//
// A refusal freezes a message into a held row and opens a review for it. The
// review ends when the message's fate is settled — and until this file existed
// only ONE of the ways that happens actually closed it, and even that one left
// the approval card behind.
//
// The two directions that arrive here:
//
//   - the message is CANCELLED, so nobody will ever send it;
//   - the message is RESUMED and staging now ALLOWS it, so it goes out on the
//     engine's own permission with no instruction to consume.
//
// The second is the one that reads like success and is not. resolveReviewTx
// closes a review when an instruction is consumed, which is the DIRECTED path,
// and staging only asks for an instruction when it actually refuses somebody. A
// resumed send the engine allows consumes nothing, so it took that path's exit
// without passing its door: the message went, and the review stayed live,
// showing a decider work about a message already in somebody's inbox. A decider
// acting on it mints an instruction for a delivery that has been made.
//
// TWO MORE DIRECTIONS SETTLE A MESSAGE AND DO NOT COME THROUGH HERE.
//
// An ERASURE closes the review in its own SQL (privacy/scheduledsends.go,
// closeReviewsForErasedMessages), because privacy may import neither this
// module nor approvals and an erasure must not depend on a seam a deployment
// could leave unwired.
//
// A RESCHEDULE closes nothing, deliberately. A moved message is still going
// out and its decision is still outstanding, and the fire path that carries it
// opens no review of its own — so closing here would leave a message refused at
// its new moment held with nothing routable in front of anybody. See
// activities' RescheduleInTx.
//
// WHY ONE FUNCTION RATHER THAN ONE PER DIRECTION. They differ only in what the
// row should SAY afterwards, and each has the same two obligations: end the
// review, and retract the card if it was handed to somebody. A call site per
// direction is a fresh chance to spell the second one wrong, and that is
// exactly the half that was missing from the one closer that existed.
//
// THE CARD ALWAYS GOES WITH THE REVIEW. A routed review names an approval card
// asking a decider to send this message. Once the review is over the card is
// asking about a message that is cancelled or already sent, and approving it
// records a second decision for one send.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReviewClosure says what ended a review and how the row should read.
//
// The state is not free text: only the three terminal states the row's own
// shape CHECK admits can be written here, and `superseded` is not one of them
// because supersession names a successor review this path never has.
type ReviewClosure struct {
	// State is the terminal state to write: ReviewResolved or ReviewCancelled.
	State string
	// Because is the sentence the audit records and the retracted card shows
	// its decider. It is why the question stopped being a question.
	Because string
}

// ClosedByCancellation is a message nobody will send.
//
// CANCELLED, NOT RESOLVED. Resolved is what a message going out leaves behind,
// and a reader asking why somebody received a message needs to find one of
// those rather than a row saying the same thing about a message nobody got.
func ClosedByCancellation() ReviewClosure {
	return ReviewClosure{State: ReviewCancelled, Because: "the message was cancelled"}
}

// ClosedBySending is the resumed message the engine allowed.
//
// RESOLVED, because this one did go out. It is the only closure of the four
// that records a delivery, and the distinction is what lets a reader tell a
// message that was sent from one that was abandoned.
func ClosedBySending() ReviewClosure {
	return ReviewClosure{
		State:   ReviewResolved,
		Because: "the message was sent once the engine allowed it",
	}
}

// CloseReviewForIntentTx ends the live review for one held message, whatever
// ended it, and retracts the card it was handed to.
//
// On the caller's transaction, so the message's fate and the review's closing
// are one fact. A review closed separately could survive a rolled-back cancel,
// which strands the message with nothing watching it.
//
// NIL ROUTER IS NOT AN ERROR. A composition with no approval surface routes
// nothing, so it has no card to retract; a composition that DOES route but
// forgot to wire the router here would silently leave cards behind, which is
// why every compose-side caller passes it rather than deciding per site.
func CloseReviewForIntentTx(
	ctx context.Context, tx pgx.Tx, intentID ids.UUID, closure ReviewClosure, router ReviewRouter,
) error {
	if intentID.IsZero() {
		return nil
	}
	if closure.State != ReviewResolved && closure.State != ReviewCancelled {
		// A caller-side bug rather than a data condition: the three terminal
		// states are a closed set and the row's CHECK would refuse anything
		// else anyway, as a constraint error from deep inside a send.
		return fmt.Errorf("consent: %q is not a state a review can be closed into", closure.State)
	}
	// LOCKED AND READ FIRST, in its own statement, and both halves of that
	// matter.
	//
	// LOCKED, because the card id is read here and acted on below. An unlocked
	// read lets a concurrent RequestDecision commit a fresh approval_id in
	// between: this closer would then move the review terminal while
	// withdrawing a card that is no longer the routed one — or none at all —
	// leaving a live card asking about a settled message, which is the exact
	// defect this function exists to prevent. The before-image would be wrong
	// too, recording a state the row had already left.
	//
	// FIRST, because the card is withdrawn before the review moves, and the
	// lock order that follows has to be the one every other path already
	// takes. See the withdrawal below.
	var reviewID ids.UUID
	var from string
	var routedTo *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id, state, approval_id
		  FROM communication_review
		 WHERE delivery_intent_id = $1 AND resolved_at IS NULL
		   FOR UPDATE`, intentID).Scan(&reviewID, &from, &routedTo)
	if errors.Is(err, pgx.ErrNoRows) {
		// No live review for this message, which is every held message nobody
		// refused and every one whose review was already answered.
		return nil
	}
	if err != nil {
		return fmt.Errorf("consent: claiming the review for this message: %w", err)
	}
	// THE CARD GOES FIRST, whichever way the review ended, and the ORDER is the
	// interesting part rather than the withdrawal itself.
	//
	// Every other path that touches both rows takes the APPROVAL lock before
	// the review's: routing stages the card and then marks the review awaiting
	// (reviewroute.go, RequestDecision), and a decline locks the approval in
	// decideInTx and reaches the review through its declined effect. Closing
	// the review first and withdrawing second would be the one path taking them
	// the other way round, and a cancel racing a decline would deadlock —
	// Postgres aborts one of them with a fault neither caller can act on.
	//
	// Withdrawing before the review row is written is also correct on its own
	// terms: if the withdrawal fails, the whole transaction rolls back and the
	// review stays live rather than ending with a card still answerable.
	if routedTo != nil && router != nil {
		if err := router.WithdrawCardTx(ctx, tx, *routedTo, closure.Because); err != nil {
			return err
		}
	}
	// The row is already locked, so this cannot lose a race with anything.
	if _, err := tx.Exec(ctx, `
		UPDATE communication_review
		   SET state = $2, resolved_at = now()
		 WHERE id = $1`, reviewID, closure.State); err != nil {
		return fmt.Errorf("consent: closing the review for this message: %w", err)
	}
	// The state it ACTUALLY moved from is the half of the audit that says what
	// changed. A review can be closed from any live state — a rep abandoning
	// their own refusal, or one they had already handed to somebody — and a
	// fixed before-image would record every one of them as the same thing.
	_, err = storekit.Audit(ctx, tx, "update", "communication_review", reviewID,
		map[string]any{fieldStatus: from},
		map[string]any{fieldStatus: closure.State, "because": closure.Because})
	return err
}
