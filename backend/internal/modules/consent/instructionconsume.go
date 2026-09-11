// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Spending a recorded decision on one message.
//
// instruction.go writes the decision: a named human read what the engine
// refused, took responsibility in writing, and said send it anyway. Until now
// nothing could act on that row. This is the act.
//
// WHAT CONSUMPTION HAS TO BE SURE OF, and each of these is a different way the
// record could end up describing something that did not happen:
//
//   THE DECISION IS STILL LIVE. Revoked, expired or already spent are three
//   different reasons the same row authorizes nothing now.
//
//   IT IS THIS MESSAGE. An instruction is given against one review, and that
//   review holds one message. Consuming it for a different message would send
//   something the human never read under their name.
//
//   THE MESSAGE HAS NOT CHANGED. The human acknowledged a warning about a
//   specific subject and body. A message edited after that is one nobody
//   signed for, and the fingerprint is what tells the two apart.
//
//   IT IS SPENT EXACTLY ONCE. The row moves to consumed and names its delivery
//   inside the same transaction that stages the message, so a retry, a double
//   click or a redelivered job cannot spend it twice.
//
// THE REFUSAL IS NEVER TOUCHED. No suppression is lifted, no consent is
// written, and the decision rows still read `deny`. What changes is one column
// saying the message went out under a human's instruction rather than under
// the engine's permission.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ExecutionAuthority names why a message was allowed to leave.
const (
	// AuthoritySupported is the engine's own permission, and the only authority
	// that existed before directed sends.
	AuthoritySupported = "supported"
	// AuthorityInstruction is a named human's recorded decision, standing over
	// a refusal that is still on record as a refusal.
	AuthorityInstruction = "instruction"
)

// LiveInstruction is a decision that can still be spent, read under the lock
// that will spend it.
type LiveInstruction struct {
	ID       ids.UUID
	ReviewID ids.UUID
	IntentID ids.UUID
	// Acknowledged fingerprints the message the human read when they decided.
	// Empty on a decision written before this was recorded, and an absent
	// fingerprint is nothing to disagree with rather than a mismatch.
	Acknowledged []byte
}

// InstructionStaleError refuses an instruction that no longer describes the
// message in hand.
//
// SEPARATE FROM "no instruction", because the two need different answers. No
// instruction means the ordinary refusal stands and the rep should be told
// their message was refused. A stale one means somebody DID decide, and the
// message has moved underneath their decision — so the honest answer names what
// changed and asks them to look again rather than repeating a refusal they have
// already answered.
type InstructionStaleError struct {
	Why string
}

func (e *InstructionStaleError) Error() string {
	return "the decision to send this message no longer describes it: " + e.Why
}

// FieldFault carries the refusal to every surface rather than the HTTP one
// alone, and names the acknowledgement — because looking again at the message
// and acknowledging afresh is the action that clears it.
func (e *InstructionStaleError) FieldFault() (field, code, message string) {
	return "acknowledged", "instruction_stale", e.Error()
}

// liveInstructionForIntentTx finds the decision standing over this held
// message, and locks it so nothing else can spend it.
//
// FOR UPDATE, taken before anything is written. Two workers reaching the same
// directed send — a double click, a redelivered job — both arrive here; one
// waits, and by the time it reads the row the winner has already marked it
// consumed, so its own read finds nothing live and it does not send.
//
// A zero id means no live decision, which is the ordinary case for every
// message in the installation and is not an error.
func liveInstructionForIntentTx(ctx context.Context, tx pgx.Tx, intentID ids.UUID, now time.Time) (LiveInstruction, error) {
	var out LiveInstruction
	err := tx.QueryRow(ctx, `
		SELECT i.id, i.review_id, r.delivery_intent_id, i.acknowledged_wording
		  FROM communication_instruction i
		  JOIN communication_review r ON r.id = i.review_id
		 WHERE r.delivery_intent_id = $1
		   AND i.status = 'directed'
		   AND i.valid_until > $2
		 ORDER BY i.directed_at DESC
		 LIMIT 1
		   FOR UPDATE OF i`, intentID, now).
		Scan(&out.ID, &out.ReviewID, &out.IntentID, &out.Acknowledged)
	if errors.Is(err, pgx.ErrNoRows) {
		return LiveInstruction{}, nil
	}
	if err != nil {
		return LiveInstruction{}, fmt.Errorf("consent: reading the decision standing over this message: %w", err)
	}
	return out, nil
}

// consumeInstructionTx spends the decision on one delivery.
//
// The fingerprint check is here rather than at the door because this is the
// last moment before the message is handed on, and it runs on the SAME
// transaction that stages it: a body edited between the check and the send
// cannot slip between the two.
func consumeInstructionTx(
	ctx context.Context, tx pgx.Tx, inst LiveInstruction, deliveryID ids.UUID, wording [32]byte,
	withdraw ReviewRouter,
) error {
	// WHAT THE HUMAN ACKNOWLEDGED, read from the INSTRUCTION rather than from
	// the held row it was given against.
	//
	// THE HELD ROW WOULD BE THE WRONG THING TO COMPARE AGAINST, and this is the
	// whole reason the fingerprint is frozen on the decision. A message edited
	// after somebody decided to send it is exactly the case this check exists
	// for — and the edit lands on that held row, so comparing the message with
	// it would compare the message with itself and find nothing changed.
	//
	// AN ABSENT FINGERPRINT IS NOTHING TO DISAGREE WITH. A decision written
	// before this column existed, or against a review whose held message could
	// not be read, records none; refusing those would park a send for a reason
	// nobody can act on.
	if len(inst.Acknowledged) > 0 && !bytes.Equal(inst.Acknowledged, wording[:]) {
		return &InstructionStaleError{
			Why: "the message has been edited since it was acknowledged",
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE communication_instruction
		   SET status = 'consumed', consumed_at = now(), delivery_id = $2
		 WHERE id = $1 AND status = 'directed'`, inst.ID, deliveryID)
	if err != nil {
		return fmt.Errorf("consent: spending the decision on this message: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// The row moved between the locked read and here, which the lock is
		// supposed to prevent. Refusing is the safe direction: a send that
		// cannot record its own authority must not go.
		return fmt.Errorf(
			"consent: the decision was no longer live when the message was staged: %w",
			apperrors.ErrConflict)
	}
	// THE REVIEW IS FINISHED, because the message it was about has gone.
	//
	// Without this the review stays live forever: the rep's queue keeps showing
	// work that is done, the message can be routed to a decider who would be
	// asked about a closed matter, and the live-intent index goes on treating a
	// spent message as one still waiting.
	//
	// Resolved rather than deleted. The review is what a subject asking "why
	// did I receive this" is shown beside the decision, and a queue that
	// emptied itself by forgetting would answer nothing.
	if err := resolveReviewTx(ctx, tx, inst.ReviewID, withdraw); err != nil {
		return err
	}
	// AUDITED AS AN UPDATE WITH BOTH IMAGES, because that is what it is: the
	// row moves from a live decision to a spent one, and the status column
	// afterwards cannot say what it moved from.
	//
	// It is the transition that answers the question a dispute asks — was this
	// override ever exercised — so an after-image alone would record that the
	// decision is spent without recording that it was ever available to spend.
	if _, err := storekit.Audit(ctx, tx, "update", "communication_instruction", inst.ID,
		map[string]any{fieldStatus: InstructionDirected},
		map[string]any{
			fieldStatus:   InstructionConsumed,
			"delivery_id": deliveryID,
			"review_id":   inst.ReviewID,
		}); err != nil {
		return err
	}
	return nil
}

// acknowledgedWordingTx answers the fingerprint of the message this review
// holds — what the human was looking at when they acknowledged.
//
// NOT FOUND IS NOT A FAULT, and this is the one place that judgement matters.
// A review whose held message has been cancelled or emptied — by an erasure, or
// by a rep who resumed it another way — has no wording to compare. Inventing a
// mismatch there would refuse a send for a reason nobody can act on; the
// caller treats an absent fingerprint as nothing to disagree with, exactly as
// the staging-to-transmit check does (authorizecontent.go).
func acknowledgedWordingTx(ctx context.Context, tx pgx.Tx, intentID ids.UUID) ([32]byte, bool, error) {
	if intentID.IsZero() {
		return [32]byte{}, false, nil
	}
	var subject, body, htmlBody string
	err := tx.QueryRow(ctx, `
		SELECT coalesce(payload->>'subject', ''),
		       coalesce(payload->>'body', ''),
		       coalesce(payload->>'html_body', '')
		  FROM scheduled_send WHERE id = $1`, intentID).Scan(&subject, &body, &htmlBody)
	if errors.Is(err, pgx.ErrNoRows) {
		return [32]byte{}, false, nil
	}
	if err != nil {
		return [32]byte{}, false, fmt.Errorf("consent: reading the message that was acknowledged: %w", err)
	}
	return SendingDigest(subject, body, htmlBody), true, nil
}

// AuthorizeDirectedExecutionTx asks whether a refused message may go out anyway
// on a recorded decision, and spends that decision if it may.
//
// Called by the staging path at the one moment a refusal becomes final, on the
// SAME transaction that stages the delivery — so the message going out and the
// decision being spent are one fact. A crash between them is not possible
// because there is no between.
//
// THREE ANSWERS, and the caller needs to tell them apart:
//
//	(zero, nil)     nothing decided this; the refusal stands as it always did
//	(id,   nil)     a named human's decision has been spent on this delivery
//	(zero, err)     somebody decided and the decision no longer fits the message
//
// The middle answer is the only one that sends, and it leaves the engine's
// refusal exactly where it was: no suppression lifted, no consent written, the
// decision rows still reading deny.
func (g *Gate) AuthorizeDirectedExecutionTx(
	ctx context.Context, tx pgx.Tx, intentID, deliveryID ids.UUID, wording [32]byte,
) (ids.UUID, error) {
	if intentID.IsZero() {
		// Not a resumed message, so there is no review and no decision. Every
		// ordinary send lands here.
		return ids.UUID{}, nil
	}
	inst, err := liveInstructionForIntentTx(ctx, tx, intentID, time.Now())
	if err != nil {
		return ids.UUID{}, err
	}
	if inst.ID.IsZero() {
		// A held message being resumed with nobody having decided anything —
		// a rep pressing Resume on their own refusal, say. The engine refuses
		// it again, which is the right answer.
		return ids.UUID{}, nil
	}
	if err := consumeInstructionTx(ctx, tx, inst, deliveryID, wording, g.store.reviewRouter); err != nil {
		return ids.UUID{}, err
	}
	return inst.ID, nil
}

// HeldMessageForReview answers which held message a review is about.
//
// NOT SCOPED TO THE INITIATOR, unlike ReviewForInitiator beside it, and the
// difference is the whole point of a reviewer path: directing a send is
// precisely the act of somebody OTHER than the sender deciding. Scoping this to
// the seat that pressed Send would make every review directable only by the
// human who was already refused — which is the one human the design does not
// rely on.
//
// WHAT IT DISCLOSES IS AN ID AND NOTHING ELSE. No address, no subject, no
// reason: a caller learns that this review holds a message, which is what they
// need to act on it and is not a fact about the recipients. The refusals
// themselves stay behind ReviewForInitiator.
//
// GATED ON THE SAME GRANT AS DIRECTING, because this read exists to serve that
// act and no other. A seat that may not direct a send has no use for it.
func (s *Store) HeldMessageForReview(ctx context.Context, reviewID ids.UUID) (ids.UUID, error) {
	// THE SAME DOOR DirectSend USES, not the weaker one beside it.
	// auth.RequireHuman refuses buyers and agents but ADMITS connectors and the
	// system principal, and a connector runs with the granting human's own
	// grants — so it would hold communication_exception:create and could learn
	// which reviews exist before the stricter check on the write ever ran.
	//
	// This read serves directing and nothing else; it answers to the same
	// human the write does.
	if err := requireAHumanAtTheKeyboard(ctx); err != nil {
		return ids.UUID{}, err
	}
	// CREATE here, not read: this lookup exists to serve DIRECTING a send, and
	// it answers to the same grant that act does. Reading the review is its own
	// door (ReviewForReader) and takes the read verb.
	if err := auth.Require(ctx, entityCommunicationException, principal.ActionCreate); err != nil {
		return ids.UUID{}, err
	}
	var intent ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT coalesce(delivery_intent_id, '00000000-0000-0000-0000-000000000000'::uuid)
			  FROM communication_review
			 WHERE id = $1 AND resolved_at IS NULL`, reviewID).Scan(&intent)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("consent: reading the message this review holds: %w", err)
		}
		return nil
	})
	return intent, err
}

// resolveReviewTx closes a review whose message has gone.
//
// The state and the moment move together, which the row's own shape CHECK
// requires: a resolved review that named no moment, or a live one that claimed
// one, would each be a row the database refuses.
func resolveReviewTx(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, router ReviewRouter) error {
	if reviewID.IsZero() {
		return nil
	}
	var routedTo *ids.UUID
	err := tx.QueryRow(ctx, `
		UPDATE communication_review
		   SET state = 'resolved', resolved_at = now()
		 WHERE id = $1 AND resolved_at IS NULL
		RETURNING approval_id`, reviewID).Scan(&routedTo)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already closed. Nothing to retract either: whoever closed it first
		// took the card with them.
		return nil
	}
	if err != nil {
		return fmt.Errorf("consent: closing the review this message answered: %w", err)
	}
	// THE CARD GOES WITH IT. A review can be routed and then directed from the
	// review itself — by the same rep once they are granted the authority, or
	// by a colleague reading it. The card is then asking about a message that
	// has already gone, and approving it would put a second decision on the
	// record for one send.
	if routedTo == nil || router == nil {
		return nil
	}
	return router.WithdrawCardTx(ctx, tx, *routedTo,
		"the message this asked about has already been sent")
}
