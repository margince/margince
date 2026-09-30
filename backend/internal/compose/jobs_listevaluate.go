// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Live List check's River wiring: one pass per workspace every fifteen
// minutes. Job args and the worker adapter only — the check itself is
// collections.Store.CheckLiveLists.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// listCheckerActor is the principal every observed entered and left event
// names as its actor.
const listCheckerActor = "system:list-checker"

// ListEvaluateArgs runs one fleet-wide Live List check.
type ListEvaluateArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ListEvaluateArgs) Kind() string { return "list_evaluate" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide).
func (ListEvaluateArgs) FleetWide() {}

type listEvaluateWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
	log  *slog.Logger
}

func (w *listEvaluateWorker) Work(ctx context.Context, _ *river.Job[ListEvaluateArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.checkWorkspace))
}

func (w *listEvaluateWorker) checkWorkspace(ctx context.Context, workspace ids.UUID) error {
	checks, err := CheckLiveLists(ctx, w.pool, workspace, w.now)
	entered, left, failed := 0, 0, 0
	for _, check := range checks {
		entered += check.Entered
		left += check.Left
		if check.Err != nil {
			failed++
			// A failed list keeps its old checkpoint, so the next pass takes it
			// first; failing the job would re-check every list that succeeded.
			w.log.WarnContext(ctx, "live list check failed", "list", check.ListID, "err", check.Err)
		}
	}
	// Logged on every pass: lists checked tells a quiet quarter-hour from a
	// broken read, which entered and left alone cannot.
	w.log.InfoContext(ctx, "live list check", "workspace", workspace, "lists", len(checks),
		"failed", failed, "entered", entered, "left", left)
	return err
}

// CheckLiveLists runs one pass over a workspace's Live Lists as the system:
// the check has to see every record, and each reader's scope is applied when
// they read what it recorded. Each list is stamped with now read as its own
// check starts.
func CheckLiveLists(ctx context.Context, pool *pgxpool.Pool, workspace ids.UUID, now func() time.Time) ([]collections.LiveCheck, error) {
	wsCtx := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), listCheckerActor)
	return NewCollectionsStore(pool).CheckLiveLists(wsCtx, func() time.Time { return now().UTC() })
}

// addListEvaluateJobs registers the check and hands back its schedule, whose
// cadence is api/jobs.yaml's.
func addListEvaluateJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	addDeclaredWorker[ListEvaluateArgs](reg, &listEvaluateWorker{pool: pool, now: time.Now, log: log})
	return periodicFor(cfg, ListEvaluateArgs{})
}
