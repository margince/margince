// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// StampSeatsPastTheCap binds the workspace's own seats from a party list the
// MaxParticipants cap withheld, and nobody else on it.
//
// The cap refuses a crowded invitation because its names are a distribution
// list, and folding them in would report a relationship with everybody who got
// the same mail. A seat is not that: the event is in their own calendar, and
// the row admits them to a meeting they attended. So the seat arm is the only
// one taken here. No contact is resolved and no unknown address is kept, which
// leaves every external name exactly as refused as the cap left it.
//
// Live capture and the attendee repair both call this, so one invitation is
// split into seats and strangers by one rule whichever pass reads it.
func StampSeatsPastTheCap(
	ctx context.Context,
	tx pgx.Tx,
	activityID ids.ActivityID,
	kind string,
	partyListIsAttested bool,
	withheld []connector.MessageParticipant,
) error {
	// Unattested, the list is a sender's text, and binding a seat from it is the
	// forged edge StampFurtherParticipants refuses for the same reason.
	if !partyListIsAttested || !relstrength.IsParticipantKind(kind) || len(withheld) == 0 {
		return nil
	}
	addresses := make([]string, 0, len(withheld))
	roles := make([]string, 0, len(withheld))
	names := make([]string, 0, len(withheld))
	for _, p := range withheld {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		if address == "" {
			continue
		}
		addresses = append(addresses, address)
		roles = append(roles, p.Role)
		names = append(names, strings.TrimSpace(p.DisplayName))
	}
	// DISTINCT ON the seat: a colleague listed as organizer and attendee is
	// still one person in the room, and the first role stated stands.
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, user_id, address, role, display_name)
		SELECT DISTINCT ON (u.id) $1, u.id, inp.address, inp.role, inp.display_name
		  FROM unnest($2::text[], $3::text[], $4::text[]) WITH ORDINALITY AS inp(address, role, display_name, ord)
		  JOIN app_user u ON lower(u.email) = inp.address
		 ORDER BY u.id, inp.ord
		ON CONFLICT DO NOTHING`,
		activityID, addresses, roles, names); err != nil {
		return fmt.Errorf("capture: binding the colleagues on a meeting past the party cap: %w", err)
	}
	return nil
}
