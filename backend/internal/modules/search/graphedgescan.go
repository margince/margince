// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// edgeLastIsDaySQL says the activity an edge's last_at names is a day entry:
// one the member and the contact both took part in, as the fold counts them.
var edgeLastIsDaySQL = occurredday.ClockIsDaySQL(`FROM activity_participant clock_up
		JOIN activity_participant clock_pp ON clock_pp.activity_id = clock_up.activity_id
		JOIN activity clock_row ON clock_row.id = clock_up.activity_id
		WHERE clock_up.user_id = e.user_id AND clock_pp.contact_id = e.contact_id`, "e.last_at")

// scanEdges reads edge rows. Who sent a contact mail, and how often, is
// activity content: a caller without activity:read keeps the pooled counts and
// gets no sent count.
func scanEdges(ctx context.Context, rows pgx.Rows) ([]InteractionEdge, error) {
	mayReadAuthorship := auth.ReadGranted(ctx, "activity")
	var out []InteractionEdge
	for rows.Next() {
		var e InteractionEdge
		if err := rows.Scan(&e.UserID, &e.ContactID, &e.LastAt, &e.LastInboundAt, &e.LastOutbound, &e.LastMetAt,
			&e.LateCount90d, &e.LateOutCount90d,
			&e.Count90d, &e.InCount90d, &e.OutCount90d, &e.SentCount90d, &e.CountTotal, &e.LastAtDateOnly); err != nil {
			return nil, err
		}
		if !mayReadAuthorship {
			e.SentCount90d = nil
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
