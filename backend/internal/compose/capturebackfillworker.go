// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CaptureBackfillArgs identifies one durable history page execution.
type CaptureBackfillArgs struct {
	BackfillID          string `json:"backfill_id"`
	ExecutionGeneration int64  `json:"execution_generation,omitempty"`
}

// Kind is the persisted River job identifier.
func (CaptureBackfillArgs) Kind() string { return "capture_backfill" }

type captureBackfillWorker struct {
	pool     *pgxpool.Pool
	registry *capture.Registry
	log      *slog.Logger
}

// A fresh job context for each page prevents one slow page consuming the next page's deadline.
const backfillPagesPerTick = 1

const backfillPageYield = time.Second

func (w *captureBackfillWorker) Work(ctx context.Context, job *river.Job[CaptureBackfillArgs]) error {
	bfID, err := ids.Parse(job.Args.BackfillID)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("capture_backfill: backfill id: %w", err))
	}
	wsCtx, err := bindInstallation(ctx, w.pool)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = capturemetrics.WithRun(wsCtx)
	for range backfillPagesPerTick {
		done, _, retryAfter, err := w.registry.RunBackfillExecution(wsCtx, bfID, job.Args.ExecutionGeneration, backfillScheduler())
		if _, snoozed := errors.AsType[*river.JobSnoozeError](err); snoozed {
			return jobs.FaultContext(ctx, err)
		}
		if retryAfter > 0 {
			if err != nil {
				w.log.WarnContext(ctx, "capture backfill page deferred", "backfill", job.Args.BackfillID,
					"retry_after", retryAfter, "err", err, rateLimitAttr(err))
				capturemetrics.ObserveDeferral(wsCtx, retryAfter, err)
			}
			if done {
				return nil
			}
			return river.JobSnooze(retryAfter)
		}
		if err != nil {
			w.log.WarnContext(ctx, "capture backfill page failed", "backfill", job.Args.BackfillID, "err", err)
			if w.resumedMeanwhile(wsCtx, bfID) {
				capturemetrics.ObserveResumed(wsCtx, backfillPageYield)
				return river.JobSnooze(backfillPageYield)
			}
			return nil
		}
		if done {
			return nil
		}
	}
	capturemetrics.ObservePacing(wsCtx, backfillPageYield)
	return river.JobSnooze(backfillPageYield)
}

// Continue can race a finishing job; a queued run needs another admission request.
func (w *captureBackfillWorker) resumedMeanwhile(ctx context.Context, bfID ids.UUID) bool {
	reopened, err := w.registry.BackfillReopened(ctx, bfID)
	return err == nil && reopened
}
