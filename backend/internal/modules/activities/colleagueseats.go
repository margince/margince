// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ColleagueSeats returns the colleagues recorded as taking part in one
// activity: the participant rows that name a seat rather than a contact. A
// blind copy is left out, as the email detail read leaves it out for everyone
// but the sender.
//
// It has no scope clause of its own. Each caller runs it after deciding the
// reader may see this activity's content.
func ColleagueSeats(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ap.user_id
		  FROM activity_participant ap
		 WHERE ap.activity_id = $1 AND ap.user_id IS NOT NULL AND ap.role <> $2
		 ORDER BY ap.user_id`, activityID, connector.ParticipantRoleBCC)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the colleagues on an activity: %w", err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading the colleagues on an activity: %w", err)
	}
	return seats, nil
}

// attachColleagues fills colleague_ids and guests on one activity read whole. A
// withheld row gets neither: who sat in a meeting is part of what its audience limits.
func attachColleagues(ctx context.Context, tx pgx.Tx, a *crmcontracts.Activity) error {
	if a.ContentState != nil && *a.ContentState == crmcontracts.ActivityContentStateWithheld {
		return nil
	}
	seats, err := ColleagueSeats(ctx, tx, ids.UUID(a.Id))
	if err != nil {
		return err
	}
	out := make([]openapi_types.UUID, len(seats))
	for i, seat := range seats {
		out[i] = openapi_types.UUID(seat)
	}
	a.ColleagueIds = &out
	return attachGuests(ctx, tx, a)
}

// attachGuests fills guests on a meeting read whole. They are the rows that name
// a guest by address alone, because no contact or seat stands behind them yet.
func attachGuests(ctx context.Context, tx pgx.Tx, a *crmcontracts.Activity) error {
	if a.Kind != crmcontracts.ActivityKindMeeting {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (ap.address) ap.address, coalesce(ap.display_name, '')
		  FROM activity_participant ap
		 WHERE ap.activity_id = $1 AND ap.address IS NOT NULL
		   AND ap.user_id IS NULL AND ap.contact_id IS NULL AND ap.role IN ($2, $3)
		 ORDER BY ap.address, coalesce(ap.display_name, '') DESC`,
		a.Id, connector.ParticipantRoleOrganizer, connector.ParticipantRoleAttendee)
	if err != nil {
		return fmt.Errorf("activities: reading the guests on a meeting: %w", err)
	}
	guests, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.MeetingGuest, error) {
		var g crmcontracts.MeetingGuest
		return g, row.Scan(&g.Address, &g.DisplayName)
	})
	if err != nil {
		return fmt.Errorf("activities: reading the guests on a meeting: %w", err)
	}
	a.Guests = &guests
	return nil
}
