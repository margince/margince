// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The input check over one deal, queued when that deal changed.
//
// The freshness consumer queues one job per changed deal, and the job runs
// the scanner's one-deal pass (assurance/targeted.go). A screen never waits
// for it: until it lands, the deal's findings read out of date.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AssuranceDealRescanArgs is the input check over one changed deal. Unique on
// both fields, so a burst of edits to one deal is one job.
type AssuranceDealRescanArgs struct {
	Workspace ids.UUID `json:"workspace_id" river:"unique"`
	Deal      ids.UUID `json:"deal_id" river:"unique"`
}

// Kind is the stable job identifier River persists in river_job.
func (AssuranceDealRescanArgs) Kind() string { return "assurance_deal_rescan" }

// WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
func (a AssuranceDealRescanArgs) WorkspaceID() ids.UUID { return a.Workspace }

const (
	// assuranceRescanQueue mirrors api/jobs.yaml, at one worker. The insert
	// takes the queue from the contract (jobs.QueuedAs).
	assuranceRescanQueue      = "assurance_rescan"
	assuranceRescanMaxWorkers = 1
	// assuranceRescanBatchLimit is how many deals one read of the stream
	// queues. A larger burst leaves the rest to the nightly pass; their
	// markers already say the findings are out of date.
	assuranceRescanBatchLimit = 50
	// assuranceRescanBacklogLimit is how many one-deal checks may wait in one
	// workspace. Under sustained edits the rest are left to the nightly pass,
	// so the queue cannot grow without bound.
	assuranceRescanBacklogLimit = 200
	// assuranceRescanRetry is when a pass that saw its deal change runs again.
	assuranceRescanRetry = 5 * time.Second
)

// assuranceRescanEnqueuer is what the freshness consumer needs from the job
// runner: an insert in the transaction that marked the deal.
type assuranceRescanEnqueuer interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// assuranceRescanInsertOpts dedupes on the deal while a job waits or runs. A
// change during a run is not lost: the run sees it and runs again.
// assuranceRescanRoom is how many more checks may be queued for workspace.
func assuranceRescanRoom(ctx context.Context, tx pgx.Tx, workspace ids.UUID) (int, error) {
	var waiting int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM river_job
		 WHERE kind = $1 AND args->>'workspace_id' = $2
		   AND state IN ('available', 'pending', 'scheduled', 'retryable')`,
		AssuranceDealRescanArgs{}.Kind(), workspace.String()).Scan(&waiting); err != nil {
		return 0, fmt.Errorf("compose: counting the waiting deal checks: %w", err)
	}
	return max(0, min(assuranceRescanBatchLimit, assuranceRescanBacklogLimit-waiting)), nil
}

func assuranceRescanInsertOpts() *river.InsertOpts {
	return jobs.QueuedAs[AssuranceDealRescanArgs](&river.InsertOpts{
		MaxAttempts: oneOffJobMaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	})
}

// assuranceDealRescanWorker shares the nightly worker's pool, clock and task
// door, so a one-deal pass bundles its findings the way the nightly one does.
type assuranceDealRescanWorker struct {
	sweep *assuranceSweepWorker
	// scan is the one-deal pass, the scanner's own unless a test wraps it.
	scan dealScan
}

// dealScan runs one deal's pass.
type dealScan func(ctx context.Context, store *assurance.Store, now time.Time, deal ids.UUID) (assurance.Result, error)

func (w *assuranceDealRescanWorker) Work(ctx context.Context, job *river.Job[AssuranceDealRescanArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	scan := w.scan
	if scan == nil {
		scan = w.sweep.scanDeal
	}
	err = w.sweep.rescanDeal(principal.SystemActing(wsCtx, assuranceActor), job.Args, scan)
	if errors.Is(err, assurance.ErrInputsMovedDuringPass) {
		return river.JobSnooze(assuranceRescanRetry)
	}
	return jobs.FaultContext(ctx, err)
}

// rescanDeal runs one deal's pass and bundles what it found. Bundling failing
// does not fail the pass, for the reason check gives: the findings landed.
//
// A change that lands after the pass cleared its marker and before this job
// ends is deduplicated against this running job. So a complete pass that
// finds the deal marked again runs again rather than leave it to the night.
func (w *assuranceSweepWorker) rescanDeal(ctx context.Context, args AssuranceDealRescanArgs, scan dealScan) error {
	store := assurance.NewStore(InstallationDB(w.pool))
	result, err := scan(ctx, store, w.now(), args.Deal)
	if err != nil || result.RunID.IsZero() {
		return err
	}
	minted, bundleErr := bundleRunFindings(ctx, bundleDeps{
		bundles: store, activities: w.activities, exceptions: bundleableFindings, now: w.now,
	}, result.RunID)
	if bundleErr != nil {
		w.log.ErrorContext(ctx, "the deal's findings were recorded but not bundled into tasks",
			"workspace_id", args.Workspace, "run_id", result.RunID, "error", bundleErr)
	}
	w.log.InfoContext(ctx, "one changed deal's inputs were checked",
		"workspace_id", args.Workspace, "deal_id", args.Deal, "run_id", result.RunID,
		"findings", result.Findings, "cleared", result.Cleared, "status", result.Status, "tasks_minted", minted)
	if result.Status != assurance.StatusComplete {
		return nil
	}
	pending, err := store.DealChangePending(ctx, args.Deal)
	if err == nil && pending {
		err = assurance.ErrInputsMovedDuringPass
	}
	return err
}

func (w *assuranceSweepWorker) scanDeal(ctx context.Context, store *assurance.Store, now time.Time, deal ids.UUID) (assurance.Result, error) {
	return assuranceScanner(w.pool, store).ScanDeal(ctx, now, deal)
}
