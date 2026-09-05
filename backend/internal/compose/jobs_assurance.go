// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The nightly input check: the caller three shipped surfaces were waiting for.
//
// assurance.NewScanner had exactly one caller in the tree and it was a test.
// So Scan never ran, StartRun never inserted, and LatestRun answered ErrNotFound
// for ever — which the Forecast tab renders as a failed panel, on every load,
// for every seat, on a default install. The module itself works and is covered;
// what was missing was the pass, and absence has no line to be wrong on.
//
// The pass runs as the SYSTEM across the whole pipeline, which is what
// AssuranceSubjects is unscoped for: a duplicate-detection rule that saw one
// rep's deals would report no duplicates. What that costs is paid on the way
// out — every reader of a finding passes through their own row scope first.
//
// It is assembled from the two seams that already existed for it
// (AssuranceSubjects, AssuranceCoverage) rather than reading deals of its own.
// A pass that re-derived its subjects would be a second answer to which deals
// are in the pipeline, and the run's coverage line would stop describing the
// read it was taken from.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AssuranceSweepArgs schedules one input check across the fleet.
type AssuranceSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (AssuranceSweepArgs) Kind() string { return "assurance_sweep" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (AssuranceSweepArgs) FleetWide() {}

// assuranceSweepWorker checks every live tenant's forecast inputs.
//
// One worker where there were two (ADR-0103).
type assuranceSweepWorker struct {
	pool *pgxpool.Pool
	now  func() time.Time
	log  *slog.Logger
}

func (w *assuranceSweepWorker) Work(ctx context.Context, _ *river.Job[AssuranceSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.assureWorkspace))
}

// assuranceActor is the principal the nightly pass runs as, and therefore the
// one every run it starts is attributed to. Declared rather than typed at the
// call site because the suite asserting that attribution has to name the same
// principal the worker binds, and two hand-typed copies would agree only until
// one of them moved.
const assuranceActor = "system:assurance"

func (w *assuranceSweepWorker) assureWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	wsCtx = principal.WithActor(wsCtx, principal.Principal{
		Type: principal.PrincipalSystem, ID: assuranceActor,
	})
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	return w.check(wsCtx, workspace)
}

// check runs one pass and reports what it came to.
//
// Scan never refuses to start, and this must not either: a source that could
// not be read makes the run INCOMPLETE and its readiness `checks_incomplete`,
// which is a run that says so, rather than no run at all. Returning an error
// on an incomplete pass would leave River retrying a state the pass already
// recorded correctly.
//
// The outcome is logged at info because it is the one line an operator has
// saying the check happened: the surfaces read the row, not the log, so a pass
// that had stopped running entirely would otherwise be visible only as a page
// that stopped changing.
func (w *assuranceSweepWorker) check(ctx context.Context, ws ids.UUID) error {
	scanner := assurance.NewScanner(
		assurance.NewStore(InstallationDB(w.pool)),
		AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig(),
	)
	result, err := scanner.Scan(ctx, w.now())
	if err != nil {
		return err
	}
	w.log.InfoContext(ctx, "the forecast's inputs were checked",
		"workspace_id", ws, "run_id", result.RunID,
		"eligible_deals", result.EligibleDeals, "findings", result.Findings, "cleared", result.Cleared,
		"readiness", result.Readiness, "status", result.Status)
	return nil
}
