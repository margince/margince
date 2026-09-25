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
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
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
	// activities is the door the bundled task is minted through — the same one
	// REST CreateTask and MCP create_task use. It is held here rather than built
	// per pass because a store is a handle, and the seam that needs it takes it
	// as a dependency so a test can hand in one over its own pool.
	activities *activities.Store
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
	wsCtx = principal.SystemActing(wsCtx, assuranceActor)
	started, err := assurance.NewStore(InstallationDB(w.pool)).EverRun(wsCtx)
	if err != nil {
		return err
	}
	if !started {
		// A workspace nobody has started is not checked by the calendar. The
		// first pass over a backlog raises every exception it can see at once,
		// and a queue that arrives that way is one nobody reads — so somebody
		// starts it, having read the preview.
		//
		// Logged rather than silent because this is the branch that can fail
		// SHORT: a workspace wrongly read as unstarted stops being checked and
		// nothing asserts. An installation already being checked cannot enter
		// it — enrolment IS the run history, and those workspaces have runs.
		w.log.InfoContext(ctx, "the forecast's inputs were not checked: nobody has started this workspace",
			"workspace_id", workspace)
		return nil
	}
	return w.check(wsCtx, workspace, nil)
}

// AssuranceRunArgs is one pass a human asked for, on one workspace.
//
// Uniqueness is keyed on Workspace alone (river:"unique"), so two managers
// pressing recheck at once collapse to one pass rather than racing two;
// RequestedBy is provenance and stays outside the hash.
type AssuranceRunArgs struct {
	Workspace   ids.UUID `json:"workspace_id" river:"unique"`
	RequestedBy string   `json:"requested_by"`
}

// Kind is the stable job identifier River persists in river_job.
func (AssuranceRunArgs) Kind() string { return "assurance_run" }

// WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
func (a AssuranceRunArgs) WorkspaceID() ids.UUID { return a.Workspace }

// assuranceRunWorker runs the pass one human asked for.
//
// It shares check with the nightly sweep and skips only the enrolment gate —
// this IS how a workspace enrols, so a gate here would make starting impossible.
type assuranceRunWorker struct {
	sweep *assuranceSweepWorker
}

func (w *assuranceRunWorker) Work(ctx context.Context, job *river.Job[AssuranceRunArgs]) error {
	// The guard FIRST, before the worker touches anything: a zero workspace
	// would bind an empty GUC and the pass would then read and write as
	// whatever the connection happens to carry.
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// SYSTEM, on a pass a human asked for, and deliberately: the pass reads the
	// whole pipeline, and every exception it raises is captured_by whoever it
	// runs as. Attributed to the manager who pressed the button, the run would
	// sign them to hundreds of findings they never made. Who asked is a
	// different fact and rides on the run row's own requested_by.
	wsCtx = principal.SystemActing(wsCtx, assuranceActor)
	// An unnamed asker is refused HERE, before the pass does its work. The run
	// row's own constraint refuses a blank requester, so the alternative is a
	// full walk of the pipeline thrown away at the commit — and the transport
	// that enqueues this already holds an authenticated human, so a blank one
	// means the args were built somewhere that does not.
	if strings.TrimSpace(job.Args.RequestedBy) == "" {
		return jobs.FaultContext(ctx, fmt.Errorf(
			"%s: a pass somebody asked for carries no asker", job.Args.Kind()))
	}
	return jobs.FaultContext(ctx, w.sweep.check(wsCtx, job.Args.Workspace, &job.Args.RequestedBy))
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
func (w *assuranceSweepWorker) check(ctx context.Context, ws ids.UUID, requestedBy *string) error {
	store := assurance.NewStore(InstallationDB(w.pool))
	scanner := assurance.NewScanner(store, AssuranceSubjects, AssuranceCoverage, assurance.DefaultConfig())
	result, err := scanner.Scan(ctx, w.now(), requestedBy)
	if err != nil {
		return err
	}

	// Bundling runs AFTER the scan has committed, and its failure does NOT fail
	// the job. The findings are recorded and the three surfaces render them
	// either way; returning an error here would have River retry a pass whose
	// real output already landed, re-scanning and re-minting on every attempt.
	// A night that scanned but did not bundle is a night without tasks, which
	// the next pass mints again — the loss is bounded, and the log says so.
	minted, bundleErr := bundleRunFindings(ctx, bundleDeps{
		bundles:    store,
		activities: w.activities,
		exceptions: bundleableFindings,
	}, result.RunID)
	if bundleErr != nil {
		w.log.ErrorContext(ctx, "the findings were recorded but not bundled into tasks",
			"workspace_id", ws, "run_id", result.RunID, "error", bundleErr)
	}

	w.log.InfoContext(ctx, "the forecast's inputs were checked",
		"workspace_id", ws, "run_id", result.RunID,
		"eligible_deals", result.EligibleDeals, "findings", result.Findings, "cleared", result.Cleared,
		"readiness", result.Readiness, "status", result.Status, "tasks_minted", minted,
		"requested", requestedBy != nil)
	return nil
}
