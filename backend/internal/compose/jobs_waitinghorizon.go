// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// waitingHorizonRefreshActor is the principal the pass reads the colleague
// domains as; the store's reads are gated and a job carries no human.
const waitingHorizonRefreshActor = "system:waiting_horizon_refresh"

// WaitingHorizonRefreshArgs schedules one measurement of the waiting horizon,
// stored for every reader of the installation.
type WaitingHorizonRefreshArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (WaitingHorizonRefreshArgs) Kind() string { return "waiting_horizon_refresh" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
func (WaitingHorizonRefreshArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

// newWaitingHorizonRefreshWorker binds the colleague-domain reader the
// measurement needs. Unbound, colleagues' fast replies would count as customers
// being answered and pull the stored horizon down.
func newWaitingHorizonRefreshWorker(pool *pgxpool.Pool, log *slog.Logger) *waitingHorizonRefreshWorker {
	db := InstallationDB(pool)
	return &waitingHorizonRefreshWorker{
		activities: activities.NewStore(db).WithOwnDomains(
			ownDomainReader{store: capture.NewOwnDomainStore(db)}),
		identity: identity.NewService(pool),
		log:      log,
	}
}

// waitingHorizonStatementBudget is the job's declared timeout less a margin for
// the own-domain read and the store. Postgres then cuts the statement with a
// clean error before River abandons the attempt.
func waitingHorizonStatementBudget() time.Duration {
	spec, ok := jobs.SpecFor(WaitingHorizonRefreshArgs{}.Kind())
	if !ok {
		panic("compose: waiting_horizon_refresh has no declared job spec")
	}
	return spec.Timeout.Duration() - waitingHorizonBudgetMargin
}

const waitingHorizonBudgetMargin = 30 * time.Second

type waitingHorizonRefreshWorker struct {
	activities *activities.Store
	identity   *identity.Service
	log        *slog.Logger
}

func (w *waitingHorizonRefreshWorker) Work(ctx context.Context, _ *river.Job[WaitingHorizonRefreshArgs]) error {
	wsCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	days, err := w.activities.RefreshWaitingHorizon(
		principal.SystemActing(wsCtx, waitingHorizonRefreshActor), waitingHorizonStatementBudget())
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	w.log.InfoContext(wsCtx, "waiting horizon refresh: horizon stored", "days", days)
	return nil
}
