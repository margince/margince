// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The pass that settles every import run nothing else will move. A stale or
// lost attempt of a test run, a commit or an undo is queued again. A commit old
// code started fails once idle, so approve resumes it. Test runs old code
// started and idle drafts are closed, and finished runs release their file.
// River's tables are never read.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// staleImportCause is the error a swept run carries; GET shows it beside the
// checkpoint approve resumes from.
const staleImportCause = "the import stopped because the server restarted; approve it again to continue from its checkpoint"

// ImportRunSweepArgs carries nothing: the pass reads which runs are stale.
type ImportRunSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ImportRunSweepArgs) Kind() string { return "import_run_sweep" }

// importRunSweepWorker runs every rule of the pass over the installation's runs.
type importRunSweepWorker struct {
	identity *identity.Service
	pool     *pgxpool.Pool
	log      *slog.Logger
}

// importSweepRule is one rule of the pass, named as the log reports it.
type importSweepRule struct {
	name   string
	settle func(context.Context, *migration.RunStore) (int, error)
}

var importSweepRules = []importSweepRule{
	{"stale_attempts_resumed", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.ResumeStaleAttempts(ctx, importStaleAfter, importMaxResumes, requeueImportAttempt)
	}},
	{"lost_jobs_requeued", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.RequeueLostJobs(ctx, importQueuedReinsertAfter, requeueImportAttempt)
	}},
	{"failed_commits", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.FailStaleRunning(ctx, importLegacyIdleAfter, staleImportCause)
	}},
	{"stopped_test_runs_cancelled", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.CancelStoppedTestRuns(ctx)
	}},
	{"drafts_expired", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.CancelIdleDrafts(ctx, importDraftTTL)
	}},
	{"sources_released", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.ReleaseFinishedSources(ctx)
	}},
}

// requeueImportAttempt inserts the job of the run's current attempt through
// the River client working the sweep, and answers whether River inserted it.
func requeueImportAttempt(ctx context.Context, tx pgx.Tx, run migration.Run) (bool, error) {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return false, fmt.Errorf("the sweep runs outside a River job: %w", err)
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return false, errors.New("the sweep's context binds no workspace")
	}
	var args importJobArgs
	switch run.Status {
	case migration.StatusRunning:
		args = ImportCommitArgs{Workspace: ws, RunID: run.ID, Attempt: run.JobAttempt}
	case migration.StatusUndoing:
		args = ImportUndoArgs{Workspace: ws, RunID: run.ID, Attempt: run.JobAttempt}
	case migration.StatusValidating:
		args = ImportValidateArgs{Workspace: ws, RunID: run.ID, Attempt: run.JobAttempt}
	default:
		return false, fmt.Errorf("import run %s is %s, which no job works", run.ID, run.Status)
	}
	res, err := client.InsertTx(ctx, tx, args, args.insertOpts())
	if err != nil {
		return false, fmt.Errorf("inserting %s for import run %s: %w", args.Kind(), run.ID, err)
	}
	return !res.UniqueSkippedAsDuplicate, nil
}

// Work runs every rule even when one fails, so one broken rule cannot keep
// the others' runs, and their files, held.
func (w *importRunSweepWorker) Work(ctx context.Context, _ *river.Job[ImportRunSweepArgs]) error {
	ctx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("import_run_sweep: resolving the installation: %w", err))
	}
	ctx = principal.SystemActing(ctx, "system:import-run-sweep")
	runs := migration.NewRunStore(InstallationDB(w.pool))
	var failures []error
	for _, rule := range importSweepRules {
		settled, err := rule.settle(ctx, runs)
		if err != nil {
			failures = append(failures, fmt.Errorf("import_run_sweep: %s: %w", rule.name, err))
		}
		if settled > 0 {
			w.log.InfoContext(ctx, "import runs settled by the sweep", "rule", rule.name, "runs", settled)
		}
	}
	return jobs.FaultContext(ctx, errors.Join(failures...))
}

// addImportRunSweepJob registers the pass and its cadence. Only the worker
// role works jobs, so only it runs the pass.
func addImportRunSweepJob(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	addDeclaredWorker[ImportRunSweepArgs](reg, &importRunSweepWorker{
		identity: identity.NewService(pool), pool: pool, log: log,
	})
	return periodicFor(cfg, ImportRunSweepArgs{})
}
