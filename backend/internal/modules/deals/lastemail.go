// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The newest email on each deal of a page, as the board card states it.
//
// A finish pass over the page rather than a correlated subquery in dealColumns
// (deal_read.go says why the closing occurrence is not one either): ONE
// statement answers every card on the page, where a subquery per row would run
// a hundred times on a board that draws a hundred cards.
//
// An AGGREGATE about mail, so it asks the audience the way last_activity_at
// does: a colleague who may not read a message must not see its date in the
// shape of a number either, and a mail the installation sent itself is not the
// buyer engaging. The two clauses are the shared spellings, so a third system
// origin or a fourth audience reaches this reader without anybody editing it.

// attachLastEmail puts each deal's newest workspace-visible email on the wire
// shape, leaving it absent where nobody has mailed about the deal.
func attachLastEmail(ctx context.Context, tx pgx.Tx, deals []crmcontracts.Deal) error {
	if len(deals) == 0 {
		return nil
	}
	dealIDs := make([]ids.UUID, len(deals))
	for i := range deals {
		dealIDs[i] = ids.UUID(deals[i].Id)
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (l.deal_id) l.deal_id, a.occurred_at, a.direction
		  FROM activity_link l
		  JOIN activity a ON a.id = l.activity_id
		 WHERE l.deal_id = ANY($1)
		   AND a.kind = 'email'
		   AND a.archived_at IS NULL
		   `+auth.OriginIsEngagement("a")+auth.AudienceWorkspaceOnly("a")+`
		 ORDER BY l.deal_id, a.occurred_at DESC, a.id DESC`, dealIDs)
	if err != nil {
		return fmt.Errorf("deals: reading the newest email on a page of deals: %w", err)
	}
	defer rows.Close()
	newest := make(map[ids.UUID]crmcontracts.DealLastEmail, len(deals))
	for rows.Next() {
		var deal ids.UUID
		var occurredAt time.Time
		var direction *string
		if err := rows.Scan(&deal, &occurredAt, &direction); err != nil {
			return fmt.Errorf("deals: reading a deal's newest email: %w", err)
		}
		newest[deal] = crmcontracts.DealLastEmail{OccurredAt: occurredAt, Direction: wireDirection(direction)}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("deals: reading the newest email on a page of deals: %w", err)
	}
	for i := range deals {
		if last, found := newest[ids.UUID(deals[i].Id)]; found {
			deals[i].LastEmail = &last
		}
	}
	return nil
}

// wireDirection carries the column's nullable text onto the contract's enum.
func wireDirection(direction *string) *crmcontracts.DealLastEmailDirection {
	if direction == nil {
		return nil
	}
	out := crmcontracts.DealLastEmailDirection(*direction)
	return &out
}
