// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WhoseMove is who answers an exchange next. A mail's History badge move
// decides when the reader may read the mail; otherwise its direction does.
func WhoseMove(move *crmcontracts.EmailSummaryMove, direction string) crmcontracts.LastExchangeWhoseMove {
	if move != nil {
		switch *move {
		case crmcontracts.EmailSummaryMoveWaitingForThem:
			return crmcontracts.LastExchangeWhoseMoveThem
		case crmcontracts.EmailSummaryMoveNeedsReply:
			return crmcontracts.LastExchangeWhoseMoveUs
		}
		return crmcontracts.LastExchangeWhoseMoveNeither
	}
	switch direction {
	case string(crmcontracts.ActivityDirectionOutbound):
		return crmcontracts.LastExchangeWhoseMoveThem
	case string(crmcontracts.ActivityDirectionInbound):
		return crmcontracts.LastExchangeWhoseMoveUs
	}
	return crmcontracts.LastExchangeWhoseMoveNeither
}

// lastExchange reads the record's newest call, meeting, email or message, by
// the interaction rule the account's strip reads its newest message with.
func (s *Store) lastExchange(
	ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, now time.Time, columns CustomColumns,
) (*crmcontracts.LastExchange, error) {
	one := 1
	rows, _, err := s.ListActivitiesIn(ctx, tx, ListActivitiesInput{
		EntityType: &entityType, EntityID: &entityID, InteractionsAsOf: &now,
		Limit: &one, AsOf: &now, Columns: columns,
	})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	row := rows[0]
	moves, err := MailMovesFor(ctx, tx, []ids.UUID{ids.UUID(row.Id)})
	if err != nil {
		return nil, err
	}
	var move *crmcontracts.EmailSummaryMove
	if found, ok := moves[ids.UUID(row.Id)]; ok {
		move = &found
	}
	direction := ""
	if row.Direction != nil {
		direction = string(*row.Direction)
	}
	return &crmcontracts.LastExchange{Activity: row, WhoseMove: WhoseMove(move, direction)}, nil
}
