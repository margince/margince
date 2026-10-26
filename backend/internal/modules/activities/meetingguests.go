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

// maxListedGuests bounds one meeting's guest list: a roster at the party cap,
// and as many again that members or other calendars added beside it.
const maxListedGuests = 2 * connector.MaxParticipants

// guestRow is one participant row a meeting's guest list is built from.
type guestRow struct {
	address, invitationName string
	contactID               *ids.UUID
	// readable holds the contact's name when the reader may read that contact.
	readable *string
}

// attachGuests fills guests on a meeting read whole. A guest is listed unless
// they hold a seat or a contact link the reader sees already shows them.
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
	return nil
}

// guest renders one row for the reader. A contact they may not read is shown as
// the invitation named them, and a row naming nothing they may see is left out.
func (row guestRow) guest() (crmcontracts.MeetingGuest, bool) {
	var out crmcontracts.MeetingGuest
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
		SELECT coalesce(ap.address, ''), coalesce(ap.display_name, ''), ap.contact_id,
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
		return g, row.Scan(&g.address, &g.invitationName, &g.contactID, &g.readable)
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading the guests on a meeting: %w", err)
	}
	return guests, nil
}

func derefLinks(links *[]crmcontracts.ActivityLink) []crmcontracts.ActivityLink {
	if links == nil {
		return nil
	}
	return *links
}
