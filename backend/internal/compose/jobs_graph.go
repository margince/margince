// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The background passes behind the relationship graph (ADR-0078), registered
// together because they are two halves of one guarantee: the backfill gives
// the projection a past to fold, and the reconcile keeps the fold true as time
// passes. Neither needs a model or a provider, so both run wherever the worker
// runs — "who on our team knows this contact" is a deterministic question
// about our own mail, and an installation with AI switched off still answers it.

import (
	"context"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// addGraphJobs registers the graph workers and returns their periodic
// schedules for the caller to append.
//
// All three passes register unconditionally, which is the point the file
// comment makes: none of them needs a model or a provider, so there is nothing
// for a deployment to leave unconfigured. Their cadences are api/jobs.yaml's.
func addGraphJobs(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	// Each pass is a dispatcher plus a workspace worker; only the dispatcher
	// gets a periodic entry, and the workspace worker reuses its wiring.
	participants := newParticipantBackfillWorker(pool, cfg.Blobstore, log)
	addDeclaredWorker[ParticipantBackfillArgs](reg, participants)
	graphEdges := newGraphEdgeReconcileWorker(pool, log)
	addDeclaredWorker[GraphEdgeReconcileArgs](reg, graphEdges)
	links := newLinkReconcileWorker(pool, linkReconcileStore(pool), cfg.CaptureConfig, log)
	addDeclaredWorker[LinkReconcileArgs](reg, links)
	rematch := newLinkedInRematchWorker(pool, newContactsStore(InstallationDB(pool)), identity.NewService(pool), log)
	addDeclaredWorker[LinkedInRematchArgs](reg, rematch)
	addDeclaredWorker[ReplySettledStampArgs](reg, &replySettledWorker{pool: pool, store: activities.NewStore(InstallationDB(pool))})
	return slices.Concat(
		periodicFor(cfg, ParticipantBackfillArgs{}),
		periodicFor(cfg, GraphEdgeReconcileArgs{}),
		periodicFor(cfg, LinkedInRematchArgs{}),
		periodicFor(cfg, LinkReconcileArgs{}),
		periodicFor(cfg, ReplySettledStampArgs{}),
	)
}

// ReplySettledStampArgs is the pass that stamps when a same-thread fact settled
// each inbound message. The database clears a stamp whenever a write can change it.
type ReplySettledStampArgs struct{}

// Kind is the River job kind for the stamping pass.
func (ReplySettledStampArgs) Kind() string { return "reply_settled_stamp" }

// FleetWide marks the pass as walking every workspace itself.
func (ReplySettledStampArgs) FleetWide() {}

// One pass stamps at most this many batches of this many messages per workspace;
// the rest wait for the next pass, which keeps no cursor.
const replySettledBatch, replySettledBatchesPerTick = 500, 20

type replySettledWorker struct {
	pool  *pgxpool.Pool
	store *activities.Store
}

func (w *replySettledWorker) Work(ctx context.Context, _ *river.Job[ReplySettledStampArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.stampWorkspace))
}

func (w *replySettledWorker) stampWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.WithActor(principal.WithWorkspaceID(ctx, workspace), principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:reply_settled_stamp",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	for range replySettledBatchesPerTick {
		if n, err := w.store.StampReplySettledBatch(ctx, replySettledBatch); err != nil || n < replySettledBatch {
			return jobs.FaultContext(ctx, err)
		}
	}
	return nil
}
