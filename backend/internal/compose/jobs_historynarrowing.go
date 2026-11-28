// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CaptureHistoryNarrowingArgs narrows one batch of one mailbox's earlier mail.
type CaptureHistoryNarrowingArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CaptureHistoryNarrowingArgs) Kind() string { return "capture_history_narrowing" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide).
func (CaptureHistoryNarrowingArgs) FleetWide() {}

type captureHistoryNarrowingWorker struct {
	pool     *pgxpool.Pool
	registry *capture.Registry
}

// Work runs one batch in the first workspace that has a narrowing pending. So
// one mailbox narrows at a time, and the cadence is the pause between batches.
// A workspace whose batch fails does not hold up the next one.
func (w *captureHistoryNarrowingWorker) Work(ctx context.Context, _ *river.Job[CaptureHistoryNarrowingArgs]) error {
	if w.registry == nil {
		return nil
	}
	var worked bool
	return jobs.FaultContext(ctx, runInstallationPass(ctx, w.pool, func(ctx context.Context, workspace ids.UUID) error {
		if worked {
			return nil
		}
		found, err := w.narrowWorkspace(ctx, workspace)
		worked = found && err == nil
		return err
	}))
}

// narrowWorkspace runs one batch in workspace as the system, the way
// captureBackfillReconcileWorker binds each workspace of its fleet walk.
func (w *captureHistoryNarrowingWorker) narrowWorkspace(ctx context.Context, workspace ids.UUID) (bool, error) {
	return w.registry.NarrowPendingHistory(principal.SystemActing(ctx, "system:history_narrowing"))
}
