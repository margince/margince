// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The interaction-participant backfill as a background pass (ADR-0078).
//
// Capture stamps participants for new mail, but every message already in the
// timeline predates ACT-DDL-3. Until those are recovered, "who on our team
// knows this contact" reads empty on exactly the workspaces that have the most
// history — which, to the contact looking at the screen, is indistinguishable
// from a broken feature.
//
// It is a job and not an UPDATE inside migration 0157 because a migration
// holds its lock for its whole duration: a workspace with a real mailbox has
// hundreds of thousands of activity rows, and a slow backfill inside the
// migration turns a deploy into an outage.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ParticipantBackfillArgs is the periodic pass's (empty) job payload.
type ParticipantBackfillArgs struct{}

// Kind is the River job kind for the participant backfill.
func (ParticipantBackfillArgs) Kind() string { return "participant_backfill" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (ParticipantBackfillArgs) FleetWide() {}

// participantBackfillBatch is how many activities one statement attributes.
// Modest on purpose: the pass holds a write transaction for its duration and
// competes with live capture for the same rows, so a large batch buys
// throughput nobody is waiting for at the cost of lock time capture IS waiting
// on.
const participantBackfillBatch = 500

// participantBackfillBatchesPerTick bounds one Work invocation. A tick that
// drains 25 batches recovers 12,500 activities and then yields, so a
// long-history workspace finishes over a few passes instead of monopolizing a
// worker slot — and no single transaction grows long enough to matter.
const participantBackfillBatchesPerTick = 25

// participantBackfillWorker recovers participants for one workspace at a time.
type participantBackfillWorker struct {
	pool  *pgxpool.Pool
	store *activities.Store
	log   *slog.Logger
}

func newParticipantBackfillWorker(pool *pgxpool.Pool, log *slog.Logger) *participantBackfillWorker {
	return &participantBackfillWorker{pool: pool, store: activities.NewStore(InstallationDB(pool)), log: log}
}

// Work sweeps every live workspace. A per-workspace fault is logged and never
// aborts the pass — one workspace's bad row must not starve the rest — and the
// next tick simply re-selects whatever this one did not finish, because the
// pass carries no cursor to lose.
func (w *participantBackfillWorker) Work(ctx context.Context, _ *river.Job[ParticipantBackfillArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.backfillOneWorkspace))
}

func (w *participantBackfillWorker) backfillOneWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.WithWorkspaceID(ctx, workspace)
	recovered, err := w.backfillWorkspace(ctx, workspace)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if recovered > 0 {
		w.log.InfoContext(ctx, "participant backfill: recovered interaction participants",
			"workspace", workspace.String(), "rows", recovered)
	}
	return nil
}

// backfillWorkspace drains up to a tick's worth of batches, stopping early the
// moment a batch reports no work — which is what makes a caught-up
// installation cost one probe per pass rather than a full drain.
func (w *participantBackfillWorker) backfillWorkspace(ctx context.Context, ws ids.UUID) (int, error) {
	wsCtx := principal.WithWorkspaceID(ctx, ws)
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:participant_backfill",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	total := 0
	for i := 0; i < participantBackfillBatchesPerTick; i++ {
		n, err := w.store.BackfillParticipantsBatch(wsCtx, participantBackfillBatch)
		if err != nil {
			return total, err
		}
		if n == 0 {
			break
		}
		total += n
	}
	for _, pass := range backfillPasses {
		n, err := w.drainPass(wsCtx, pass)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// backfillPass is one pass the worker drains each tick: the function that runs
// one bounded batch of it, and that batch's size.
type backfillPass struct {
	batch func(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error)
	limit int
}

// backfillPasses run after the two-end backfill and never instead of it: an
// activity with no participants at all is the worse gap, and settling that
// first means a workspace part-way through recovery still answers "who was in
// this" with something true.
var backfillPasses = []backfillPass{
	// The CCs and meeting attendees of activities whose two ends are recorded.
	{replayParticipantsBatch, participantReplayBatch},
	// The meetings whose attendees were read under the earlier rule, which bound no
	// colleague from a calendar's list. They already carry a replay marker, so
	// the pass above never offers them again, and until they are re-read a
	// colleague who was in a meeting cannot read it.
	{repairMeetingAttendeesBatch, meetingAttendeeRepairPerTick},
	// The meetings the calendar had already called off when they were captured.
	// A provider stops listing an event once it is off, so no later sync will
	// say otherwise. It writes only meeting_status, so its place here is free.
	{backfillMeetingRSVPBatch, meetingRSVPBackfillPerTick},
	// After the passes above, because an attendee row that does not exist yet
	// cannot be given the name its invitation used.
	{recoverAttendeeNamesBatch, participantReplayBatch},
	// And after that, because the names it recovers are what a stale display
	// name is refreshed from.
	{repairStaleDisplayNamesBatch, participantReplayBatch},
	// Mail a seat sent from another address of theirs, still stored as
	// received. Last, so a fault of its own stops none of the passes above; a
	// claim hands the row back to the replay for the next tick.
	{repairOwnSentMailBatch, ownSentMailRepairPerTick},
}

// drainPass runs one pass's batches until a batch finds nothing or the tick's
// bound is reached, so a workspace with none left costs one probe a tick.
func (w *participantBackfillWorker) drainPass(wsCtx context.Context, pass backfillPass) (int, error) {
	total := 0
	for i := 0; i < participantBackfillBatchesPerTick; i++ {
		n, err := pass.batch(wsCtx, w.pool, pass.limit, w.log)
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, nil
		}
		total += n
	}
	return total, nil
}

// participantReplayBatch is smaller than the two-end batch because this pass does
// real work per row — it parses a stored RFC822 message or calendar resource
// in Go rather than running one join in the database.
const participantReplayBatch = 100
