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

// DedupeRecheckArgs identifies the fleet-wide pass that closes open duplicate
// pairs the filing rule no longer files.
type DedupeRecheckArgs struct{}

// Kind names the declared dedupe re-check job.
func (DedupeRecheckArgs) Kind() string { return "dedupe_recheck" }

// FleetWide makes workspace fan-out explicit to the job binder.
func (DedupeRecheckArgs) FleetWide() {}

type dedupeRecheckWorker struct{ pool *pgxpool.Pool }

func (w *dedupeRecheckWorker) Work(ctx context.Context, _ *river.Job[DedupeRecheckArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.recheckWorkspace))
}

func (w *dedupeRecheckWorker) recheckWorkspace(ctx context.Context, workspace ids.UUID) error {
	store := contacts.NewStore(database.BindTo(w.pool, ids.From[ids.WorkspaceKind](workspace)))
	actor := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "dedupe_recheck")
	_, err := store.RecheckOpenDedupeCandidates(actor)
	return err
}

func addDedupeRecheckJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	addDeclaredWorker[DedupeRecheckArgs](reg, &dedupeRecheckWorker{pool: pool})
	return periodicFor(cfg, DedupeRecheckArgs{})
}
