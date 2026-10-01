// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Continuing a mailbox import that stopped on an error.
//
// A run that ends on an error keeps its cursor: the provider page it stopped
// at, and the counts it had reached. Starting again from the newest message
// re-reads everything already captured, which on a large mailbox under the
// provider's rate limits takes hours and spends nothing useful. So a new start
// whose window that run already covers reopens it instead, and it pages on
// from where it stopped with its counts intact.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// resumableRunPredicate is what makes a run continuable, on a query that names
// the run `b` and its connection `c`: it ended on an error, it still holds the
// page it stopped at, and it read the account the connection holds now. A page
// token belongs to one mailbox, so a run from before the connection was bound
// to another account has nothing to continue.
const resumableRunPredicate = `b.status = 'error' AND b.cursor IS NOT NULL
	AND (c.account_bound_at IS NULL OR b.created_at >= c.account_bound_at)`

// resumeFailedBackfillTx reopens the connection's newest run when it is
// resumable and its window covers windowMonths, and answers it; nil when there
// is none and the caller starts a new run.
//
// The caller holds the connection lock and has already refused a narrowing
// start, so no other run is live and windowMonths is at least the widest run's.
// Only the newest run is considered: a later run that finished or was stopped
// is the newer statement about this mailbox.
//
// The run goes back to queued with its cursor, its counts and its estimate as
// they were; the failure ladder and the restart allowance start again, because a start by hand is the
// evidence that whatever stopped it may be fixed.
// resumeOrNil continues the connection's failed run when one covers the window,
// schedules it in the same transaction, and records the resumption on the
// connection's trail. nil means nothing was resumed and the caller starts a new
// run, which is also the answer when the caller asked to start over (resume
// false).
func resumeOrNil(ctx context.Context, tx pgx.Tx, resume bool, connID ids.UUID, windowMonths int, widest *int, enqueue EnqueueBackfill) (*BackfillRun, error) {
	if !resume {
		return nil, nil //nolint:nilnil // starting over resumes nothing
	}
	resumed, err := resumeFailedBackfillTx(ctx, tx, connID, windowMonths)
	if err != nil || resumed == nil {
		return nil, err
	}
	if err := enqueue(ctx, tx, resumed.ID); err != nil {
		return nil, fmt.Errorf("capture: scheduling the backfill: %w", err)
	}
	return resumed, auditLifecycle(ctx, tx, "update", captureConnectionObject, connID,
		map[string]any{auditBackfillWindowMonths: widest},
		map[string]any{auditBackfillWindowMonths: windowMonths, "backfill_resumed": resumed.ID.String()})
}

// auditBackfillWindowMonths names how far back an import reaches on the
// connection's audit trail.
const auditBackfillWindowMonths = "backfill_window_months"

func resumeFailedBackfillTx(ctx context.Context, tx pgx.Tx, connID ids.UUID, windowMonths int) (*BackfillRun, error) {
	var run BackfillRun
	err := tx.QueryRow(ctx, `
		UPDATE capture_backfill
		   SET status = 'queued', completed_at = NULL, consecutive_failures = 0,
		       last_error_class = NULL, window_restarts = 0`+resetInflightProgress+`
		 WHERE id = (
		         SELECT b.id FROM capture_backfill b
		           JOIN capture_connection c ON c.id = b.connection_id
		          WHERE b.id = (SELECT id FROM capture_backfill WHERE connection_id = $1
		                         ORDER BY created_at DESC, id DESC LIMIT 1)
		            AND b.window_months >= $2
		            AND `+resumableRunPredicate+`)
		RETURNING id, connection_id, window_months, after_date, status, cursor,
		          total_estimate, total_estimate_is_floor,
		          scanned, captured, skipped, failed, contacts_created, companies_created,
		          started_at, updated_at`,
		connID, windowMonths).Scan(&run.ID, &run.ConnectionID, &run.WindowMonths, &run.AfterDate, &run.Status, &run.Cursor,
		&run.Estimate, &run.EstimateIsFloor,
		&run.Scanned, &run.Captured, &run.Skipped, &run.Failed, &run.Contacts, &run.Companies,
		&run.StartedAt, &run.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no resumable run is the ordinary answer: the caller starts a new one
	}
	if err != nil {
		return nil, fmt.Errorf("capture: resuming the failed backfill: %w", err)
	}
	return &run, nil
}

// BackfillReopened says the run is queued: something reopened it after it
// ended — a human's Continue, or a sync reviving a run its credential ended.
// A job that has just ended the run uses it to come back for the reopened run
// instead of leaving it to the nightly reconcile. A run that stayed live for
// any other reason (a page whose commit failed) is running, not queued, and
// is the reconcile's.
func (r *Registry) BackfillReopened(ctx context.Context, backfillID ids.UUID) (bool, error) {
	var queued bool
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT status = 'queued' FROM capture_backfill WHERE id = $1`, backfillID).Scan(&queued)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return queued, err
}
