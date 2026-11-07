// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// closeFacts is what a close reads besides the input check's evidence.
type closeFacts struct {
	// undoneAt is when the owner last undid a close of this deal.
	undoneAt *time.Time
	// workedAt is the newest note, call or meeting a colleague logged after it.
	workedAt *time.Time
	// anyProgressAt and refusalAt count every customer message, those only
	// their participants may read included. Only the dates are read: a private
	// reply keeps a deal open without its content reaching any report.
	anyProgressAt *time.Time
	refusalAt     *time.Time
	// waitUntil is the day the customer asked us to wait until; meetingAhead
	// is a meeting still to come that a customer guest accepted.
	waitUntil    *time.Time
	meetingAhead bool
	// mailboxes are the mail connections that delivered the deal's mail, of
	// every audience, in dealMailboxesSQL's form.
	mailboxes []string
}

// deferredAt reports a wait or an accepted meeting still ahead at now.
func (f closeFacts) deferredAt(now time.Time) bool {
	return f.meetingAhead || (f.waitUntil != nil && now.Before(*f.waitUntil))
}

// keptOpenAt is when a colleague last said the deal is alive, after an Undo. Nil
// when nobody undid a close, or nobody logged anything since.
func (f closeFacts) keptOpenAt() *time.Time {
	if f.undoneAt == nil {
		return nil
	}
	return f.workedAt
}

// closeFactsSQL reads closeFacts for the deals in $1, keyed by deal id.
var closeFactsSQL = `
	SELECT d.id::text, u.undone_at,
	       (SELECT max(a.created_at) FROM activity a JOIN activity_link al ON al.activity_id = a.id
	         WHERE al.deal_id = d.id AND a.archived_at IS NULL AND a.kind IN ('note', 'call', 'meeting')
	           AND a.origin = $4 AND starts_with(a.captured_by, $5) AND a.created_at > u.undone_at),
	       (SELECT max(a.occurred_at) FROM activity a JOIN activity_link al ON al.activity_id = a.id
	         WHERE al.deal_id = d.id AND a.archived_at IS NULL AND %[1]s),
	       (SELECT max(a.occurred_at) FROM activity a JOIN activity_link al ON al.activity_id = a.id
	         WHERE al.deal_id = d.id AND a.archived_at IS NULL AND %[2]s),
	       d.wait_until,
	       EXISTS (SELECT 1 FROM activity a JOIN activity_link al ON al.activity_id = a.id
	                WHERE al.deal_id = d.id AND a.archived_at IS NULL AND a.kind = 'meeting' AND %[3]s
	                  AND EXISTS (SELECT 1 FROM activity_participant p
	                               WHERE p.activity_id = a.id AND p.contact_id IS NOT NULL
	                                 AND p.response = 'accepted')),
	       %[4]s
	  FROM deal d
	  LEFT JOIN LATERAL (SELECT max(c.reversed_at) AS undone_at FROM deal_correction c
	                      WHERE c.deal_id = d.id AND c.correction = $3) u ON true
	 WHERE d.id::text = ANY($1)`

func closeFactsFor(
	ctx context.Context, tx pgx.Tx, now time.Time, subjects []assurance.Subject,
) (map[string]closeFacts, error) {
	dealIDs := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		dealIDs = append(dealIDs, subject.DealID)
	}
	query := fmt.Sprintf(closeFactsSQL, customerProgressAnyAudienceSQL("a", "$2"), customerRefusalAnyAudienceSQL("a", "$2"),
		relstrength.MeetingAheadSQL("a", "$2"), dealMailboxesAnyAudienceSQL("d"))
	rows, err := tx.Query(ctx, query, dealIDs, now, deals.CloseLostCorrection,
		activities.OriginHuman, principal.HumanIDPrefix)
	if err != nil {
		return nil, fmt.Errorf("compose: reading what keeps a deal open: %w", err)
	}
	defer rows.Close()
	out := make(map[string]closeFacts, len(dealIDs))
	for rows.Next() {
		var deal string
		var f closeFacts
		if err := rows.Scan(&deal, &f.undoneAt, &f.workedAt, &f.anyProgressAt, &f.refusalAt,
			&f.waitUntil, &f.meetingAhead, &f.mailboxes); err != nil {
			return nil, fmt.Errorf("compose: reading what keeps a deal open: %w", err)
		}
		out[deal] = f
	}
	return out, rows.Err()
}
