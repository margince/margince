// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lockedDeal is the deal as the sweep's write finds it under the row lock. A
// correction's audit before-image comes from it, never from the page.
type lockedDeal struct {
	status        string
	version       int64
	ownerID       *ids.UUID
	stageID       ids.UUID
	expectedClose *time.Time
	provisional   bool
	override      *int
	overrideBy    *string
}

func readLockedDeal(ctx context.Context, tx pgx.Tx, id ids.DealID) (lockedDeal, error) {
	var row lockedDeal
	err := tx.QueryRow(ctx, `
		SELECT status, version, owner_id, stage_id, expected_close_date, close_date_provisional,
		       win_probability_override, win_probability_set_by
		  FROM deal WHERE id = $1`, id).Scan(&row.status, &row.version, &row.ownerID, &row.stageID,
		&row.expectedClose, &row.provisional, &row.override, &row.overrideBy)
	if err != nil {
		return lockedDeal{}, fmt.Errorf("deals: reading the deal to correct: %w", err)
	}
	return row, nil
}

// stillAssessed reports whether the locked row and the horizon setting still
// hold what the assessment read from the page. The page and the write are
// separate transactions, and a colleague's edit in between wins.
func (c closeDateCandidate) stillAssessed(ctx context.Context, tx pgx.Tx, row lockedDeal) (bool, error) {
	if !sameOwner(c.ownerID, row.ownerID) || c.stageID != row.stageID || !sameDate(c.expectedClose, row.expectedClose) ||
		c.provisional != row.provisional || !samePtr(c.override, row.override) || !samePtr(c.overrideBy, row.overrideBy) {
		return false, nil
	}
	horizon, err := config.StoredApplyTx(ctx, tx, config.Bound(), SilentCloseDays)
	if err != nil {
		return false, fmt.Errorf("deals: reading the silent close days the estimate stops at: %w", err)
	}
	return horizon == c.horizonDays, nil
}
