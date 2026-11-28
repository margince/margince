// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TxInserter is the slice of River that schedules a flush.
type TxInserter interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// Enqueuer schedules a link's jobs in the caller's transaction. Job builds
// the flush's args and Pull the pull's. The flush is unique per link while it
// waits or runs, so ten events in a window make one post. The pull carries
// its own insert options.
type Enqueuer struct {
	Inserter    TxInserter
	Delay       time.Duration
	Job         func(linkID ids.UUID) river.JobArgs
	Pull        func(connectionID ids.UUID) river.JobArgs
	MaxAttempts int
	States      []rivertype.JobState
}

// EnqueueFlushTx implements chatlinks.Scheduler.
func (e Enqueuer) EnqueueFlushTx(ctx context.Context, tx pgx.Tx, linkID ids.UUID) error {
	return e.Inserter.EnqueueTx(ctx, tx, e.Job(linkID), &river.InsertOpts{
		MaxAttempts: e.MaxAttempts,
		ScheduledAt: time.Now().Add(e.Delay),
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: e.States},
	})
}

// EnqueuePullTx implements chatlinks.Scheduler.
func (e Enqueuer) EnqueuePullTx(ctx context.Context, tx pgx.Tx, connectionID ids.UUID) error {
	args := e.Pull(connectionID)
	return e.Inserter.EnqueueTx(ctx, tx, args, jobs.QueuedAsArgs(args, nil))
}

// AmbientInserter schedules through the River client working the current
// job, so a worker needs no client of its own.
type AmbientInserter struct{}

// EnqueueTx implements TxInserter.
func (AmbientInserter) EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("chat links: no River client on the job context: %w", err)
	}
	_, err = client.InsertTx(ctx, tx, args, opts)
	return err
}
