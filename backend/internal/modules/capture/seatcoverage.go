// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// UncapturedSeat is a colleague who is on a contact's threads and has no
// connected mailbox, so mail they alone receive from that contact is not here.
type UncapturedSeat struct {
	UserID       ids.UUID
	DisplayName  string
	Invited      bool
	LastOnThread time.Time
}

// SeatsWithoutMailbox lists the colleagues who appear on this contact's threads
// and have no connected mailbox, most recent first, at most limit.
//
// A seat counts when a participant row names it by user_id (calendar
// attendees, own sent mail) or, for an unattested To/Cc list, by an address
// equal to its login email. Only counted, workspace-audience, unarchived
// activities qualify, the same terms the interaction graph folds, so a limited
// message discloses nothing here. Invited seats are included because they may
// receive mail before they sign in.
//
// This is information only. It must never feed graph_interaction_edge, an
// assignment or a send check: an address on a sender's Cc line is the sender's
// text, which is why StampFurtherParticipants does not bind it to a user_id.
// mailProviders names the connection providers that deliver mail; a
// calendar-only connection does not make a seat's mail visible.
//
// The caller must already have passed the contact read gates.
func SeatsWithoutMailbox(
	ctx context.Context, tx pgx.Tx, contactID ids.UUID, mailProviders []string, limit int,
) ([]UncapturedSeat, error) {
	rows, err := tx.Query(ctx, `
		SELECT u.id, u.display_name, u.status = 'invited', max(a.occurred_at)
		  FROM activity_participant pc
		  JOIN activity a ON a.id = pc.activity_id AND a.archived_at IS NULL
		   AND `+relstrength.WorkspaceAudienceSQL("a")+`
		   AND `+relstrength.InteractionCountsSQL("a")+`
		  JOIN activity_participant ps ON ps.activity_id = a.id AND ps.id <> pc.id
		  JOIN app_user u ON u.is_agent = false AND `+workforce.ActivatableMemberSQL("u")+`
		   AND (ps.user_id = u.id
		        OR (ps.user_id IS NULL AND lower(ps.address) = lower(u.email)))
		 WHERE pc.contact_id = $1 AND pc.role IN `+relstrength.InteractionRolesSQL+`
		   AND NOT EXISTS (
		       SELECT 1 FROM capture_connection cc
		        WHERE cc.user_id = u.id AND cc.status = 'connected'
		          AND cc.archived_at IS NULL AND cc.provider = ANY($2))
		 GROUP BY u.id, u.display_name, u.status
		 ORDER BY max(a.occurred_at) DESC, u.id
		 LIMIT $3`, contactID, mailProviders, limit)
	if err != nil {
		return nil, fmt.Errorf("capture: reading seats without a mailbox: %w", err)
	}
	defer rows.Close()
	out := []UncapturedSeat{}
	for rows.Next() {
		var s UncapturedSeat
		if err := rows.Scan(&s.UserID, &s.DisplayName, &s.Invited, &s.LastOnThread); err != nil {
			return nil, fmt.Errorf("capture: scanning a seat without a mailbox: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading seats without a mailbox: %w", err)
	}
	return out, nil
}
