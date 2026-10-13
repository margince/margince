// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The pass that settles every import run nothing else will move. A commit
// whose process stopped is marked failed, so approve resumes it from its
// checkpoint. A test run whose process stopped returns to draft. A draft left
// untouched past its time to live is cancelled. A run that will never read its
// file again releases it, so the stored-object reaper deletes the bytes. An
// undoing run is never touched, because undo already resumes it.

import (
	"context"
	"errors"
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
	{"failed_commits", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.FailStaleRunning(ctx, importCommitTimeout, staleImportCause)
	}},
	{"test_runs_returned", func(ctx context.Context, runs *migration.RunStore) (int, error) {
		return runs.ReturnStaleTestRuns(ctx, staleValidationAfter)
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
