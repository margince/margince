// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// TagScoutArgs runs one fleet-wide tag scout pass.
type TagScoutArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (TagScoutArgs) Kind() string { return "tag_scout" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide).
func (TagScoutArgs) FleetWide() {}

type tagScoutWorker struct{ scoutWorker }

func (w *tagScoutWorker) Work(ctx context.Context, _ *river.Job[TagScoutArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.scoutWorkspace))
}

// addTagScoutJobs registers the pass and hands back its schedule, whose
// cadence is api/jobs.yaml's.
func addTagScoutJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	addDeclaredWorker[TagScoutArgs](reg, &tagScoutWorker{scoutWorker{
		pool: pool, now: time.Now, log: log, actor: "agent:tag-scout", logLine: "tag scout pass",
		run: func(ctx context.Context, tx pgx.Tx, now time.Time) (scoutTally, error) {
			pass, err := RunTagScout(ctx, tx, now)
			return scoutTally(pass), err
		},
	}})
	return periodicFor(cfg, TagScoutArgs{})
}
