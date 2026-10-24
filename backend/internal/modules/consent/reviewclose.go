// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What closes a review.
//
// A refusal freezes a message into a held row and opens a review for it. The
// review ends when the message's fate is settled.
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
// A reschedule closes nothing. A moved message is still going out and its
// decision is still outstanding. The fire path that carries it opens no review
// of its own. Closing here would leave a message that is refused at its new
// moment held with no review open. See activities' RescheduleInTx.
//
// One function serves every direction. They differ only in what the row says
// afterwards, and a call site per direction is a fresh chance to spell the
// closing wrong.

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
	// Because is the sentence the audit records: why the review ended.
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
// ended it.
//
// On the caller's transaction, so the message's fate and the review's closing
// are one fact. A review closed separately could survive a rolled-back cancel,
// which strands the message with nothing watching it.
func CloseReviewForIntentTx(
	ctx context.Context, tx pgx.Tx, intentID ids.UUID, closure ReviewClosure,
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
	// Locked, so the before-image the audit records is the state the row
	// actually leaves.
	var reviewID ids.UUID
	var from string
	err := tx.QueryRow(ctx, `
		SELECT id, state
		  FROM communication_review
		 WHERE delivery_intent_id = $1 AND resolved_at IS NULL
		   FOR UPDATE`, intentID).Scan(&reviewID, &from)
	if errors.Is(err, pgx.ErrNoRows) {
		// No live review for this message, which is every held message nobody
		// refused and every one whose review was already answered.
		return nil
	}
	if err != nil {
		return fmt.Errorf("consent: claiming the review for this message: %w", err)
	}
	// The row is already locked, so this cannot lose a race with anything.
	if _, err := tx.Exec(ctx, `
		UPDATE communication_review
		   SET state = $2, resolved_at = now()
		 WHERE id = $1`, reviewID, closure.State); err != nil {
		return fmt.Errorf("consent: closing the review for this message: %w", err)
	}
	// The state it ACTUALLY moved from is the half of the audit that says what
	// changed. A review can be closed from any live state, and a fixed
	// before-image would record every one of them as the same thing.
	_, err = storekit.Audit(ctx, tx, "update", "communication_review", reviewID,
		map[string]any{fieldStatus: from},
		map[string]any{fieldStatus: closure.State, "because": closure.Because})
	return err
}
