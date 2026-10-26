// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// evidenceAuthorshipActor is the principal the pass corrects evidence as.
const evidenceAuthorshipActor = "system:evidence_authorship"

// EvidenceAuthorshipArgs schedules one pass that judges the author of every
// live activity-backed stage evidence row again, against the current own
// domains.
type EvidenceAuthorshipArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (EvidenceAuthorshipArgs) Kind() string { return "evidence_authorship" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
func (EvidenceAuthorshipArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

// newEvidenceAuthorshipWorker binds the own-domain reader every author
// judgement reads, the one the evidence writers use at the write.
func newEvidenceAuthorshipWorker(pool *pgxpool.Pool, log *slog.Logger) *evidenceAuthorshipWorker {
	db := InstallationDB(pool)
	return &evidenceAuthorshipWorker{
		deals:    deals.NewStore(db, DealsInstallation()),
		own:      ownDomainReader{store: capture.NewOwnDomainStore(db)},
		identity: identity.NewService(pool),
		log:      log,
	}
}

type evidenceAuthorshipWorker struct {
	deals    *deals.Store
	own      deals.OwnDomainReader
	identity *identity.Service
	log      *slog.Logger
}

func (w *evidenceAuthorshipWorker) Work(ctx context.Context, _ *river.Job[EvidenceAuthorshipArgs]) error {
	wsCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	corrected, err := w.deals.ReconcileEvidenceAuthorship(principal.SystemActing(wsCtx, evidenceAuthorshipActor), w.own, deals.EvidenceAuthorshipScope{})
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	w.log.InfoContext(wsCtx, "evidence authorship: pass finished", "corrected", corrected)
	return nil
}
