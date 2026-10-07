// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The daily model price sync's River wiring: one fleet pass that walks the
// workspaces and runs the same engine "Refresh now" does.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AIPriceSyncSweepArgs runs one fleet-wide model price sync.
type AIPriceSyncSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (AIPriceSyncSweepArgs) Kind() string { return "ai_price_sync_sweep" }

// FleetWide marks this as answering for the whole installation (jobs.FleetWide).
func (AIPriceSyncSweepArgs) FleetWide() {}

// aiPriceSyncActor names the pass on every price and audit row it writes.
const aiPriceSyncActor = "system:ai-price-sync"

type aiPriceSyncSweepWorker struct {
	pool *pgxpool.Pool
	sync *ai.PriceSync
	log  *slog.Logger
}

func (w *aiPriceSyncSweepWorker) Work(ctx context.Context, _ *river.Job[AIPriceSyncSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.syncWorkspace))
}

// syncWorkspace is one workspace's turn; its failure is logged here and joined
// by runPerWorkspace, so it does not stop the others.
func (w *aiPriceSyncSweepWorker) syncWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), aiPriceSyncActor)
	if err := w.sync.RunScheduled(wsCtx); err != nil {
		w.log.WarnContext(wsCtx, "ai price sync: this workspace's pass failed", "workspace", workspace, "error", err)
		return jobs.FaultContext(ctx, err)
	}
	return nil
}

// addAIPriceSyncJobs registers the sweep and hands back its schedule.
func addAIPriceSyncJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	sync := newAIPriceSync(pool, cfg.AIKeyVault, config.FromOS, log, newAIPriceCatalogues())
	addDeclaredWorker[AIPriceSyncSweepArgs](reg, &aiPriceSyncSweepWorker{pool: pool, sync: sync, log: log})
	return periodicFor(cfg, AIPriceSyncSweepArgs{})
}
