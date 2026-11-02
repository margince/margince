// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingOrganizerQuery picks, per meeting, the seat its organizer participant
// is bound to. Capture binds an organizer's address to a seat only when that
// seat alone proved it, and a meeting two calendars disagree on names nobody.
const meetingOrganizerQuery = `
	SELECT op.activity_id, (array_agg(DISTINCT op.user_id))[1]
	  FROM activity_participant op
	  JOIN activity a ON a.id = op.activity_id AND a.kind = 'meeting' AND ` + activityLive + ` AND %s
	 WHERE op.activity_id = ANY($%d) AND op.role = 'organizer' AND op.user_id IS NOT NULL
	 GROUP BY op.activity_id
	HAVING count(DISTINCT op.user_id) = 1`

// MeetingOrganizers answers which colleague organized each of these meetings,
// for this caller. Outside organizers and unreadable meetings are absent. This
// differs from host_user_id, which names whose calendar synced the meeting
// first: often an invited colleague's.
func (s *Store) MeetingOrganizers(ctx context.Context, meetingIDs []ids.UUID) (map[ids.UUID]ids.UUID, error) {
	out := make(map[ids.UUID]ids.UUID, len(meetingIDs))
	if len(meetingIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	query := storekit.SQLf(meetingOrganizerQuery, orUnbounded(activityScope), arg(meetingIDs))
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("activities: reading who organized the meetings: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var meeting, organizer ids.UUID
			if err := rows.Scan(&meeting, &organizer); err != nil {
				return fmt.Errorf("activities: reading who organized a meeting: %w", err)
			}
			out[meeting] = organizer
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
