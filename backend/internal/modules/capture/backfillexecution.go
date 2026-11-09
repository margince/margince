// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type backfillStepOutcome struct {
	done, completed, scheduled bool
	retryAfter                 time.Duration
	err                        error
}

// RunBackfillStep is the synchronous adapter for callers that drive pages themselves.
func (r *Registry) RunBackfillStep(ctx context.Context, id ids.UUID) (done, completed bool, retryAfter time.Duration, err error) {
	scheduler := BackfillScheduler{
		Enqueue:   func(context.Context, pgx.Tx, ids.UUID, int64, time.Time) error { return nil },
		Executing: func(context.Context, pgx.Tx, ids.UUID, int64) (bool, error) { return false, nil },
	}
	if err := r.DispatchBackfills(ctx, scheduler, false); err != nil {
		return false, false, 0, err
	}
	var generation int64
	var status string
	args := []any{id}
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT execution_generation,status FROM capture_backfill WHERE id=`+storekit.Placeholders(args), args...).Scan(&generation, &status)
	})
	if err != nil {
		return true, false, 0, err
	}
	if status == backfillStatusQueued {
		return false, false, time.Second, nil
	}
	if status != backfillStatusRunning {
		return true, false, 0, nil
	}
	result := r.runAdmittedBackfill(ctx, id, generation, scheduler)
	return result.done, result.completed, result.retryAfter, result.err
}

// RunBackfillExecution settles one generation; its successor commits with the page's saved position.
func (r *Registry) RunBackfillExecution(ctx context.Context, id ids.UUID, generation int64, scheduler BackfillScheduler) (done, completed bool, retryAfter time.Duration, err error) {
	if generation == 0 {
		return true, false, 0, r.DispatchBackfills(ctx, scheduler, false)
	}
	result := r.runAdmittedBackfill(ctx, id, generation, scheduler)
	if dispatchErr := r.DispatchBackfills(ctx, scheduler, false); dispatchErr != nil {
		result.err = errors.Join(result.err, dispatchErr)
	}
	if result.scheduled {
		if result.err == nil && !result.done {
			capturemetrics.ObservePacing(ctx, backfillPageDelay)
		}
		return true, result.completed, result.retryAfter, result.err
	}
	return result.done, result.completed, result.retryAfter, result.err
}

func (r *Registry) runAdmittedBackfill(ctx context.Context, id ids.UUID, generation int64, scheduler BackfillScheduler) backfillStepOutcome {
	if r.backfillHeld {
		return backfillStepOutcome{retryAfter: 15 * time.Second}
	}
	pageCtx, release, held, err := r.leaseBackfill(ctx, id, generation, scheduler)
	if errors.Is(err, pgx.ErrNoRows) {
		return backfillStepOutcome{done: true}
	}
	if err != nil {
		return backfillStepOutcome{err: err}
	}
	if !held {
		return backfillStepOutcome{retryAfter: time.Second}
	}
	defer release()
	pageCtx, stopMonitoring := r.monitorBackfill(pageCtx)
	defer stopMonitoring()
	// The run marks its events as imported, so a consumer that acts only on
	// news (workflows, introductions, stage evidence) leaves history alone.
	pageCtx = storekit.WithImportRun(storekit.WithBatch(pageCtx, id), id)
	result := backfillStepOutcome{}
	result.done, result.completed, result.retryAfter, result.err = r.executeBackfillPage(pageCtx, id)
	if errors.Is(result.err, apperrors.ErrConflict) && ctx.Err() == nil {
		var cancelled bool
		args := []any{id}
		readErr := r.db.Tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT status='cancelled' FROM capture_backfill WHERE id=`+storekit.Placeholders(args), args...).Scan(&cancelled)
		})
		result.err = errors.Join(result.err, readErr)
		if readErr == nil && cancelled {
			result.done, result.completed, result.retryAfter, result.err = true, false, 0, nil
		}
	}

	execution, ok := pageCtx.Value(backfillExecutionKeyType{}).(backfillExecution)
	if ok {
		result.scheduled = *execution.scheduled
	}
	return result
}
