// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Deal Scout's River wiring: one hourly pass per workspace. Job args and the
// worker adapter only — the pass itself is RunDealScout.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealScoutArgs runs one fleet-wide Deal Scout pass.
type DealScoutArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (DealScoutArgs) Kind() string { return "deal_scout" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide).
func (DealScoutArgs) FleetWide() {}

type dealScoutWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
	log  *slog.Logger
}

func (w *dealScoutWorker) Work(ctx context.Context, _ *river.Job[DealScoutArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.scoutWorkspace))
}

func (w *dealScoutWorker) scoutWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	// The scout is the acting principal, so every suggestion carries agent:
	// provenance and reads as the scout's claim rather than a colleague's.
	wsCtx = principal.SystemActing(wsCtx, "agent:deal-scout")
	var pass DealScoutPass
	if err := database.WithWorkspaceTx(wsCtx, w.pool, func(tx pgx.Tx) error {
		var err error
		pass, err = RunDealScout(wsCtx, tx, w.now())
		return err
	}); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// Logged on every pass: considered tells a quiet week from a broken read,
	// which raised alone cannot.
	w.log.InfoContext(wsCtx, "deal scout pass",
		"considered", pass.Considered, "raised", pass.Raised, "superseded", pass.Superseded)
	return nil
}

// addDealScoutJobs registers the pass and hands back its schedule, whose
// cadence is api/jobs.yaml's.
func addDealScoutJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	addDeclaredWorker[DealScoutArgs](reg, &dealScoutWorker{pool: pool, now: time.Now, log: log})
	return periodicFor(cfg, DealScoutArgs{})
}
