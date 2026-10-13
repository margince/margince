// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The pass that frees an import whose process stopped mid-commit or mid-test
// run. It only marks the run failed; approve resumes a commit, and a test run
// is uploaded again. An undoing run is never touched, because undo already
// resumes it.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// staleImportCause is the error a swept run carries; GET shows it beside the
// checkpoint approve resumes from.
const staleImportCause = "the import stopped because the server restarted; approve it again to continue from its checkpoint"

// staleValidationAfter is when a test run is past saving. Its request ends at
// the response deadline, so twice that leaves the request time to record it.
const staleValidationAfter = 2 * httpserver.ResponseDeadline

// ImportRunSweepArgs carries nothing: the pass reads which runs are stale.
type ImportRunSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ImportRunSweepArgs) Kind() string { return "import_run_sweep" }

// importRunSweepWorker fails every running import idle past the commit budget,
// and every test run its request left behind.
type importRunSweepWorker struct {
	identity *identity.Service
	pool     *pgxpool.Pool
	log      *slog.Logger
}

func (w *importRunSweepWorker) Work(ctx context.Context, _ *river.Job[ImportRunSweepArgs]) error {
	ctx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("import_run_sweep: resolving the installation: %w", err))
	}
	ctx = principal.SystemActing(ctx, "system:import-run-sweep")
	runs := migration.NewRunStore(InstallationDB(w.pool))
	failed, err := runs.FailStaleRunning(ctx, importCommitTimeout, staleImportCause)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("import_run_sweep: failing stale runs: %w", err))
	}
	abandoned, err := runs.FailStaleValidating(ctx, staleValidationAfter)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("import_run_sweep: failing stale test runs: %w", err))
	}
	if failed+abandoned > 0 {
		w.log.WarnContext(ctx, "import runs had no process left working them", "failed", failed, "test_runs", abandoned)
	}
	return nil
}

// addImportRunSweepJob registers the pass and its cadence. Every role that
// works jobs runs it: a commit's process may be any API process.
func addImportRunSweepJob(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	addDeclaredWorker[ImportRunSweepArgs](reg, &importRunSweepWorker{
		identity: identity.NewService(pool), pool: pool, log: log,
	})
	return periodicFor(cfg, ImportRunSweepArgs{})
}
