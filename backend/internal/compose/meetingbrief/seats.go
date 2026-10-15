// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// Who from OUR side is in the room.
//
// `activity_participant.user_id` is the seat a colleague holds in a meeting, as
// against `contact_id`, which is the counterparty. The coaching projection needs
// the first: it is deciding whether the reader is a lead looking at somebody
// else's meeting, and a room full of the buyer's contacts says nothing about that.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readSeats returns the colleagues seated in this meeting, through the same
// read that fills an activity's colleague_ids.
//
// It runs inside the meeting read, after EnsureActivityContentVisibleLive has
// already decided this caller may see this meeting at all.
func (s *Service) readSeats(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]ids.UUID, error) {
	seats, err := activities.ColleagueSeats(ctx, tx, activityID)
	if err != nil {
		return nil, fmt.Errorf("read the meeting's seats: %w", err)
	}
	return seats, nil
}
