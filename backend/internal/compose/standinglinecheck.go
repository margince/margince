// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The How it stands check: an activity landing on a contact, company or
// project asks whether that record's status line still holds. The
// cg:standing-line consumer (standingline.Trigger) queues one check per activity.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/standingline"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// standingLineActor names the check as the principal of its reads and writes.
const standingLineActor = "system:standing-line"

// StandingLineCheckArgs is one check of the records one activity landed on.
type StandingLineCheckArgs struct {
	ActivityID ids.UUID `json:"activity_id" river:"unique"`
}

// Kind is the string River persists for this job.
func (StandingLineCheckArgs) Kind() string { return "standing_line_check" }

func standingLineCheckInsertOpts() *river.InsertOpts {
	return jobs.QueuedAs[StandingLineCheckArgs](&river.InsertOpts{
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	})
}

// StandingLineLane is the lane the How it stands check reads with. A nil
// brain registers nothing, and each page keeps the reading it derives.
type StandingLineLane struct {
	StandingLineBrain completer
	// StandingLineRoutingVersion keys a line's fingerprint to the binding.
	StandingLineRoutingVersion func() string
}

type standingLineCheckWorker struct {
	pool      *pgxpool.Pool
	refresher *standingline.Refresher
}

func newStandingLineCheckWorker(pool *pgxpool.Pool, cfg JobRunnerConfig) *standingLineCheckWorker {
	routing := cfg.StandingLineRoutingVersion
	if routing == nil {
		routing = func() string { return "" }
	}
	return &standingLineCheckWorker{pool: pool, refresher: standingline.NewRefresher(
		pool, activities.NewStore(InstallationDB(pool)), cfg.StandingLineBrain, routing, time.Now)}
}

func (w *standingLineCheckWorker) Work(ctx context.Context, job *river.Job[StandingLineCheckArgs]) error {
	wsCtx, err := bindInstallation(ctx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	err = w.refresher.Refresh(principal.SystemActing(wsCtx, standingLineActor),
		ids.From[ids.ActivityKind](job.Args.ActivityID))
	return jobs.FaultContext(ctx, snoozeWhileRestricted(err))
}

// StandingLineEnqueuer queues a check in a transaction of its own.
type StandingLineEnqueuer interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// NewStandingLineTrigger builds the cg:standing-line consumer.
func NewStandingLineTrigger(pool *pgxpool.Pool, queue StandingLineEnqueuer, log *slog.Logger) *standingline.Trigger {
	enqueue := func(ctx context.Context, tx pgx.Tx, ws ids.WorkspaceID, activity ids.UUID) error {
		return queue.EnqueueTx(ctx, tx, StandingLineCheckArgs{ActivityID: activity},
			standingLineCheckInsertOpts())
	}
	return standingline.NewTrigger(pool, enqueue, identity.NewService(pool).InstallationWorkspace, log)
}
