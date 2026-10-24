// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefoldClocksTx moves the last-activity clock of each project named, outside
// any write to it. It repairs a clock an older rule stored. The caller holds
// storekit.LockLastActivityTargets.
func RefoldClocksTx(ctx context.Context, tx pgx.Tx, projectIDs []ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		SELECT move_project_last_activity(named)
		  FROM unnest(coalesce($1::uuid[], '{}')) named
		 ORDER BY named`, projectIDs); err != nil {
		return fmt.Errorf("projects: refolding last-activity clocks: %w", err)
	}
	return nil
}
