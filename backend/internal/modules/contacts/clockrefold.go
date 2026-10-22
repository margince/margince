// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Refolding the contact and company last-activity clocks outside a write: when
// an activity's time comes, and where an older rule stored a future clock.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// RefoldClocksTx moves the clock of every contact and company a write filed
// under targets reaches. The caller has taken storekit.LockLastActivityTargets
// for the whole set.
func RefoldClocksTx(ctx context.Context, tx pgx.Tx, targets storekit.LastActivityTargets) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		SELECT move_last_activity(reach.kind::regclass, reach.id)
		  FROM last_activity_reach(coalesce($1::uuid[], '{}'), coalesce($2::uuid[], '{}'), coalesce($3::uuid[], '{}')) reach
		 WHERE reach.kind IN ('contact', 'company')
		 ORDER BY reach.kind, reach.id`,
		targets.Contacts, targets.Deals, targets.Companies); err != nil {
		return fmt.Errorf("contacts: refolding last-activity clocks: %w", err)
	}
	return nil
}
