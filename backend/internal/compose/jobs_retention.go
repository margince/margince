// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// River wiring for the idempotency-retention pass: a dispatcher over EVERY
// workspace and a worker that purges one. It sits beside the other per-concern
// job files (jobs_capture.go, jobs_deals.go, jobs_overlay.go) rather than in
// jobs.go, which owns the runner's assembly.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// IdempotencyRetentionArgs schedules one purge of replay claims past the
// window. Always-on: the claim bodies are record snapshots, so retaining them
// past the retry they protect is subject data kept for no purpose.
type IdempotencyRetentionArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (IdempotencyRetentionArgs) Kind() string { return "idempotency_retention" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (IdempotencyRetentionArgs) FleetWide() {}

// idempotencyRetentionWorker is the dispatcher. It enumerates EVERY
// workspace, archived ones included: archiving does not un-store the claim
// snapshots inside a workspace, and idempotency_key.workspace_id is
// ON DELETE RESTRICT, so leftovers would also refuse the eventual hard delete.
type idempotencyRetentionWorker struct {
	pool    *pgxpool.Pool
	sweeper *IdempotencyRetentionSweeper
}

func (w *idempotencyRetentionWorker) Work(ctx context.Context, _ *river.Job[IdempotencyRetentionArgs]) error {
	return jobs.FaultContext(ctx, runPerEveryWorkspace(ctx, w.pool, w.retainWorkspace))
}

func (w *idempotencyRetentionWorker) retainWorkspace(ctx context.Context, workspace ids.UUID) error {
	return w.sweeper.SweepWorkspace(principal.WithWorkspaceID(ctx, workspace))
}
