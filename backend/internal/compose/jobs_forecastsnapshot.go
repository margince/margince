// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ForecastSnapshotSweepArgs schedules one daily freeze across the fleet.
type ForecastSnapshotSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ForecastSnapshotSweepArgs) Kind() string { return "forecast_snapshot_sweep" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (ForecastSnapshotSweepArgs) FleetWide() {}

// forecastSnapshotSweepWorker freezes every live workspace's forecast.
//
// One worker where there were two (ADR-0103).
type forecastSnapshotSweepWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
	log  *slog.Logger
}

func (w *forecastSnapshotSweepWorker) Work(ctx context.Context, _ *river.Job[ForecastSnapshotSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.snapshotWorkspace))
}

// forecastSnapshotActor is the principal the daily freeze runs as, and therefore
// the one every snapshot it writes is attributed to. Declared rather than typed
// at the call site because the suite asserting that attribution has to name the
// same principal the worker binds, and two hand-typed copies of it would agree
// only until one of them moved.
const forecastSnapshotActor = "system:forecast-snapshot"

func (w *forecastSnapshotSweepWorker) snapshotWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	wsCtx = principal.SystemActing(wsCtx, forecastSnapshotActor)
	return jobs.FaultContext(ctx, w.freeze(wsCtx, workspace))
}

// alreadyFrozenToday answers whether the daily arbiter refused this write.
//
// Two indexes enforce one rule, and both have to be recognised: a partial index
// cannot cover a null scope_id, so the workspace scope — the one this pass takes
// — has an arbiter of its own beside the general one.
func alreadyFrozenToday(err error) bool {
	if err == nil {
		return false
	}
	constraint, unique := storekit.UniqueViolation(err)
	return unique &&
		(constraint == "uq_forecast_snapshot_daily" ||
			constraint == "uq_forecast_snapshot_daily_workspace" ||
			constraint == "uq_forecast_snapshot_daily_context")
}
