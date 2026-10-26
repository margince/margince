// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// maxListedGuests bounds one meeting's guest list. It holds a roster at the
// party cap, as many again from members or other calendars, and a crowd's guests.
const maxListedGuests = 2*connector.MaxParticipants + connector.MaxGuestsPastCap

// guestRow is one participant row a meeting's guest list is built from.
type guestRow struct {
	address, invitationName string
	response                *string
	contactID               *ids.UUID
	// readable holds the contact's name when the reader may read that contact.
	readable *string
}

// attachGuests fills guests, the answers of the seats and contacts shown, and
// the count a crowd leaves out on a meeting read whole. A guest is listed
// unless they hold a seat or a contact link the reader sees already shows them.
func attachGuests(ctx context.Context, tx pgx.Tx, a *crmcontracts.Activity) error {
	if a.Kind != crmcontracts.ActivityKindMeeting {
		return nil
	}
	rows, err := readGuestRows(ctx, tx, ids.UUID(a.Id))
	if err != nil {
		return err
	}
	shown := map[ids.UUID]bool{}
	for _, link := range derefLinks(a.Links) {
		if link.EntityType == crmcontracts.ActivityLinkEntityType(linkEntityContact) {
			shown[ids.UUID(link.EntityId)] = true
		}
	}
	if err := attachAttendeeResponses(ctx, tx, a, shown); err != nil {
		return err
	}
	seen := map[string]bool{}
	guests := []crmcontracts.MeetingGuest{}
	for _, row := range rows {
		key := "address:" + row.address
		if row.contactID != nil {
			key = "contact:" + row.contactID.String()
		}
		if seen[key] || (row.contactID != nil && shown[*row.contactID]) {
			continue
		}
		seen[key] = true
		if guest, ok := row.guest(); ok {
			guests = append(guests, guest)
		}
	}
	a.Guests = &guests
	var leftOut int
	if err := tx.QueryRow(ctx, `SELECT guests_left_out FROM activity WHERE id = $1`, a.Id).Scan(&leftOut); err != nil {
		return fmt.Errorf("activities: reading how many guests a meeting leaves out: %w", err)
	}
	a.GuestsLeftOut = &leftOut
	return nil
}

// guest renders one row for the reader. A contact they may not read is shown as
// the invitation named them, and a row naming nothing they may see is left out.
func (row guestRow) guest() (crmcontracts.MeetingGuest, bool) {
	var out crmcontracts.MeetingGuest
	if row.response != nil {
		response := crmcontracts.MeetingResponse(*row.response)
		out.Response = &response
	}
	if row.address != "" {
		address := row.address
		out.Address = &address
	}
	if row.readable != nil {
		id := openapi_types.UUID(*row.contactID)
		out.ContactId, out.DisplayName = &id, *row.readable
		return out, true
	}
	out.DisplayName = row.invitationName
	return out, out.Address != nil
}

func readGuestRows(ctx context.Context, tx pgx.Tx, activityID ids.UUID) ([]guestRow, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	meeting, organizer, attendee := arg(activityID), arg(connector.ParticipantRoleOrganizer), arg(connector.ParticipantRoleAttendee)
	readable := scopeNothing
	if auth.ReadGranted(ctx, "contact") {
		clause, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
		if err != nil {
			return nil, err
		}
		readable = orUnbounded(clause)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT coalesce(ap.address, ''), coalesce(ap.display_name, ''), ap.response, ap.contact_id,
		       CASE WHEN c.id IS NOT NULL AND (%s) THEN c.full_name END
		  FROM activity_participant ap
		  LEFT JOIN contact c ON c.id = ap.contact_id AND c.archived_at IS NULL
		 WHERE ap.activity_id = $%d AND ap.user_id IS NULL AND ap.role IN ($%d, $%d)
		 ORDER BY ap.contact_id NULLS LAST, ap.address, ap.id
		 LIMIT $%d`, readable, meeting, organizer, attendee, arg(maxListedGuests)), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the guests on a meeting: %w", err)
	}
	guests, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (guestRow, error) {
		var g guestRow
		return g, row.Scan(&g.address, &g.invitationName, &g.response, &g.contactID, &g.readable)
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the guests on a meeting: %w", err)
	}
	return guests, nil
}

// attachAttendeeResponses fills attendee_responses: the answer each colleague
// the meeting names, and each contact a link the reader sees shows, gave.
func attachAttendeeResponses(ctx context.Context, tx pgx.Tx, a *crmcontracts.Activity, shownContacts map[ids.UUID]bool) error {
	seats := map[ids.UUID]bool{}
	for _, seat := range derefUUIDs(a.ColleagueIds) {
		seats[ids.UUID(seat)] = true
	}
	if a.HostUserId != nil {
		seats[ids.UUID(*a.HostUserId)] = true
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (coalesce(user_id, contact_id)) user_id, contact_id, response
		  FROM activity_participant
		 WHERE activity_id = $1 AND response IS NOT NULL AND role IN ($2, $3)
		   AND (user_id IS NOT NULL OR contact_id IS NOT NULL)
		 ORDER BY coalesce(user_id, contact_id), created_at DESC, id`,
		a.Id, connector.ParticipantRoleOrganizer, connector.ParticipantRoleAttendee)
	if err != nil {
		return fmt.Errorf("activities: reading the answers on a meeting: %w", err)
	}
	answers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.MeetingAttendeeResponse, error) {
		var userID, contactID *ids.UUID
		var response string
		if err := row.Scan(&userID, &contactID, &response); err != nil {
			return crmcontracts.MeetingAttendeeResponse{}, err
		}
		out := crmcontracts.MeetingAttendeeResponse{Response: crmcontracts.MeetingResponse(response)}
		if userID != nil && seats[*userID] {
			id := openapi_types.UUID(*userID)
			out.UserId = &id
		} else if contactID != nil && userID == nil && shownContacts[*contactID] {
			id := openapi_types.UUID(*contactID)
			out.ContactId = &id
		}
		return out, nil
	})
	if err != nil {
		return fmt.Errorf("activities: reading the answers on a meeting: %w", err)
	}
	shown := make([]crmcontracts.MeetingAttendeeResponse, 0, len(answers))
	for _, answer := range answers {
		if answer.UserId != nil || answer.ContactId != nil {
			shown = append(shown, answer)
		}
	}
	a.AttendeeResponses = &shown
	return nil
}

func derefUUIDs(list *[]openapi_types.UUID) []openapi_types.UUID {
	if list == nil {
		return nil
	}
	return *list
}

func derefLinks(links *[]crmcontracts.ActivityLink) []crmcontracts.ActivityLink {
	if links == nil {
		return nil
	}
	return *links
}
