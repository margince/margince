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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// tagScoutActor raises the tag suggestions, so each reads as the scout's claim.
const tagScoutActor = "agent:tag-scout"

// TagScoutArgs runs one fleet-wide tag scout pass.
type TagScoutArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (TagScoutArgs) Kind() string { return "tag_scout" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide).
func (TagScoutArgs) FleetWide() {}

// tagScoutWorker runs the model path where a tag_suggest lane is bound and the
// phrase pass where none is.
type tagScoutWorker struct {
	scoutWorker
	suggester *TagSuggester
}

func (w *tagScoutWorker) Work(ctx context.Context, _ *river.Job[TagScoutArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, func(ctx context.Context, workspace ids.UUID) error {
		if w.suggester == nil {
			return scoutWorkspace(ctx, &w.scoutWorker, tagScoutActor, workspace)
		}
		return w.suggestWorkspace(ctx, workspace)
	}))
}

// suggestWorkspace runs the model path in one workspace as the scout.
func (w *tagScoutWorker) suggestWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), tagScoutActor)
	return w.report(ctx, wsCtx, func() (scoutTally, error) {
		return w.suggester.RunWorkspace(wsCtx, w.now())
	})
}

// addTagScoutJobs registers the pass and hands back its schedule, whose
// cadence is api/jobs.yaml's.
func addTagScoutJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	worker := &tagScoutWorker{
		pool: pool, now: time.Now, log: log, logLine: "tag scout pass",
		run: func(ctx context.Context, tx pgx.Tx, now time.Time) (scoutTally, error) {
			pass, err := RunTagScout(ctx, tx, now)
			return scoutTally(pass), err
		},
	}
	if cfg.TagSuggestBrain != nil {
		worker.suggester = NewTagSuggester(pool, cfg.TagSuggestBrain, log)
	}
	addDeclaredWorker[TagScoutArgs](reg, worker)
	return periodicFor(cfg, TagScoutArgs{})
}
