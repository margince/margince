// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TxInserter is the slice of River that schedules a flush.
type TxInserter interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// Enqueuer schedules a link's flush in the caller's transaction. Job builds
// the flush's args. The flush is unique per link while it waits or runs, so
// ten events in a window make one post.
type Enqueuer struct {
	Inserter    TxInserter
	Delay       time.Duration
	Job         func(workspace, linkID ids.UUID) river.JobArgs
	MaxAttempts int
	States      []rivertype.JobState
}

// EnqueueFlushTx implements chatlinks.FlushEnqueuer.
func (e Enqueuer) EnqueueFlushTx(ctx context.Context, tx pgx.Tx, linkID ids.UUID) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("chat links: scheduling a flush outside a workspace context")
	}
	return e.Inserter.EnqueueTx(ctx, tx, e.Job(ws, linkID), &river.InsertOpts{
		MaxAttempts: e.MaxAttempts,
		ScheduledAt: time.Now().Add(e.Delay),
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: e.States},
	})
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
