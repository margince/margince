// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the two morning passes share: who has a morning at all, and how one
// colleague's own authority is bound for the reads made on their behalf.
//
// The overnight brief and the notification digest are the same message to the
// same colleagues at the same hour — one says what to start with, the other what
// arrived overnight — so they answer these two questions together or they
// eventually answer them differently. The roster in particular: a seat the brief
// serves and the digest skips, or the reverse, is a difference nobody decided.
//
// The other job-time authority bindings in this package are deliberately not
// folded in here. An approval's fan-out is asking which colleagues may DECIDE
// something and a scheduled send is asking whose mailbox a message leaves from;
// they take the same snapshot but they answer different questions, and they
// differ in what an unresolvable seat costs them.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// morningSeatSQL is who has a morning: a live human full seat, aliased `u`.
//
// identity.LiveMemberSQL and not a hand-spelled pair — identity owns app_user,
// compose may reach it, and both halves matter because neither implies the
// other: deactivating a seat leaves archived_at NULL, so archived_at alone would
// go on mailing a departed colleague every morning of the year.
//
// Agents are excluded because an agent seat has no morning to prepare. Read
// seats are excluded because both messages are about a queue to act on — and a
// read seat still holds every notice addressed to it on screen, which is where a
// notice lands whatever these passes decide.
var morningSeatSQL = identity.LiveMemberSQL("u") + `
			  AND u.is_agent = false
			  AND u.seat_type = 'full'`

// seatsWithAMorning is the roster as an addressed list: every colleague who has
// a morning, and where a message to them goes.
//
// The address comes back WITH the roster rather than from a read per seat: they
// are one snapshot of the same table, and asked separately a seat could answer
// the first and be gone by the second. An empty address is possible in principle
// and means the same thing as absence to a caller — nowhere to send.
func seatsWithAMorning(ctx context.Context, tx pgx.Tx) (map[ids.UUID]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT u.id, u.email
		  FROM app_user u
		 WHERE `+morningSeatSQL)
	if err != nil {
		return nil, fmt.Errorf("listing the colleagues who have a morning: %w", err)
	}
	defer rows.Close()
	roster := map[ids.UUID]string{}
	for rows.Next() {
		var id ids.UUID
		var address string
		if err := rows.Scan(&id, &address); err != nil {
			return nil, err
		}
		roster[id] = address
	}
	return roster, rows.Err()
}

// seatContext binds one colleague's own authority, which every read and write on
// their behalf then runs under.
//
// EffectiveAuthority reads the grants AND the seat as ONE snapshot. Composed
// from separate reads they can describe an authority the colleague never held —
// permissions from before a role change with a seat type from after — and the
// seat type is carried onto the principal for that reason rather than as
// decoration: it is half of what the row-scope clauses answer with.
//
// A fresh correlation id per seat, so one morning's work for one colleague is
// one recoverable trace in the audit spine rather than a fleet pass nobody can
// take apart.
func seatContext(
	ctx context.Context, users *identity.Service, wsID, userID ids.UUID,
) (context.Context, error) {
	rbac, seat, err := users.EffectiveAuthority(ctx, wsID, userID)
	if err != nil {
		return nil, fmt.Errorf("resolving the colleague's authority: %w", err)
	}
	bound := principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + userID.String(),
		UserID:      userID,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	})
	return principal.WithCorrelationID(bound, ids.NewV7()), nil
}
