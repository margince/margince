// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	mailHistoryQueue   = "mail_history"
	mailAdmissionQueue = "mail_admission"
)

func backfillScheduler(workspace ids.UUID) capture.BackfillScheduler {
	return capture.BackfillScheduler{
		Enqueue: func(ctx context.Context, tx pgx.Tx, id ids.UUID, generation int64, due time.Time) error {
			client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
			if err != nil {
				return err
			}
			// The database clock stamped due, in this transaction, so the
			// comparison reads that clock too.
			var now time.Time
			if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
				return err
			}
			_, err = client.InsertTx(ctx, tx, CaptureBackfillArgs{Workspace: workspace, BackfillID: id.String(), ExecutionGeneration: generation},
				jobs.QueuedAs[CaptureBackfillArgs](&river.InsertOpts{MaxAttempts: rowOwnedMaxAttempts, ScheduledAt: successorStart(due, now), UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}}))
			return err
		},
		Executing: func(ctx context.Context, tx pgx.Tx, id ids.UUID, generation int64) (bool, error) {
			return jobs.IsRunningTx(ctx, tx, CaptureBackfillArgs{Workspace: workspace, BackfillID: id.String(), ExecutionGeneration: generation})
		},
	}
}

// successorStart is the ScheduledAt for a page execution due at due. River
// inserts any job with a ScheduledAt as scheduled. Only its maintenance pass,
// every few seconds, makes that job available. A job with no ScheduledAt is
// available when the insert commits. So a page due now starts at once, and
// only a real delay such as a rate-limit backoff waits.
// now is the database's transaction time, the clock due was stamped with.
func successorStart(due, now time.Time) time.Time {
	if due.After(now) {
		return due
	}
	return time.Time{}
}
