// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Applying a posture backwards, over mail this mailbox already brought in.
//
// Only narrows. A mailbox moving towards `shared` leaves its history exactly
// where it is, because opening what was captured under a stricter answer is a
// different decision from what to do next — the messages were held for reasons
// that were true when they landed, and a posture change is not a review of
// them. Widening history back is its own explicit call.
//
// It writes the seat's own import rows and lets the audience derivation move
// the activity, so a message another mailbox also imported ends at the
// strictest answer across all of them rather than at this one's.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// narrowBatch is how many import rows one pass claims.
//
// It bounds the STATEMENT, not the transaction. Every pass currently runs
// inside the caller's single transaction, so no lock is released until the last
// one commits and SKIP LOCKED has nothing to skip — a mailbox with tens of
// thousands of messages is one request holding that many row locks and writing
// that many audit and outbox rows before it returns.
//
// That is a real limit and it is the reason apply_to_history belongs in a job:
// the ratchet in narrowDue already makes this idempotent and resumable, which
// is what a job needs, and committing per batch is the only thing missing. It
// is not done here because moving it out of the posture write's transaction
// changes what a caller who asked for both is promised, and that is a product
// question rather than a refactor. Filed rather than half-done.
const narrowBatch = 500

// narrowDue is the batch predicate the claim and the progress check both ask.
// One constant rather than two copies: a termination test that disagreed with
// what the loop actually claims is the one way this spins, and two spellings are
// how they would come to disagree.
//
// Held by: TestNarrowingToClassifiedNeverLoosensAHeldMessage
// (backend/internal/compose/integration/capture/capture_posture_integration_test.go)
// — it fails the moment the claim stops excluding an already-held row. The
// single-source property itself is held by construction rather than by that
// test: this is one Go constant interpolated into both statements, so there is
// no second copy to drift.
//
// Two things ride on the second clause. It is what makes the loop terminate — a
// row it writes stops matching, so a pass that claims nothing means the work is
// done — and the third clause keeps the operation one-directional: narrowing to
// classified must not reach a row already held, which would be a widening
// wearing a narrowing's name.
const narrowDue = `user_id = $1
	   AND coalesce(posture_at_import, 'shared') <> $2
	   AND coalesce(posture_at_import, 'shared') <> 'held'`

// NarrowRemainingTx counts what a further pass would still claim. The caller
// calls it to prove each pass made progress, rather than trusting that it did.
func NarrowRemainingTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, posture string) (int, error) {
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_import WHERE `+narrowDue, seat, posture).Scan(&n); err != nil {
		return 0, fmt.Errorf("capture: counting the import rows still to narrow: %w", err)
	}
	return n, nil
}

// NarrowHistoryTx applies one posture to the mail this seat already imported,
// in one bounded batch, and answers how many rows it moved.
//
// The caller loops until it answers zero. That is the shape rather than a
// single unbounded statement because the work is proportional to a mailbox's
// history and the transaction holding it blocks captures into the same rows.
func NarrowHistoryTx(
	ctx context.Context, tx pgx.Tx, seat ids.UUID, posture string,
	recompute AudienceRecomputer,
) (int, error) {
	if posture != PostureHeld && posture != PostureClassified {
		// A posture that opens is not applied backwards. See the file comment:
		// the messages were held for reasons that were true when they landed.
		return 0, nil
	}
	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT id FROM capture_import
			 WHERE `+narrowDue+`
			 ORDER BY id
			 LIMIT $3
			 FOR UPDATE SKIP LOCKED
		)
		UPDATE capture_import i
		   SET posture_at_import = $2
		  FROM due
		 WHERE i.id = due.id
		RETURNING i.activity_id`, seat, posture, narrowBatch)
	if err != nil {
		return 0, fmt.Errorf("capture: claiming import rows to narrow: %w", err)
	}
	var touched []ids.ActivityID
	for rows.Next() {
		var id ids.ActivityID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("capture: reading a claimed import row: %w", err)
		}
		touched = append(touched, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("capture: claiming import rows to narrow: %w", err)
	}
	return len(touched), recomputeEach(ctx, tx, touched, recompute)
}

// recomputeEach re-derives the audience of every activity a batch touched.
//
// Per row rather than in bulk: the derivation locks one activity, reads every
// import row of it and may emit an event, and there is no set-based spelling of
// that which keeps the lock order the rest of the tree follows.
func recomputeEach(
	ctx context.Context, tx pgx.Tx, ids []ids.ActivityID, recompute AudienceRecomputer,
) error {
	if recompute == nil {
		return nil
	}
	for _, id := range ids {
		if err := recompute(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
