// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The nightly repair behind mailbox backfill (ADR-0063). A paging job is
// inserted with one attempt because the run row owns the outcome, and a run
// is protected by a unique index over its connection while it is live. Lose
// that one attempt to something River never sees and the run stays live with
// no job — and the index then refuses every future start on that connection.
// This pass re-enqueues, under the same uniqueness the start uses, so a live
// job absorbs the insert and a lost one is replaced.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CaptureBackfillReconcileArgs is the nightly pass that puts a paging job back
// behind every backfill run that is still live (ADR-0063).
type CaptureBackfillReconcileArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CaptureBackfillReconcileArgs) Kind() string { return "capture_backfill_reconcile" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CaptureBackfillReconcileArgs) FleetWide() {}

// captureBackfillReconcileWorker re-enqueues the runs nothing is paging.
//
// It DECIDES NOTHING, and that is the whole design. A run that is queued or
// running is owed a job; the insert carries the same ByArgs uniqueness the
// start does, so a run whose job is alive dedupes onto it and a run whose job
// is gone gets a new one. There is no staleness threshold to tune and no
// judgement about whether a run "looks stuck" — the uniqueness answers that
// exactly, and a wrong answer from a heuristic would either re-page a healthy
// import or leave a stranded one stranded.
//
// It does not fail runs either. A stranded run has recorded no failure —
// nothing failed, something disappeared — so its give-up cap has nothing to say
// about it. What decides whether it keeps going is the engine, on the next page
// it actually runs.
type captureBackfillReconcileWorker struct {
	pool     *pgxpool.Pool
	registry *capture.Registry
	log      *slog.Logger
}

func (w *captureBackfillReconcileWorker) Work(ctx context.Context, _ *river.Job[CaptureBackfillReconcileArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.reconcileWorkspace))
}

func (w *captureBackfillReconcileWorker) reconcileWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	live, err := w.registry.LiveBackfills(wsCtx)
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range live {
		// One insert per run, each in its own transaction: a run whose enqueue
		// is refused must not take the others down with it, because they are
		// unrelated imports that happen to be live at the same moment.
		if err := w.enqueuePaging(wsCtx, workspace, id); err != nil {
			failures = append(failures, fmt.Errorf("backfill %s: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

// enqueuePaging offers ONE run a paging job, on the same terms the start does.
//
// The options are the start's, deliberately spelled the same way: one attempt,
// because the run row owns the outcome, and ByArgs uniqueness over the active
// states, which is what makes this whole pass idempotent. A different cap or a
// different window here would make a re-enqueued run behave unlike a started
// one, and the difference would only ever show up on the runs that had already
// gone wrong once.
// The AMBIENT client, the way this file's other one-off enqueues reach River:
// this runs inside a job, so the client that is working it is the one to insert
// through. The Safely variant is deliberate — the plain ClientFromContext
// panics when there is none, and reaching this from outside a running job is a
// wiring mistake worth an error rather than a crash.
func (w *captureBackfillReconcileWorker) enqueuePaging(ctx context.Context, workspace, backfillID ids.UUID) error {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("no River client in context: %w", err)
	}
	_, err = client.Insert(ctx, CaptureBackfillArgs{
		Workspace: workspace, BackfillID: backfillID.String(),
	}, &river.InsertOpts{
		MaxAttempts: rowOwnedMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	})
	return err
}
