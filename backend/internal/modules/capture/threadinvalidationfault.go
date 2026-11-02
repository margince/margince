// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

func invalidateThreadContentUnitTx(ctx context.Context, tx pgx.Tx, component string, apply ThreadRepairApplier) (bool, error) {
	in, found, err := selectThreadInvalidationTx(ctx, tx, component)
	if err != nil || !found {
		return false, err
	}
	unit, err := tx.Begin(ctx)
	if err != nil {
		return false, err
	}
	cause := invalidateThreadContentTx(ctx, unit, in, apply)
	if cause == nil {
		return true, unit.Commit(ctx)
	}
	if err := unit.Rollback(ctx); err != nil {
		return false, fmt.Errorf("capture: rollback failed privacy repair unit: %w", err)
	}
	if ctx.Err() != nil {
		return false, cause
	}
	var attempts int
	err = tx.QueryRow(ctx, `UPDATE capture_thread_invalidation SET `+threadFaultAssignments+` WHERE activity_id=@id RETURNING failures`, pgx.NamedArgs{"id": in.Activity}).Scan(&attempts)
	if err != nil {
		return false, err
	}
	slog.ErrorContext(ctx, "conversation privacy repair deferred after a failed unit", "activity_id", in.Activity, "attempts", attempts, "quarantined", attempts == 3, "error", cause)
	return true, nil
}
