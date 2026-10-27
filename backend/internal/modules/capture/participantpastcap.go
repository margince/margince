// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// StampSeatsPastTheCap binds the workspace's own seats from a party list the
// MaxParticipants cap withheld, and nobody else on it.
//
// A seat's own calendar holds the event. The row admits them to a meeting they
// attended, and no contact is resolved. Live capture then names the outside
// guests (stampGuestsPastCap).
func StampSeatsPastTheCap(
	ctx context.Context,
	tx pgx.Tx,
	activityID ids.ActivityID,
	kind string,
	partyListIsAttested bool,
	withheld []connector.MessageParticipant,
) error {
	_, err := stampSeatsPastCap(ctx, tx, activityID, kind, partyListIsAttested, withheld, nil)
	return err
}

// stampSeatsPastCap is StampSeatsPastTheCap with the roster that listed the
// seats, answering how many rows it wrote. A seat that declined is not bound.
func stampSeatsPastCap(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, kind string,
	partyListIsAttested bool, withheld []connector.MessageParticipant, rosterSeat *ids.UUID,
) (int, error) {
	// Unattested, the list is a sender's text, and binding a seat from it is the
	// forged edge StampFurtherParticipants refuses for the same reason.
	// Meetings only: a seat on an invitation attended it, which is the case the
	// decision covers. A crowded mail list stays refused whole.
	if !partyListIsAttested || kind != "meeting" || len(withheld) == 0 {
		return 0, nil
	}
	addresses := make([]string, 0, len(withheld))
	roles := make([]string, 0, len(withheld))
	names := make([]string, 0, len(withheld))
	responses := make([]string, 0, len(withheld))
	for _, p := range withheld {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		if address == "" || p.Declined() {
			continue
		}
		addresses = append(addresses, address)
		roles = append(roles, p.Role)
		names = append(names, strings.TrimSpace(p.DisplayName))
		responses = append(responses, p.Response)
	}
	// One attendance row per seat: a colleague listed twice is one colleague in
	// the room. The owner's directional row from capture records whose calendar
	// it is, not attendance, so it does not stand in for this one.
	args := []any{activityID, addresses, roles, names, storedDomainsOf(addresses), rosterSeat, responses}
	seatMatch, err := calendarSeatMatch(ctx, tx, kind, partyListIsAttested, &args)
	if err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, user_id, address, role, display_name, address_domain_folded, roster_seat, response)
		SELECT DISTINCT ON (u.id) $1, u.id, inp.address, inp.role, inp.display_name, inp.domain_folded, $6::uuid, NULLIF(inp.response, '')
		  FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $7::text[]) WITH ORDINALITY AS inp(address, role, display_name, domain_folded, response, ord)
		  JOIN app_user u ON `+seatMatch+`
		 WHERE NOT EXISTS (
		       SELECT 1 FROM activity_participant ap
		        WHERE ap.activity_id = $1 AND ap.user_id = u.id AND ap.role IN ('organizer', 'attendee'))
		 ORDER BY u.id, inp.ord
		ON CONFLICT DO NOTHING`,
		args...)
	if err != nil {
		return 0, fmt.Errorf("capture: binding the colleagues on a meeting past the party cap: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// rowSpeakingFor answers which row already speaks for a party, if any. An
// account names one first; an entry carrying none falls back to its address,
// because a party described once with an account and once without is one human.
func rowSpeakingFor(rowOf, rowOfAddress map[string]int, account, address string) (int, bool) {
	if account != "" {
		at, ok := rowOf[account]
		return at, ok
	}
	if address != "" {
		at, ok := rowOfAddress[address]
		return at, ok
	}
	return -1, false
}

// storedDomainsOf is owndomain.StoredOfAddress for each address, in order, for
// a set-based INSERT that writes the folded domain beside each address.
func storedDomainsOf(addresses []string) []*string {
	domains := make([]*string, len(addresses))
	for at, address := range addresses {
		domains[at] = owndomain.StoredOfAddress(address)
	}
	return domains
}
