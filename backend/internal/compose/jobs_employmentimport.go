// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EmploymentImportSweepArgs identifies the fleet-wide retained-evidence sweep.
type EmploymentImportSweepArgs struct{}

// Kind names the declared employment import job.
func (EmploymentImportSweepArgs) Kind() string { return "employment_import_sweep" }

// FleetWide makes workspace fan-out explicit to the job binder.
func (EmploymentImportSweepArgs) FleetWide() {}

type employmentImportWorker struct{ pool *pgxpool.Pool }

func (w *employmentImportWorker) Work(ctx context.Context, _ *river.Job[EmploymentImportSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.processWorkspace))
}

func (w *employmentImportWorker) processWorkspace(ctx context.Context, workspace ids.UUID) error {
	store := contacts.NewStore(database.BindTo(w.pool, ids.From[ids.WorkspaceKind](workspace)))
	actor := principal.WithWorkspaceID(ctx, workspace)
	actor = principal.SystemActing(actor, "employment_import_worker")
	return store.SweepEmploymentImports(actor)
}

func addEmploymentImportJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	addDeclaredWorker[EmploymentImportSweepArgs](reg, &employmentImportWorker{pool: pool})
	return periodicFor(cfg, EmploymentImportSweepArgs{})
}
