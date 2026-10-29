// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The backfill for activity_meeting_history.scheduled_precision. A row
// recorded before the column has none, and one kind of row has kept proof of
// it: a start the noon repair put back. That task audited each restore with
// the instant it restored, so a history row of that meeting holding that
// instant holds an instant. Every other old row stays unknown.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MeetingPrecisionPage is one inspected page: the history rows looked at and
// how many of them were proved instants.
type MeetingPrecisionPage struct {
	Scanned []ids.UUID
	Proved  int
}

// proveRestoredInstantsSQL takes $1 as the cursor, $2 the page size, $3 the
// repair's actor and $4 the instant precision.
const proveRestoredInstantsSQL = `
	WITH page AS (
	    SELECT h.id, h.activity_id, h.scheduled_start
	      FROM activity_meeting_history h
	     WHERE h.scheduled_precision IS NULL AND h.scheduled_start IS NOT NULL AND h.id > $1
	     ORDER BY h.id
	     LIMIT $2
	), proved AS (
	    UPDATE activity_meeting_history h
	       SET scheduled_precision = $4
	      FROM page
	     WHERE h.id = page.id AND h.scheduled_precision IS NULL
	       AND EXISTS (
	           SELECT 1 FROM audit_log au
	            WHERE au.entity_type = 'activity' AND au.entity_id = page.activity_id
	              AND au.action = 'update' AND au.actor_type = 'system' AND au.actor_id = $3
	              AND au.after ->> 'occurred_precision' = $4
	              AND (au.after ->> 'occurred_at')::timestamptz = page.scheduled_start)
	    RETURNING h.id
	)
	SELECT page.id, EXISTS (SELECT 1 FROM proved p WHERE p.id = page.id)
	  FROM page ORDER BY page.id`

// ProveRestoredInstantsTx marks, in one page after the cursor, the history
// rows whose start the noon repair restored as an instant. repairActor is
// the actor that repair's audit rows name. It writes only the precision.
func ProveRestoredInstantsTx(ctx context.Context, tx pgx.Tx, repairActor string, after ids.UUID, limit int) (MeetingPrecisionPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return MeetingPrecisionPage{}, err
	}
	rows, err := tx.Query(ctx, proveRestoredInstantsSQL, after, limit, repairActor, PrecisionInstant)
	if err != nil {
		return MeetingPrecisionPage{}, fmt.Errorf("activities: proving restored meeting starts: %w", err)
	}
	defer rows.Close()
	var page MeetingPrecisionPage
	for rows.Next() {
		var id ids.UUID
		var proved bool
		if err := rows.Scan(&id, &proved); err != nil {
			return MeetingPrecisionPage{}, fmt.Errorf("activities: reading a meeting history row: %w", err)
		}
		page.Scanned = append(page.Scanned, id)
		if proved {
			page.Proved++
		}
	}
	if err := rows.Err(); err != nil {
		return MeetingPrecisionPage{}, fmt.Errorf("activities: proving restored meeting starts: %w", err)
	}
	return page, nil
}

// EstimateUnknownMeetingPrecision counts, up to limit+1, the rows
// ProveRestoredInstantsTx inspects: every start with no recorded precision.
func EstimateUnknownMeetingPrecision(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM activity_meeting_history WHERE scheduled_precision IS NULL AND scheduled_start IS NOT NULL LIMIT $1) unknown`,
		limit+1).Scan(&count)
	return count, err
}
