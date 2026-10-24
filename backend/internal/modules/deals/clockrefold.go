// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefoldClocksTx moves the last-activity clock of each deal named, outside any
// write to it. It runs when an activity's time comes, or where an older rule
// stored a future clock. The caller holds storekit.LockLastActivityTargets.
func RefoldClocksTx(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		SELECT move_last_activity('deal'::regclass, named)
		  FROM unnest(coalesce($1::uuid[], '{}')) named
		 ORDER BY named`, dealIDs); err != nil {
		return fmt.Errorf("deals: refolding last-activity clocks: %w", err)
	}
	return nil
}
