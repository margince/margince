// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

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
			&e.Count90d, &e.InCount90d, &e.OutCount90d, &e.SentCount90d, &e.CountTotal); err != nil {
			return nil, err
		}
		if !mayReadAuthorship {
			e.SentCount90d = nil
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
