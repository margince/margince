// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The River wiring of the hourly scout passes, Deal Scout's and the tag
// scout's. Each runs once per workspace, in that workspace's transaction, as
// the scout's own system principal. The passes themselves are RunDealScout and
// RunTagScout.

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

// scoutTally is what one scout pass did, for the log line. DealScoutPass and
// TagScoutPass convert to it.
type scoutTally struct {
	Superseded, Considered, Raised int
}

// scoutWorker runs one scout's pass in every workspace. Each scout's own
// worker type embeds it and names its job args.
type scoutWorker struct {
	pool    *pgxpool.Pool
	now     func() time.Time
	log     *slog.Logger
	logLine string
	run     func(ctx context.Context, tx pgx.Tx, now time.Time) (scoutTally, error)
}

// scoutWorkspace runs w's pass as actor, the scout's own system principal, so
// every suggestion reads as the scout's claim rather than a colleague's.
func scoutWorkspace(ctx context.Context, w *scoutWorker, actor string, workspace ids.UUID) error {
	wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), actor)
	var pass scoutTally
	if err := database.WithWorkspaceTx(wsCtx, w.pool, func(tx pgx.Tx) error {
		var err error
		pass, err = w.run(wsCtx, tx, w.now())
		return err
	}); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// Logged on every pass: considered tells a quiet week from a broken read,
	// which raised alone cannot.
	w.log.InfoContext(wsCtx, w.logLine,
		"considered", pass.Considered, "raised", pass.Raised, "superseded", pass.Superseded)
	return nil
}

type dealScoutWorker struct{ scoutWorker }

func (w *dealScoutWorker) Work(ctx context.Context, _ *river.Job[DealScoutArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, func(ctx context.Context, workspace ids.UUID) error {
		return scoutWorkspace(ctx, &w.scoutWorker, "agent:deal-scout", workspace)
	}))
}

// addDealScoutJobs registers the pass and hands back its schedule, whose
// cadence is api/jobs.yaml's.
func addDealScoutJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	addDeclaredWorker[DealScoutArgs](reg, &dealScoutWorker{scoutWorker{
		pool: pool, now: time.Now, log: log, logLine: "deal scout pass",
		run: func(ctx context.Context, tx pgx.Tx, now time.Time) (scoutTally, error) {
			pass, err := RunDealScout(ctx, tx, now)
			return scoutTally(pass), err
		},
	}})
	return periodicFor(cfg, DealScoutArgs{})
}
