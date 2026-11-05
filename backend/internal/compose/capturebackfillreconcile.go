// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CaptureBackfillReconcileArgs restores queued imports and lost page executions.
type CaptureBackfillReconcileArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CaptureBackfillReconcileArgs) Kind() string { return "capture_backfill_reconcile" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CaptureBackfillReconcileArgs) FleetWide() {}

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
	return w.registry.DispatchBackfills(wsCtx, backfillScheduler(workspace), true)
}
