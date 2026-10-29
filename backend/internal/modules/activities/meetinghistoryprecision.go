// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The backfill for activity_meeting_history.scheduled_precision. A row
// recorded before the column has none. One kind of row has kept proof of it:
// a row the noon repair restored to an instant. Every other old row stays
// unknown, including a row that merely holds the same instant.

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
//
// The repair rewrote the rows holding the noon start, and audited both starts.
// A row held that noon if it was written between the outcome save that set it
// and the repair, with no other start change between. The archived path also
// records its own transition, under the repair's actor and transaction.
// created_at and audit occurred_at are both the transaction's now().
const proveRestoredInstantsSQL = `
	WITH page AS (
	    SELECT h.id, h.activity_id, h.scheduled_start, h.created_at, h.actor
	      FROM activity_meeting_history h
	     WHERE h.scheduled_precision IS NULL AND h.scheduled_start IS NOT NULL AND h.id > $1
	     ORDER BY h.id
	     LIMIT $2
	), restored AS (
	    SELECT page.id
	      FROM page
	      JOIN audit_log repair
	        ON repair.entity_type = 'activity' AND repair.entity_id = page.activity_id
	       AND repair.action = 'update' AND repair.actor_type = 'system' AND repair.actor_id = $3
	       AND repair.after ->> 'occurred_precision' = $4
	       AND (repair.after ->> 'occurred_at')::timestamptz = page.scheduled_start
	     WHERE (page.actor = $3 AND page.created_at = repair.occurred_at)
	        OR EXISTS (
	           SELECT 1 FROM audit_log damage
	            WHERE damage.entity_type = 'activity' AND damage.entity_id = page.activity_id
	              AND damage.action = 'update'
	              AND (damage.before ->> 'occurred_at')::timestamptz = page.scheduled_start
	              AND (damage.after ->> 'occurred_at')::timestamptz = (repair.before ->> 'occurred_at')::timestamptz
	              AND damage.occurred_at <= page.created_at AND page.created_at < repair.occurred_at
	              AND NOT EXISTS (
	                  SELECT 1 FROM audit_log moved
	                   WHERE moved.entity_type = 'activity' AND moved.entity_id = page.activity_id
	                     AND moved.after ? 'occurred_at'
	                     AND moved.occurred_at > damage.occurred_at AND moved.occurred_at < repair.occurred_at))
	), proved AS (
	    UPDATE activity_meeting_history h
	       SET scheduled_precision = $4
	      FROM restored
	     WHERE h.id = restored.id AND h.scheduled_precision IS NULL
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
