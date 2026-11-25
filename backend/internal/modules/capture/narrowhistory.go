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
// It writes the connection's own import rows and lets the audience derivation
// move the activity. A message another mailbox also imported ends at the
// strictest answer of them all.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// narrowBatch is how many import rows one batch of the background narrowing
// claims. Each row locks an activity and writes an audit and an outbox row, so
// a small batch keeps its transaction short.
const narrowBatch = 200

// narrowScope is the import rows one connection's narrowing reaches. where
// tests the unaliased capture_import row against the connection id in $1.
type narrowScope struct {
	connection ids.UUID
	where      string
}

// mailboxScope reaches a shared mailbox's own imports.
func mailboxScope(connection ids.UUID) narrowScope {
	return narrowScope{connection: connection, where: "capture_import.connection_id = $1"}
}

// deliveredScope reaches the imports of a seat's mailbox: the ones its
// delivery mark names, and the seat's rows from before marks existed. Not
// CapturingMailboxSQL: that also asks captured_by, which names only the first seat.
func deliveredScope(connection ids.UUID) narrowScope {
	return narrowScope{connection: connection, where: `EXISTS (SELECT 1 FROM capture_connection due_mailbox
	  WHERE due_mailbox.id = $1 AND due_mailbox.user_id = capture_import.user_id
	    AND (cardinality(capture_import.delivered_by) = 0 OR EXISTS (SELECT 1 FROM unnest(capture_import.delivered_by) delivered(mark)
	      WHERE split_part(delivered.mark, ':', 1) = ` + storekit.MailboxSQL("due_mailbox") + `)))`}
}

// narrowDue is the batch predicate the claim and the progress check both
// interpolate. A row it writes stops matching, which ends the
// narrowing. A held row never matches, so narrowing to classified never loosens it.
func narrowDue(scope narrowScope) string {
	return scope.where + `
	   AND coalesce(posture_at_import, 'shared') <> $2
	   AND coalesce(posture_at_import, 'shared') <> 'held'`
}

// narrowRemainingTx counts what a further batch would still claim. The job
// calls it to prove each batch made progress, rather than trusting that it did.
func narrowRemainingTx(ctx context.Context, tx pgx.Tx, scope narrowScope, posture string) (int, error) {
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM capture_import WHERE `+narrowDue(scope), scope.connection, posture).Scan(&n); err != nil {
		return 0, fmt.Errorf("capture: counting the import rows still to narrow: %w", err)
	}
	return n, nil
}

// narrowHistoryTx applies one posture to the mail a connection already brought
// in, in one bounded batch, and answers the activities whose import it moved.
func narrowHistoryTx(
	ctx context.Context, tx pgx.Tx, scope narrowScope, posture string,
	recompute AudienceRecomputer,
) ([]ids.ActivityID, error) {
	if posture != PostureHeld && posture != PostureClassified {
		// A posture that opens is not applied backwards. See the file comment:
		// the messages were held for reasons that were true when they landed.
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT id FROM capture_import
			 WHERE `+narrowDue(scope)+`
			 ORDER BY id
			 LIMIT $3
			 FOR UPDATE SKIP LOCKED
		)
		UPDATE capture_import i
		   SET posture_at_import = $2
		  FROM due
		 WHERE i.id = due.id
		RETURNING i.activity_id`, scope.connection, posture, narrowBatch)
	if err != nil {
		return nil, fmt.Errorf("capture: claiming import rows to narrow: %w", err)
	}
	touched, err := pgx.CollectRows(rows, pgx.RowTo[ids.ActivityID])
	if err != nil {
		return nil, fmt.Errorf("capture: claiming import rows to narrow: %w", err)
	}
	return touched, recomputeEach(ctx, tx, touched, recompute)
}

// recomputeEach re-derives the audience of every activity a batch touched.
//
// Per row rather than in bulk: the derivation locks one activity, reads every
// import row of it and may emit an event, and there is no set-based spelling of
// that which keeps the lock order the rest of the tree follows.
func recomputeEach(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.ActivityID, recompute AudienceRecomputer,
) error {
	if recompute == nil {
		return nil
	}
	// IN ORDER, because the recompute row-locks each activity and
	// this module is not the only writer that locks several of them at once.
	// Two callers looping in whatever order their own query returned can each
	// hold the other's next row, and Postgres ends that by aborting one.
	//
	// activities.RecomputeAudiencesTx sorts the same way for the callers that
	// can reach it. This module cannot — a module never imports a sibling, and
	// the recompute arrives here as an injected function — so the order is
	// taken from ids.Ascending, which is the one spelling both share.
	for _, id := range ids.Ascending(activityIDs) {
		if err := recompute(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}
