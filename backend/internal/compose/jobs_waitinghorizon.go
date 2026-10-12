// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

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
	days, err := w.activities.RefreshWaitingHorizon(wsCtx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	w.log.InfoContext(wsCtx, "waiting horizon refresh: horizon stored", "days", days)
	return nil
}
