// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The embed drift sweep (ADR-0069 §3a, SEARCH-AC-13): the at-least-once
// bus loses embed events — a worker dies between ack and write — and the
// lost entities would otherwise sit invisible to semantic search until a
// human confirmed a reindex they never caused. Re-embedding them is the
// same spend class as the event lane that missed them, so the sweep heals
// them without a confirm. The binding-change case (configured identity ≠
// populated identity) is NOT this sweep's to touch — the store method
// no-ops there and the preview→confirm flow in embedreindextransport.go
// keeps its human consent.

// EmbedDriftSweepArgs is the periodic drift-sweep job (no payload — the
// sweep derives everything from the binding marker and the pending scan).
type EmbedDriftSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (EmbedDriftSweepArgs) Kind() string { return "embed_drift_sweep" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (EmbedDriftSweepArgs) FleetWide() {}

// embedDriftSweepWorker heals every live workspace's drift.
//
// One worker where there were two (ADR-0103): the dispatcher that enqueued an
// embed_drift_workspace child per workspace is gone, and this walks them.
type embedDriftSweepWorker struct {
	pool  *pgxpool.Pool
	store *search.Store
	// corpora is the knowledge module's half of the same question. It rides
	// this sweep rather than a job of its own: the sweep already answers "has
	// the live identity moved", and a second one would be a second answer to
	// one question — the two would disagree the first time either changed.
	corpora  *knowledge.Store
	embedder search.Embedder
	log      *slog.Logger
}

func (w *embedDriftSweepWorker) Work(ctx context.Context, _ *river.Job[EmbedDriftSweepArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.sweepWorkspace))
}

// sweepWorkspace heals ONE workspace. It was the child job's Work, and binds
// the workspace itself: the pass walks tenants, so the binding belongs to the
// tenant's turn rather than to the row.
func (w *embedDriftSweepWorker) sweepWorkspace(ctx context.Context, workspace ids.UUID) error {
	wsCtx := principal.WithWorkspaceID(ctx, workspace)
	healed, err := w.store.SweepWorkspaceEmbeddingDrift(wsCtx, ids.From[ids.WorkspaceKind](workspace), w.embedder)
	if healed > 0 {
		w.log.InfoContext(wsCtx, "embed drift sweep healed entities", "healed", healed)
	}
	// The corpora are swept even when the entity pass healed nothing: the two
	// hold their vectors in different tables and drift independently. A corpus
	// left under a superseded binding retrieves NOTHING — the identity filter
	// excludes every passage — and no other path re-embeds an existing
	// document, so this is the only way back.
	//
	// Run even when the ENTITY pass failed, and deliberately. Coupling them
	// sequentially would let one bad search entity keep every corpus in the
	// workspace bricked after a binding swap, which is a far larger failure
	// than the one being retried — the two sweeps share a question, not a fate.
	// Closed BEFORE the drift repair, because an abandoned document holds the
	// whole corpus not_ready and the repair cannot lift that: readiness reads
	// `running` as in-flight whatever the vectors say.
	closed, aerr := w.corpora.SweepAbandonedIngests(wsCtx)
	if closed > 0 {
		w.log.InfoContext(wsCtx, "closed corpus ingests that stopped without finishing", "closed", closed)
	}
	repaired, cerr := w.corpora.SweepCorpusDrift(wsCtx, w.embedder)
	if repaired > 0 {
		w.log.InfoContext(wsCtx, "embed drift sweep re-embedded corpus passages", "repaired", repaired)
	}
	// Every failure reaches River, and none of the three is allowed to hide the
	// others. Logging one and returning nil would report the job as successful,
	// so nothing retries and the next tick starts from the same place — which
	// for the abandoned-ingest sweep means the corpus it was going to unblock
	// stays not_ready with a green job behind it.
	//
	// The entity pass is named FIRST when several failed: it is the one with a
	// retry budget behind it, and a reader looking for what to do sees it
	// before the two that ride along.
	return jobs.FaultContext(ctx, errors.Join(err, aerr, cerr))
}

// addEmbedDriftSweepJob registers the sweep worker and its periodic tick
// (NewJobRunner appends what it returns — addGraphJobs' self-registration
// shape).
//
// The guard here is STRICTER than the declaration's when: [Embedder], and
// deliberately so. A configured embed lane with an empty identity — what
// --ai-fake leaves behind — seeds no binding marker, so there is nothing for
// the sweep to compare a row against and no store to heal; that is a property
// of the bound lane rather than of the configuration, so no config field can
// express it. periodicFor still resolves the cadence and the nil-embedder
// posture, which is what keeps this to ONE extra condition.
func addEmbedDriftSweepJob(reg *jobRegistry, pool *pgxpool.Pool, cfg JobRunnerConfig, log *slog.Logger) []*river.PeriodicJob {
	embedder := cfg.Embedder
	if embedder == nil {
		return nil
	}
	if identity, _ := embedder.EmbedIdentity(); identity == "" {
		return nil
	}
	addDeclaredWorker[EmbedDriftSweepArgs](reg, &embedDriftSweepWorker{
		pool:  pool,
		store: search.NewStore(InstallationDB(pool)),
		// WithBlobstore, or the abandoned-ingest sweep marks a document failed
		// and leaves its stored file behind: FailIngest deletes the bytes
		// through this store, and a nil one skips that silently.
		corpora:  knowledge.NewStore(InstallationDB(pool)).WithBlobstore(cfg.Blobstore),
		embedder: embedder,
		log:      log,
	})
	return periodicFor(cfg, EmbedDriftSweepArgs{})
}
