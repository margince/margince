// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// crowdGuests is what stampGuestsPastCap did on one meeting.
type crowdGuests struct {
	written, leftOut int
}

// stampGuestsPastCap names a crowded meeting's outside guests by address alone,
// up to connector.MaxGuestsPastCap, after its seats are bound. A row it writes
// is marked past_party_cap, so no reader resolves, files or credits it.
func stampGuestsPastCap(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, kind string,
	partyListIsAttested bool, withheld []connector.MessageParticipant, rosterSeat *ids.UUID,
) (crowdGuests, error) {
	if !partyListIsAttested || kind != meetingKind || len(withheld) == 0 {
		return crowdGuests{}, nil
	}
	held, kept, err := crowdRoom(ctx, tx, activityID)
	if err != nil {
		return crowdGuests{}, err
	}
	room := max(connector.MaxGuestsPastCap-kept, 0)
	var out crowdGuests
	addresses := make([]string, 0, min(len(withheld), room))
	roles := make([]string, 0, cap(addresses))
	names := make([]string, 0, cap(addresses))
	responses := make([]string, 0, cap(addresses))
	for _, p := range withheld {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		if address == "" || held[address] {
			continue
		}
		held[address] = true
		if len(addresses) == room {
			out.leftOut++
			continue
		}
		addresses = append(addresses, address)
		roles = append(roles, p.Role)
		names = append(names, strings.TrimSpace(p.DisplayName))
		responses = append(responses, p.Response)
	}
	if len(addresses) == 0 {
		return out, nil
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, address, role, display_name, address_domain_folded, roster_seat, response, past_party_cap)
		SELECT $1, inp.address, inp.role, inp.display_name, inp.domain_folded, $6::uuid, NULLIF(inp.response, ''), true
		  FROM unnest($2::text[], $3::text[], $4::text[], $5::text[], $7::text[]) AS inp(address, role, display_name, domain_folded, response)
		ON CONFLICT DO NOTHING`,
		activityID, addresses, roles, names, storedDomainsOf(addresses), rosterSeat, responses)
	if err != nil {
		return crowdGuests{}, fmt.Errorf("capture: naming the outside guests of a meeting past the party cap: %w", err)
	}
	out.written = int(tag.RowsAffected())
	return out, nil
}

// crowdRoom answers every address the meeting holds, and how many rows past
// the party cap it keeps already.
func crowdRoom(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (map[string]bool, int, error) {
	rows, err := tx.Query(ctx, `
		SELECT address, bool_or(past_party_cap) FROM activity_participant
		 WHERE activity_id = $1 AND address IS NOT NULL GROUP BY address`, activityID)
	if err != nil {
		return nil, 0, fmt.Errorf("capture: reading who a crowded meeting already names: %w", err)
	}
	defer rows.Close()
	held := map[string]bool{}
	kept := 0
	for rows.Next() {
		var address string
		var pastCap bool
		if err := rows.Scan(&address, &pastCap); err != nil {
			return nil, 0, fmt.Errorf("capture: reading who a crowded meeting already names: %w", err)
		}
		held[address] = true
		if pastCap {
			kept++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("capture: reading who a crowded meeting already names: %w", err)
	}
	return held, kept, nil
}

// recordGuestsLeftOut stores how many outside guests the bound left off the
// meeting, and answers whether that count moved.
func recordGuestsLeftOut(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, leftOut int) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE activity SET guests_left_out = $2
		 WHERE id = $1 AND archived_at IS NULL AND guests_left_out IS DISTINCT FROM $2`,
		activityID, leftOut)
	if err != nil {
		return false, fmt.Errorf("capture: counting the guests a crowded meeting leaves out: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// settleAnswers brings the answers on this calendar's rows in line with its
// roster. A current roster replaces a stored answer; a stored original only
// fills a row that holds none. An empty answer never erases one.
func settleAnswers(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, roster MeetingRoster, listed []connector.MessageParticipant,
) (int, error) {
	addresses := make([]string, 0, len(listed))
	responses := make([]string, 0, len(listed))
	for _, p := range listed {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		if address == "" || p.Response == "" {
			continue
		}
		addresses = append(addresses, address)
		responses = append(responses, p.Response)
	}
	if len(addresses) == 0 {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE activity_participant ap SET response = inp.response
		  FROM unnest($3::text[], $4::text[]) AS inp(address, response)
		 WHERE ap.activity_id = $1 AND ap.roster_seat = $2 AND ap.address = inp.address
		   AND ap.response IS DISTINCT FROM inp.response AND ($5 OR ap.response IS NULL)`,
		activityID, roster.Seat, addresses, responses, roster.Current)
	if err != nil {
		return 0, fmt.Errorf("capture: recording the answers a calendar lists: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
