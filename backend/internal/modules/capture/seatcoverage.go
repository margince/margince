// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// UncapturedSeat is a colleague on a contact's threads with no connected
// mailbox. Mail they alone receive from that contact is not captured here.
type UncapturedSeat struct {
	UserID       ids.UUID
	DisplayName  string
	Invited      bool
	LastOnThread time.Time
	// LastOnThreadPrecision says how to read LastOnThread, from the activity it names.
	LastOnThreadPrecision string
}

// UncapturedSeats is the listed seats and how many met the condition in all,
// so a caller can say the list stopped at its limit.
type UncapturedSeats struct {
	Seats []UncapturedSeat
	Total int
}

// SeatsWithoutMailbox lists the colleagues on this contact's threads who have
// no connected mailbox, most recent first, at most limit, and counts them all.
//
// A seat counts when a participant row names it by user_id (calendar attendees,
// own sent mail). It also counts when an unattested To or Cc address equals its
// login email and the seat existed when the activity was captured. Only
// counted, workspace-audience, unarchived activities qualify, the terms the
// interaction graph folds, so a limited message discloses nothing here.
// Invited seats are included because they may receive mail before they sign in.
//
// This is information only. It never feeds graph_interaction_edge, an
// assignment or a send check, because an address on a Cc line is the sender's
// text. StampFurtherParticipants does not bind it to a user_id for that reason.
// mailProviders names the providers that deliver mail; a calendar-only
// connection does not make a seat's mail visible.
//
// The caller must already have passed the contact read gates.
func SeatsWithoutMailbox(
	ctx context.Context, tx pgx.Tx, contactID ids.UUID, mailProviders []string, limit int,
) (UncapturedSeats, error) {
	// The window count runs over the grouped seats before LIMIT, so the total
	// comes from the same rule and the same statement as the list.
	rows, err := tx.Query(ctx, `
		SELECT u.id, u.display_name, u.status = 'invited', max(a.occurred_at),
		       `+occurredday.LatestPrecisionSQL("a", "")+`, count(*) OVER ()
		  FROM activity_participant pc
		  JOIN activity a ON a.id = pc.activity_id AND a.archived_at IS NULL
		   AND `+relstrength.WorkspaceAudienceSQL("a")+`
		   AND `+relstrength.InteractionCountsSQL("a")+`
		  JOIN activity_participant ps ON ps.activity_id = a.id AND ps.id <> pc.id
		  JOIN app_user u ON u.is_agent = false AND `+workforce.ActivatableMemberSQL("u")+`
		   AND (ps.user_id = u.id
		        OR (ps.user_id IS NULL AND ps.contact_id IS NULL AND ps.role IN ('to','cc')
		            AND u.created_at <= a.created_at AND lower(ps.address) = lower(u.email)))
		 WHERE pc.contact_id = $1 AND pc.role IN `+relstrength.InteractionRolesSQL+`
		   AND NOT EXISTS (
		       SELECT 1 FROM capture_connection cc
		        WHERE cc.user_id = u.id AND `+liveConnection("cc")+`
		          AND cc.provider = ANY($2))
		 GROUP BY u.id, u.display_name, u.status
		 ORDER BY max(a.occurred_at) DESC, u.id
		 LIMIT $3`, contactID, mailProviders, limit)
	if err != nil {
		return UncapturedSeats{}, fmt.Errorf("capture: reading seats without a mailbox: %w", err)
	}
	defer rows.Close()
	out := UncapturedSeats{Seats: []UncapturedSeat{}}
	for rows.Next() {
		var s UncapturedSeat
		if err := rows.Scan(&s.UserID, &s.DisplayName, &s.Invited, &s.LastOnThread, &s.LastOnThreadPrecision, &out.Total); err != nil {
			return UncapturedSeats{}, fmt.Errorf("capture: scanning a seat without a mailbox: %w", err)
		}
		out.Seats = append(out.Seats, s)
	}
	if err := rows.Err(); err != nil {
		return UncapturedSeats{}, fmt.Errorf("capture: reading seats without a mailbox: %w", err)
	}
	return out, nil
}
