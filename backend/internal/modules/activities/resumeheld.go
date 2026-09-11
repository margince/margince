// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Firing a message that was stopped and has since been decided about.
//
// A send the consent engine refused is frozen into a held scheduled_send
// (holdforreview.go) so the review recording that refusal has something a
// human can act on. This is the act: the held row is claimed, the message it
// froze is replayed through the ordinary send, and the consent engine is asked
// again.
//
// THE ENGINE IS ASKED AGAIN, AND IT WILL REFUSE AGAIN. That is the point. The
// refusal is real and stays on record; what lets the message through is a
// decision a named human already recorded against this message's review, which
// the staging path finds and spends. Nothing here overrules anything — this
// code does not know whether an instruction exists, and must not, or the
// authority question would be answered in two places.
//
// SO A RESUME WITH NO DECISION BEHIND IT IS REFUSED, exactly as the first
// attempt was. A rep can press Resume on their own refusal and will be told the
// same thing they were told before, which is the honest answer.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ResumeHeldSend fires the message a held row froze.
//
// TWO STEPS, AND THE ORDER IS FORCED.
//
// PrepareSend reads through the STORE rather than through a caller's
// transaction, so running it inside one takes a SECOND pool connection while
// the first is still held. Enough concurrent resumes and every connection is
// held by a transaction waiting for another — not slow, stuck. The suite found
// this the honest way: the full integration lane hung for ten minutes on a test
// that has nothing to do with sending.
//
// So the message is read and prepared with no transaction open, and then ONE
// transaction claims the held row and writes. The claim is what makes that
// safe: it takes the row FOR UPDATE and requires it still to be held, so a
// second caller that prepared the same message in parallel finds nothing to
// claim and sends nothing. Preparing twice costs two reads; sending twice is
// the thing that must not happen, and it cannot.
//
// The scheduled fire (scheduledsendfire.go) still prepares inside its
// transaction. It runs one message at a time off a queue rather than at a
// request's pace, so it has not hit this — but it is the same trade and the
// same fix when it does.
func (s *Store) ResumeHeldSend(
	ctx context.Context, id ids.UUID, gate ConsentGate, stager DeliveryStager,
) (crmcontracts.Activity, error) {
	held, found, err := s.readHeldForResume(ctx, id)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if !found {
		// Cancelled, already resumed, or never held. Not found rather than a
		// conflict: from the caller's side there is no such message to resume,
		// and saying which of those it was would disclose the row.
		return crmcontracts.Activity{}, apperrors.ErrNotFound
	}
	origin, in, err := held.replay()
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// The intent travels with the message so the staging path can find the
	// review that names it, and through the review the decision standing over
	// it. It is where to look, never permission by itself.
	in.ResumingIntentID = id
	prepared, err := s.PrepareSend(ctx, origin, in, gate, stager)
	if err != nil {
		return crmcontracts.Activity{}, err
	}

	var sent crmcontracts.Activity
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// CLAIMED AGAIN, INSIDE THE TRANSACTION THAT WRITES. The read above was
		// unlocked, so the row may have been cancelled or resumed by somebody
		// else since — and this is where that is decided, under the row's own
		// lock, before anything is staged.
		if _, found, err := s.claimHeldForResume(ctx, tx, id); err != nil {
			return err
		} else if !found {
			return apperrors.ErrNotFound
		}
		sent, err = s.SendPreparedTx(ctx, tx, origin, prepared, stager)
		if err != nil {
			return err
		}
		if err := s.releaseHeldInTx(ctx, tx, id, ids.UUID(sent.Id)); err != nil {
			return err
		}
		// AND THE REVIEW, because this message has now gone.
		//
		// THE HOLE THIS CLOSES. Consuming an instruction resolves the review it
		// answered, which covers every DIRECTED resume. A resume the engine now
		// ALLOWS — the evidence arrived, the stop was lifted, the cap rolled
		// over — consumes no instruction and so took that path's exit without
		// passing its door. The message went out and its review stayed live,
		// showing a decider a refusal about a message already in somebody's
		// inbox; approving it mints an instruction for a delivery that has been
		// made.
		//
		// SAFE TO CALL FOR BOTH. The closer only touches a review that is still
		// live, so a directed resume finds its own review already resolved and
		// this does nothing. Asking "was an instruction consumed" here would
		// mean reaching into consent's own bookkeeping to avoid a statement
		// that is already a no-op.
		return s.closeReview(ctx, tx, id, ReviewOutcomeSent)
	})
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	return sent, nil
}

// readHeldForResume reads the frozen message with no transaction open, so the
// preparation that follows does not run while a connection is held. Nothing is
// decided here: claimHeldForResume decides, under the lock, in the transaction
// that writes.
func (s *Store) readHeldForResume(ctx context.Context, id ids.UUID) (claimedSend, bool, error) {
	var c claimedSend
	var found bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		c, found, err = s.readHeldRow(ctx, tx, id)
		return err
	})
	return c, found, err
}

// claimHeldForResume locks one held row, or reports that there is nothing to
// resume.
//
// SCOPED TO send_refused, not to every hold. The other hold reasons stop a
// message that was already a promise to send at a moment — one that missed its
// window, or whose sender went inactive — and those resume through the
// scheduled path by being re-armed, not by being fired now. A refusal hold is
// the only one whose resolution is "send this, now, because somebody decided".
func (s *Store) claimHeldForResume(ctx context.Context, tx pgx.Tx, id ids.UUID) (claimedSend, bool, error) {
	return s.heldRow(ctx, tx, id, " FOR UPDATE")
}

// readHeldRow is the same read without the lock, for the preparation step.
func (s *Store) readHeldRow(ctx context.Context, tx pgx.Tx, id ids.UUID) (claimedSend, bool, error) {
	return s.heldRow(ctx, tx, id, "")
}

// heldRow is the one query both spellings run, so the claim and the read cannot
// come to disagree about which rows are resumable.
func (s *Store) heldRow(ctx context.Context, tx pgx.Tx, id ids.UUID, lock string) (claimedSend, bool, error) {
	var c claimedSend
	err := tx.QueryRow(ctx, `
		SELECT scheduled_at, origin_kind, anchor_activity_id,
		       origin_links, also_links, payload, payload_version, version
		  FROM scheduled_send
		 WHERE id = $1 AND status = 'held' AND held_reason = $2`+lock, id, HeldSendRefused).
		Scan(&c.ScheduledAt, &c.OriginKind, &c.Anchor,
			&c.OriginLinks, &c.AlsoLinks, &c.Payload, &c.Version, &c.RowVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedSend{}, false, nil
	}
	if err != nil {
		return claimedSend{}, false, fmt.Errorf("held send: claiming the message to resume: %w", err)
	}
	return c, true, nil
}

// releaseHeldInTx records that the held message reached the delivery
// machinery.
//
// 'released' rather than 'sent', for the scheduled path's reason: the provider
// has not been called and the dispatcher can still park or fail this delivery.
// The held_reason is cleared with the move, which the row's shape CHECK
// requires of anything that is no longer held.
func (s *Store) releaseHeldInTx(ctx context.Context, tx pgx.Tx, id, activityID ids.UUID) error {
	var deliveryID ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM comms_outbound WHERE activity_id = $1`, activityID).Scan(&deliveryID); err != nil {
		return fmt.Errorf("held send: finding the delivery it staged: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE scheduled_send
		   SET status = 'released', held_reason = NULL,
		       activity_id = $2, delivery_id = $3,
		       version = version + 1, updated_at = now()
		 WHERE id = $1 AND status = 'held'`, id, activityID, deliveryID)
	if err != nil {
		return fmt.Errorf("held send: recording that the message went: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// The claim above holds this row's lock, so this cannot happen unless a
		// caller reached here without one. Refusing is the safe direction.
		return fmt.Errorf("held send: the message was no longer held when it was sent: %w",
			apperrors.ErrConflict)
	}
	return nil
}
