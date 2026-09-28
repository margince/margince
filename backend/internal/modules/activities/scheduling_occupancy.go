// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func localMeetingSlots(ctx context.Context, tx pgx.Tx, host ids.UserID, from, to time.Time, exclude ids.UUID) ([]slot, error) {
	args := schedulingArgs{}
	h, f, t, id := args.add(host), args.add(from), args.add(to), args.add(exclude)
	rows, err := tx.Query(ctx, `SELECT occurred_at,occurred_at+(CASE WHEN booking_interval_exact THEN duration_seconds ELSE 3600 END)*interval '1 second'
 FROM activity a WHERE host_user_id=`+h+` AND id<>`+id+` AND kind='meeting' AND archived_at IS NULL
 AND (claims_host_slot OR EXISTS(SELECT 1 FROM meeting_invitation m WHERE m.activity_id=a.id))
 AND meeting_status IS DISTINCT FROM 'canceled' AND occurred_at<`+t+`
 AND occurred_at+(CASE WHEN booking_interval_exact THEN duration_seconds ELSE 3600 END)*interval '1 second'>`+f+`
 UNION ALL SELECT (appointment->>'Start')::timestamptz,(appointment->>'End')::timestamptz
 FROM meeting_invitation WHERE host_user_id=`+h+` AND activity_id<>`+id+`
 AND command='update' AND status IN ('rescheduling','needs_attention')
 AND (appointment->>'Start')::timestamptz<`+t+` AND (appointment->>'End')::timestamptz>`+f, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []slot{}
	for rows.Next() {
		var interval slot
		if err := rows.Scan(&interval.Start, &interval.End); err != nil {
			return nil, err
		}
		result = append(result, interval)
	}
	return result, rows.Err()
}
