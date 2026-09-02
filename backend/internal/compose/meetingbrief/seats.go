// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Who from OUR side is in the room.
//
// `activity_participant.user_id` is the seat a colleague holds in a meeting, as
// against `person_id`, which is the counterparty. The coaching projection needs
// the first: it is deciding whether the reader is a lead looking at somebody
// else's meeting, and a room full of the buyer's people says nothing about that.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const seatsQuery = `
	SELECT DISTINCT ap.user_id
	FROM activity_participant ap
	WHERE ap.activity_id = $1 AND ap.user_id IS NOT NULL`

// readSeats returns the colleagues seated in this meeting.
//
// No scope clause of its own: it runs inside the meeting read, after
// EnsureActivityContentVisibleLive has already decided this caller may see this
// meeting at all. What it returns is who is IN it, which is the same fact the
// attendees section prints.
func (s *Service) readSeats(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, seatsQuery, activityID)
	if err != nil {
		return nil, fmt.Errorf("read the meeting's seats: %w", err)
	}
	defer rows.Close()

	var seats []ids.UUID
	for rows.Next() {
		var seat ids.UUID
		if err := rows.Scan(&seat); err != nil {
			return nil, fmt.Errorf("read the meeting's seats: %w", err)
		}
		seats = append(seats, seat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the meeting's seats: %w", err)
	}
	return seats, nil
}
