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
			_, err = client.InsertTx(ctx, tx, CaptureBackfillArgs{Workspace: workspace, BackfillID: id.String(), ExecutionGeneration: generation},
				jobs.QueuedAs[CaptureBackfillArgs](&river.InsertOpts{MaxAttempts: rowOwnedMaxAttempts, ScheduledAt: successorStart(due, time.Now()), UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: activeSweepStates}}))
			return err
		},
		Executing: func(ctx context.Context, tx pgx.Tx, id ids.UUID, generation int64) (bool, error) {
			return jobs.IsRunningTx(ctx, tx, CaptureBackfillArgs{Workspace: workspace, BackfillID: id.String(), ExecutionGeneration: generation})
		},
	}
}

// immediateSuccessorSlack covers the drift between the database clock that
// stamped due and this process's clock.
const immediateSuccessorSlack = time.Second

// successorStart is the ScheduledAt for a page execution due at due. River
// inserts any job with a ScheduledAt as scheduled. Only its maintenance pass,
// every few seconds, makes that job available. A job with no ScheduledAt is
// available when the insert commits. So a page due now starts at once, and
// only a real delay such as a rate-limit backoff waits.
func successorStart(due, now time.Time) time.Time {
	if due.After(now.Add(immediateSuccessorSlack)) {
		return due
	}
	return time.Time{}
}
