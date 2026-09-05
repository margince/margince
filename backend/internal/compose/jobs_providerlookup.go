// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The catch-up lane: a dispatcher that fans out per workspace, and a worker
// that queues one tick's worth of free lookups for contacts no run has covered.
//
// Separate from the poll lane beside it because the two do opposite things.
// That one DRAINS work already created, retries three times, and runs every
// thirty seconds. This one CREATES work, must not retry — a retry queues a
// second batch for the same tick — and its budget is what paces the customer's
// spending.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ProviderLookupSweepArgs schedules one fleet-wide catch-up pass.
type ProviderLookupSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ProviderLookupSweepArgs) Kind() string { return "provider_lookup_sweep" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (ProviderLookupSweepArgs) FleetWide() {}

// providerLookupSweepWorker fans the pass out per workspace.
type providerLookupSweepWorker struct {
	pool *pgxpool.Pool
	cfg  ProviderRunsConfig
}

// The tick is the real cadence: a workspace that failed is picked up by the
// next pass a minute later, which is why this takes one attempt and no ladder.
func (w *providerLookupSweepWorker) Work(ctx context.Context, _ *river.Job[ProviderLookupSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.lookupWorkspace))
}

func (w *providerLookupSweepWorker) lookupWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := providerJobActor(principal.WithWorkspaceID(ctx, workspace))
	store, err := providerLookupStore(w.pool, w.cfg, workspace)
	if err != nil {
		return err
	}
	// The count is deliberately dropped: this lane's progress is the backlog
	// count the settings card reads, which is derived from the same predicate
	// the sweep selects on. A log line per tick per workspace would say the
	// same thing less reliably and once a minute forever.
	_, err = store.BackfillSweep(wsCtx)
	return jobs.FaultContext(ctx, err)
}

// providerLookupStore is providerRunStore PLUS the submit enqueue.
//
// The poll lane's store deliberately has none: it only drains runs somebody
// else queued, so a nil enqueue there is a wiring fact rather than a gap. This
// lane queues, and QueueRun refuses outright without one — a sweep on the poll
// lane's store would error on every contact it tried, forever, while looking
// like a slow sweep.
//
// The inserter comes from the job's own context rather than from config: River
// puts its client there for exactly this, and the three capture lanes already
// reach it the same way. Threading a runner through ProviderRunsConfig would
// make every construction site carry one so that this path could enqueue.
func providerLookupStore(pool *pgxpool.Pool, cfg ProviderRunsConfig, workspace ids.UUID) (*integrations.Store, error) {
	store, err := providerRunStore(pool, cfg, workspace)
	if err != nil {
		return nil, err
	}
	return store.WithSubmitEnqueue(providerLookupEnqueue), nil
}

// providerLookupEnqueue commits the submit job in the same transaction as the
// run row, through the client River bound to this worker's context.
func providerLookupEnqueue(ctx context.Context, tx pgx.Tx, runID, workspaceID string) error {
	client, err := river.ClientFromContextSafely[pgx.Tx](ctx)
	if err != nil {
		return fmt.Errorf("compose: the catch-up sweep has no job client to submit through: %w", err)
	}
	ws, err := ids.Parse(workspaceID)
	if err != nil {
		return fmt.Errorf("compose: the submit job's workspace id does not parse: %w", err)
	}
	if _, err := client.InsertTx(ctx, tx, ProviderRunSubmitArgs{Workspace: ws, RunID: runID}, nil); err != nil {
		return fmt.Errorf("compose: enqueueing a swept contact's submission: %w", err)
	}
	return nil
}
