// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
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
// the interaction rule the account's strip reads its newest message with. At a
// tie the row that decides the move is LastWord's.
func (s *Store) lastExchange(
	ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, now time.Time, columns CustomColumns,
) (*crmcontracts.LastExchange, error) {
	tied, err := s.newestTied(ctx, tx, ListActivitiesInput{
		EntityType: &entityType, EntityID: &entityID, InteractionsAsOf: &now, AsOf: &now, Columns: columns,
	})
	if err != nil || len(tied) == 0 {
		return nil, err
	}
	row := LastWord(tied, exchangeMoment)
	moves, err := MailMovesFor(ctx, tx, []ids.UUID{ids.UUID(row.Id)}, now)
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

// newestTied reads the rows in at the newest stored instant, newest first,
// paging until that instant ends.
func (s *Store) newestTied(ctx context.Context, tx pgx.Tx, in ListActivitiesInput) ([]crmcontracts.Activity, error) {
	size := tiedPageSize
	in.Limit = &size
	var tied []crmcontracts.Activity
	for {
		rows, page, err := s.ListActivitiesIn(ctx, tx, in)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if len(tied) > 0 && !row.OccurredAt.Equal(tied[0].OccurredAt) {
				return tied, nil
			}
			tied = append(tied, row)
		}
		if !page.HasMore {
			return tied, nil
		}
		in.Cursor = &page.NextCursor
	}
}

// tiedPageSize covers the usual tie, an inbound and its reply, in one read.
const tiedPageSize = 4

func exchangeMoment(row crmcontracts.Activity) (occurredday.Moment, bool) {
	inbound := row.Direction != nil && *row.Direction == crmcontracts.ActivityDirectionInbound
	return momentOf(row), inbound
}
