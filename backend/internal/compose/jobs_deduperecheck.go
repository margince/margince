// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DedupeRecheckArgs identifies the fleet-wide pass that closes open duplicate
// pairs the filing rule no longer files. It then files the pairs of the
// records changed since the last pass.
type DedupeRecheckArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (DedupeRecheckArgs) Kind() string { return "dedupe_recheck" }

// FleetWide makes workspace fan-out explicit to the job binder.
func (DedupeRecheckArgs) FleetWide() {}

type dedupeRecheckWorker struct{ pool *pgxpool.Pool }

func (w *dedupeRecheckWorker) Work(ctx context.Context, _ *river.Job[DedupeRecheckArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.recheckWorkspace))
}

// recheckWorkspace closes before it scans, so a pair closed tonight is not
// filed again by the same run.
func (w *dedupeRecheckWorker) recheckWorkspace(ctx context.Context, workspace ids.UUID) error {
	store := contacts.NewStore(database.BindTo(w.pool, ids.From[ids.WorkspaceKind](workspace))).
		WithConsumerMail(capture.MatcherTx)
	actor := principal.SystemActing(principal.WithWorkspaceID(ctx, workspace), "dedupe_recheck")
	if err := w.retractUnsupportedPhones(actor); err != nil {
		return err
	}
	if _, err := store.RecheckOpenDedupeCandidates(actor); err != nil {
		return err
	}
	_, err := store.ScanChangedForDuplicates(actor)
	return err
}

func addDedupeRecheckJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig) []*river.PeriodicJob {
	addDeclaredWorker[DedupeRecheckArgs](reg, &dedupeRecheckWorker{pool: pool})
	return periodicFor(cfg, DedupeRecheckArgs{})
}

// Reuse the repair's bounded transactions so later evidence loss is repaired
// before duplicate pairs are reconsidered, even after the data task completed.
func (w *dedupeRecheckWorker) retractUnsupportedPhones(ctx context.Context) error {
	const limit = 200
	after := ids.Nil
	for {
		var page contacts.SignaturePhoneRepairPage
		if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
			var err error
			page, err = contacts.RetractUnsupportedSignaturePhonesTx(ctx, tx, after, limit)
			return err
		}); err != nil {
			return err
		}
		if len(page.Scanned) < limit {
			return nil
		}
		after = page.Scanned[len(page.Scanned)-1]
	}
}
