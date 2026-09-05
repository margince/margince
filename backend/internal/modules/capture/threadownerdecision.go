// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What the MAILBOX OWNER concluded about a thread, which outranks the
// classifier: the seat whose correspondence it is decides, and their answer is
// recorded as its own status so a later reader can tell a person's decision
// from a model's.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DecideAsOwner records what the mailbox owner themselves concluded about a
// thread, overruling whatever a classifier said.
//
// A human decision is not a verdict with a different author: it is the end of
// the question. The row resolves, its attempts stop mattering, and no later
// pass re-asks — a classifier that could overturn an owner would make the
// owner's click advisory, which is not what a person clicking "keep this
// private" is told they are doing.
//
// Own thread only. The ledger is per seat, so the row this writes is the
// caller's own contribution and no other seat's is touched: a thread reaching
// two mailboxes ends at the strictest of what the two of them ask for, and one
// owner sharing cannot publish what the other holds.
func (s *ThreadVerdictStore) DecideAsOwner(
	ctx context.Context, tx pgx.Tx, threadKey string, share bool,
) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return apperrors.ErrPermissionDenied
	}
	status := VerdictHeldByOwner
	if share {
		status = VerdictSharedByOwner
	}
	// A thread this call opens fresh (never through EnsureTx — a message born
	// open under a `shared` mailbox never did) needs an anchor of its own, or
	// heldthreadslist's existence check has nothing to join against and reports
	// an intact message as erased. The earliest of the seat's own live
	// messages in the thread, the same messages ThreadActivityIDsTx names.
	anchor, err := firstThreadActivityTx(ctx, tx, threadKey, actor.UserID)
	if err != nil {
		return err
	}
	// Upserted, because a thread the owner decides about may have no ledger row
	// at all: a message born open under a `shared` mailbox never opened a
	// question, and its owner may still want it held.
	if _, err := tx.Exec(ctx, `
		INSERT INTO capture_thread_verdict
		  (thread_key, user_id, first_activity_id, status, disposition_reason, resolved_at, next_attempt_at)
		VALUES ($1, $2, $3, $4, $5, now(), NULL)
		ON CONFLICT (thread_key, user_id) DO UPDATE
		   SET first_activity_id = coalesce(capture_thread_verdict.first_activity_id, EXCLUDED.first_activity_id),
		       status = EXCLUDED.status,
		       disposition_reason = EXCLUDED.disposition_reason,
		       resolved_at = now(),
		       next_attempt_at = NULL,
		       claimed_by = NULL, claimed_until = NULL,
		       updated_at = now()`,
		threadKey, actor.UserID, anchor, status, ownerDecisionReason); err != nil {
		return fmt.Errorf("capture: recording the owner's decision about a thread: %w", err)
	}
	// The seat's own import rows for the thread, which is what the audience
	// derivation reads. Without this the ledger would carry a decision nothing
	// acts on.
	// restricted_at excluded: a message under a statutory hold or an open
	// erasure is not one an owner's click may move. The hold is an obligation
	// the installation owes somebody else, and it outranks both directions of
	// this decision — sharing a held message would publish it, and re-holding
	// one changes nothing a hold has not already done.
	if _, err := tx.Exec(ctx, `
		UPDATE capture_import i
		   SET verdict_status = $3
		  FROM activity a
		 WHERE a.id = i.activity_id
		   AND a.thread_key = $1
		   AND a.restricted_at IS NULL
		   AND a.archived_at IS NULL
		   AND i.user_id = $2`, threadKey, actor.UserID, status); err != nil {
		return fmt.Errorf("capture: applying the owner's decision to their import rows: %w", err)
	}
	return nil
}

// ownerDecisionReason marks a ledger row a person settled, so a reader can tell
// it from one a classifier reached.
const ownerDecisionReason = "owner_decision"

// firstThreadActivityTx names the earliest of the seat's own live messages in
// a thread, for a ledger row that needs an anchor and does not have one yet.
// Built over ThreadActivityIDsTx rather than a hand-rolled query: it already
// reads exactly this seat's own thread and ids sort chronologically (ids.UUID
// is a v7). The store re-derives its own answer rather than trusting a
// caller's — DecideAsOwner's one caller happens to have already run this same
// read, but a store that assumed so would break the day a second caller did
// not. A thread with nothing to anchor to answers nil rather than the zero
// UUID, which heldthreadslist reads as "no message" if it were ever written.
func firstThreadActivityTx(ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID) (*ids.UUID, error) {
	activityIDs, err := ThreadActivityIDsTx(ctx, tx, threadKey, user)
	if err != nil {
		return nil, err
	}
	if len(activityIDs) == 0 {
		return nil, nil //nolint:nilnil // deliberate: no message to anchor to is not an error
	}
	return &activityIDs[0], nil
}

// ThreadActivityIDsTx lists the messages of one thread this seat imported.
//
// Scoped to the caller's own imports rather than to the thread key alone: a
// thread key is the sender-controlled References root in a workspace-wide
// namespace, so a walk over it would reach messages this seat never received.
func ThreadActivityIDsTx(ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $2
		 WHERE a.thread_key = $1 AND a.archived_at IS NULL
		   AND a.restricted_at IS NULL
		 ORDER BY a.id`, threadKey, user)
	if err != nil {
		return nil, fmt.Errorf("capture: listing a thread's messages: %w", err)
	}
	defer rows.Close()
	var out []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("capture: listing a thread's messages: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: listing a thread's messages: %w", err)
	}
	return out, nil
}

// ThreadJudgedByClassifierTx answers whether this seat's verdict on a thread
// came from a CLASSIFIER rather than from the sender's own subject marking.
//
// The two are indistinguishable on the activity row, where both spell their
// reason `explicitly_confidential`, and they are not the same fact: a marking
// is the sender's and no recipient lifts it, while a verdict is a judgement its
// owner may disagree with. The ledger is where they differ — a verdict records
// the `kind` it concluded, and a marking records none — so the question is
// asked here, of the table this module owns, and answered for the caller that
// owns the column.
//
// False for a thread with no ledger row at all, which is the honest answer:
// nothing judged it, so nothing of the classifier's is there to lift.
func ThreadJudgedByClassifierTx(
	ctx context.Context, tx pgx.Tx, threadKey string, user ids.UUID,
) (bool, error) {
	if threadKey == "" || user == ids.Nil {
		return false, nil
	}
	var judged bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM capture_thread_verdict
		   WHERE thread_key = $1 AND user_id = $2 AND kind IS NOT NULL)`,
		threadKey, user).Scan(&judged); err != nil {
		return false, fmt.Errorf("capture: reading whether a classifier judged this thread: %w", err)
	}
	return judged, nil
}
