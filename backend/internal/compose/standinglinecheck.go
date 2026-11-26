// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The How it stands check: an activity landing on a contact, company or
// project asks whether that record's status line still holds
// (compose/standingline). The cg:standing-line consumer queues one check per
// activity, and the job does the reading and the model call.

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/standingline"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// standingLineActor names the check as the principal of its reads and writes.
const standingLineActor = "system:standing-line"

// standingLineBacklogWindow skips replayed history: a new consumer group
// starts at the stream's beginning, and an old activity is not news.
const standingLineBacklogWindow = 7 * 24 * time.Hour

// StandingLineCheckArgs is one check of the records one activity landed on.
type StandingLineCheckArgs struct {
	Workspace  ids.UUID `json:"workspace_id" river:"unique"`
	ActivityID ids.UUID `json:"activity_id" river:"unique"`
}

// Kind is the string River persists for this job.
func (StandingLineCheckArgs) Kind() string { return "standing_line_check" }

// WorkspaceID binds the check to its tenant (jobs.WorkspaceScoped).
func (a StandingLineCheckArgs) WorkspaceID() ids.UUID { return a.Workspace }

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
	refresher *standingline.Refresher
}

func newStandingLineCheckWorker(pool *pgxpool.Pool, cfg JobRunnerConfig) *standingLineCheckWorker {
	routing := cfg.StandingLineRoutingVersion
	if routing == nil {
		routing = func() string { return "" }
	}
	return &standingLineCheckWorker{refresher: standingline.NewRefresher(
		pool, activities.NewStore(InstallationDB(pool)), cfg.StandingLineBrain, routing, time.Now)}
}

func (w *standingLineCheckWorker) Work(ctx context.Context, job *river.Job[StandingLineCheckArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
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

// StandingLineTrigger queues a check for each activity that lands or changes.
type StandingLineTrigger struct {
	pool      *pgxpool.Pool
	queue     StandingLineEnqueuer
	workspace func(context.Context) (ids.WorkspaceID, error)
	log       *slog.Logger
	now       func() time.Time
}

// NewStandingLineTrigger builds the cg:standing-line consumer.
func NewStandingLineTrigger(pool *pgxpool.Pool, queue StandingLineEnqueuer, log *slog.Logger) *StandingLineTrigger {
	return &StandingLineTrigger{
		pool: pool, queue: queue, workspace: identity.NewService(pool).InstallationWorkspace,
		log: log, now: time.Now,
	}
}

// HandleEvent queues one check for an activity that may move the picture.
func (t *StandingLineTrigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	if !events.StandingLineTypes.Has(env.Type) || t.now().Sub(env.OccurredAt) > standingLineBacklogWindow {
		return nil
	}
	if !t.moves(ctx, env) {
		return nil
	}
	ws, err := t.workspace(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, t.pool, func(tx pgx.Tx) error {
		return t.queue.EnqueueTx(ctx, tx, StandingLineCheckArgs{Workspace: ws.UUID, ActivityID: env.Entity.ID},
			standingLineCheckInsertOpts())
	})
}

// moves says whether the event can change where a record stands. That is a
// new activity other than a task, or one filed or marked held later. Margince
// files many tasks of its own, and what is owed is on the page.
func (t *StandingLineTrigger) moves(ctx context.Context, env events.Envelope) bool {
	if env.Type == eventActivityUpdated {
		var updated crmcontracts.PublicEventActivityUpdated
		if err := json.Unmarshal(env.Payload, &updated); err != nil {
			t.log.WarnContext(ctx, "standing line trigger: unreadable payload", "event", env.EventID.String(), "err", err)
			return false
		}
		return updated.ChangedFields.Relinked != nil || updated.ChangedFields.MeetingStatus != nil
	}
	var captured crmcontracts.PublicEventActivityCaptured
	if err := json.Unmarshal(env.Payload, &captured); err != nil {
		t.log.WarnContext(ctx, "standing line trigger: unreadable payload", "event", env.EventID.String(), "err", err)
		return false
	}
	return captured.Kind != string(crmcontracts.ActivityKindTask)
}
