// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const threadFaultAssignments = `failures=least(failures+1,3),state=CASE WHEN failures>=2 THEN 'quarantined' ELSE 'backoff' END,retry_after=statement_timestamp()+interval '1 minute'`

func isolatedThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, apply ThreadRepairApplier) (bool, error) {
	unit, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	moved, cause := threadRepairUnitTx(ctx, unit, job, apply)
	if cause == nil {
		return moved, unit.Commit(ctx)
	}
	if err := unit.Rollback(ctx); err != nil {
		return false, fmt.Errorf("capture: rollback failed repair unit: %w", err)
	}
	if ctx.Err() != nil {
		return false, cause
	}
	var attempts int
	err = tx.QueryRow(ctx, `UPDATE capture_thread_repair SET `+threadFaultAssignments+` WHERE id=@job RETURNING failures`, pgx.NamedArgs{threadArgJob: job.ID}).Scan(&attempts)
	if err != nil {
		return false, err
	}
	slog.ErrorContext(ctx, "conversation repair deferred after a failed unit", "repair_id", job.ID, "attempts", attempts, "quarantined", attempts == 3, "error", cause)
	return false, nil
}

func resumeThreadRepairTx(ctx context.Context, tx pgx.Tx, component string) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE capture_thread_repair SET state='ready',retry_after=NULL WHERE id=(SELECT id FROM capture_thread_repair WHERE state='backoff' AND retry_after<=statement_timestamp() AND NOT (`+threadRepairInvalidationSQL("capture_thread_repair.id")+`) AND (@component='' OR EXISTS(SELECT 1 FROM capture_thread_barrier b WHERE b.repair_id=capture_thread_repair.id AND b.blocked_key=@component)) ORDER BY retry_after,id LIMIT 1)`, pgx.NamedArgs{threadArgComponent: component})
	return tag.RowsAffected() > 0, err
}

// RetryThreadRepairTx rearms one quarantined component through an authorized correction retry.
func RetryThreadRepairTx(ctx context.Context, tx pgx.Tx) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "data_task", principal.ActionUpdate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE capture_thread_invalidation SET failures=0,state='ready',retry_after=NULL WHERE activity_id=(SELECT activity_id FROM capture_thread_invalidation WHERE state='quarantined' ORDER BY activity_id LIMIT 1)`)
	if err != nil || tag.RowsAffected() > 0 {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair SET failures=0,state='backoff',retry_after=statement_timestamp() WHERE id=(SELECT id FROM capture_thread_repair WHERE state='quarantined' ORDER BY id LIMIT 1)`)
	return err
}
