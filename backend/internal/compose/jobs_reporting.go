// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReportScheduleSweepArgs identifies the fleet job; each installation evaluates its own due schedules.
type ReportScheduleSweepArgs struct{}

// Kind keeps the persisted River job identity stable.
func (ReportScheduleSweepArgs) Kind() string { return "report_schedule_sweep" }

// FleetWide keeps tenant work inside the per-installation sweep.
func (ReportScheduleSweepArgs) FleetWide() {}

type reportScheduleSweepWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func (w *reportScheduleSweepWorker) Work(ctx context.Context, _ *river.Job[ReportScheduleSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.sweepWorkspace))
}

func (w *reportScheduleSweepWorker) sweepWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.WithWorkspaceID(ctx, workspace)
	ctx = principal.SystemActing(ctx, "system:report-schedule")
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return newReportingService(w.pool, w.now).Sweep(ctx)
}
