// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The api role's door onto the finance sync. It queues the worker's pass and
// returns, so a source with many customers never holds a request open.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// accountingReadQueue keeps the finance passes, which read an outside system
// for minutes, off the default queue's workers.
const (
	accountingReadQueue      = "accounting_read"
	accountingReadMaxWorkers = 2
)

// financeSyncEnqueuer is the enqueue seam (jobs.Runner.Enqueue); tests fake it.
type financeSyncEnqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

type financeSyncQueue struct{ enqueue financeSyncEnqueuer }

// QueueSync queues this workspace's pass. Unique while queued or running, so
// a second press joins the first rather than reading the source twice.
func (q financeSyncQueue) QueueSync(ctx context.Context) error {
	return q.enqueue.Enqueue(ctx, FinanceSyncArgs{Workspace: storekit.MustWorkspace(ctx)},
		jobs.QueuedAs[FinanceSyncArgs](&river.InsertOpts{
			MaxAttempts: oneOffJobMaxAttempts,
			UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
		}))
}

// WithFinanceSources wires the accounting-source endpoints: the composed
// sources they offer, and the api role's insert-only runner their syncs go
// through. Without it those endpoints answer 501.
func WithFinanceSources(inserter financeSyncEnqueuer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.financeHandlers = newFinanceHandlers(pool).WithSources(composedFinanceSources{}, financeSyncQueue{enqueue: inserter})
	}
}
