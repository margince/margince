// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// clearedDuration is one activity whose negative duration became unknown.
type clearedDuration struct {
	ID       ids.UUID
	Negative int64
}

// negativeDurationScope is the rows the correction may write. A restricted row
// is immutable until its lift, and the lift's update clears its duration then.
const negativeDurationScope = `duration_seconds < 0 AND restricted_at IS NULL`

// CountNegativeDurations counts the activities still holding a negative
// duration, up to maximum plus one.
func CountNegativeDurations(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var left int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM activity WHERE `+negativeDurationScope+` LIMIT $1) negative`,
		maximum+1).Scan(&left); err != nil {
		return 0, false, fmt.Errorf("activities: counting negative durations: %w", err)
	}
	return min(left, maximum), left > maximum, nil
}

// ClearNegativeDurationsTx sets the next page of negative durations to NULL:
// the real duration was never known, and zero would claim it took no time. A
// cleared row leaves the page, so a replay finds nothing to change. The
// activity event names no duration, so the audit row alone records it.
func ClearNegativeDurationsTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `WITH page AS (
		  SELECT id, duration_seconds FROM activity
		   WHERE `+negativeDurationScope+` AND id > $1 ORDER BY id LIMIT $2 FOR UPDATE),
		cleared AS (UPDATE activity a SET duration_seconds = NULL FROM page WHERE a.id = page.id
		  RETURNING a.id, page.duration_seconds)
		SELECT id, duration_seconds FROM cleared ORDER BY id`, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: clearing negative durations: %w", err)
	}
	cleared, err := pgx.CollectRows(rows, pgx.RowToStructByPos[clearedDuration])
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: clearing negative durations: %w", err)
	}
	page := make([]ids.UUID, 0, len(cleared))
	for _, row := range cleared {
		if _, err := storekit.Audit(ctx, tx, "update", "activity", row.ID,
			map[string]any{auditFieldDuration: row.Negative}, map[string]any{auditFieldDuration: nil}); err != nil {
			return datatask.BatchOut{}, fmt.Errorf("activities: auditing a cleared duration: %w", err)
		}
		page = append(page, row.ID)
	}
	return datatask.AdvanceUUID(in, page, len(page))
}
