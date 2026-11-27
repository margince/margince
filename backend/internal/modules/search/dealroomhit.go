// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attachRoomDeals fills DealID on the deal_room hits of one page. A client
// opens a room through its deal, so the hit names the deal. The read carries the
// room list's own scope again, so the id discloses nothing the list does not.
func attachRoomDeals(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	var roomIDs []ids.UUID
	for i := range hits {
		if hits[i].Type == entityDealRoom {
			roomIDs = append(roomIDs, hits[i].ID)
		}
	}
	if len(roomIDs) == 0 {
		return nil
	}
	args := []any{roomIDs}
	scope, err := auth.DealChildScopeClause(ctx, "r", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT r.id, r.deal_id FROM deal_room r WHERE r.id = ANY($1) AND `+scope, args...)
	if err != nil {
		return fmt.Errorf("search: reading this page's room deals: %w", err)
	}
	defer rows.Close()
	dealOf := make(map[ids.UUID]ids.UUID, len(roomIDs))
	for rows.Next() {
		var room, deal ids.UUID
		if err := rows.Scan(&room, &deal); err != nil {
			return fmt.Errorf("search: reading this page's room deals: %w", err)
		}
		dealOf[room] = deal
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("search: reading this page's room deals: %w", err)
	}
	for i := range hits {
		if deal, ok := dealOf[hits[i].ID]; ok && hits[i].Type == entityDealRoom {
			hits[i].DealID = &deal
		}
	}
	return nil
}
